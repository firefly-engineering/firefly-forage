package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/injection"
)

// JJBackend implements Backend for jj (Jujutsu) repositories
type JJBackend struct{}

// JJ returns a new JJ workspace backend
func JJ() Backend {
	return &JJBackend{}
}

func (b *JJBackend) Name() string {
	return "jj"
}

func (b *JJBackend) IsRepo(path string) bool {
	jjPath := filepath.Join(path, ".jj", "repo")
	info, err := os.Stat(jjPath)
	return err == nil && info.IsDir()
}

func (b *JJBackend) Exists(repoPath, name string) bool {
	cmd := exec.Command("jj", "workspace", "list", "-R", repoPath)
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) > 0 && strings.TrimSpace(parts[0]) == name {
			return true
		}
	}
	return false
}

func (b *JJBackend) Create(repoPath, name, workspacePath string) error {
	if err := ValidateName(name); err != nil {
		return fmt.Errorf("invalid workspace name: %w", err)
	}
	cmd := exec.Command("jj", "workspace", "add", "-R", repoPath, "--name", name, workspacePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create jj workspace: %s: %w", string(output), err)
	}
	return nil
}

func (b *JJBackend) Remove(repoPath, name, workspacePath string) error {
	if err := ValidateName(name); err != nil {
		return fmt.Errorf("invalid workspace name: %w", err)
	}
	// Forget the workspace in jj
	cmd := exec.Command("jj", "workspace", "forget", name, "-R", repoPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to forget jj workspace: %w", err)
	}

	// Remove the workspace directory
	if workspacePath != "" {
		if err := os.RemoveAll(workspacePath); err != nil {
			return fmt.Errorf("failed to remove workspace directory: %w", err)
		}
	}

	return nil
}

// ContributePackages returns the packages needed for jj inside the container.
// The nixpkgs package is "jujutsu" (not "jj", which is an unrelated JSON tool).
func (b *JJBackend) ContributePackages(ctx context.Context) ([]injection.Package, error) {
	return []injection.Package{{Name: "jujutsu"}}, nil
}

// ContributeMounts returns mounts for jj workspace mode.
// Mounts both .jj and .git directories since jj uses git as its storage backend.
func (b *JJBackend) ContributeMounts(ctx context.Context, req *injection.MountRequest) ([]injection.Mount, error) {
	if req.SourceRepo == "" {
		return nil, nil
	}

	jjPath := filepath.Join(req.SourceRepo, ".jj")
	if _, err := os.Stat(jjPath); err != nil {
		return nil, nil
	}

	mounts := []injection.Mount{{
		HostPath:      jjPath,
		ContainerPath: jjPath,
		ReadOnly:      req.ReadOnlyWorkspace,
	}}

	// jj uses git as its storage backend, so .git must also be mounted
	gitPath := filepath.Join(req.SourceRepo, ".git")
	if _, err := os.Stat(gitPath); err == nil {
		mounts = append(mounts, injection.Mount{
			HostPath:      gitPath,
			ContainerPath: gitPath,
			ReadOnly:      req.ReadOnlyWorkspace,
		})
	}

	return mounts, nil
}

// ContributeEnvVars sets GIT_DIR so the git CLI can find the repository
// inside containers. jj mounts .git at the host absolute path, but the
// container workspace is at a different location, so git can't discover
// the repo by walking up from the working directory.
func (b *JJBackend) ContributeEnvVars(ctx context.Context, req *injection.EnvVarRequest) ([]injection.EnvVar, error) {
	if req.SourceRepo == "" {
		return nil, nil
	}
	gitDir := filepath.Join(req.SourceRepo, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		return nil, nil
	}
	// Value is a Nix expression (double-quoted string); the OCI path strips the quotes.
	return []injection.EnvVar{{Name: "GIT_DIR", Value: `"` + gitDir + `"`}}, nil
}

// ContributePromptFragments returns jj-specific VCS instructions.
func (b *JJBackend) ContributePromptFragments(ctx context.Context) ([]injection.PromptFragment, error) {
	return []injection.PromptFragment{{
		Section:  injection.PromptSectionVCS,
		Priority: 10,
		Content:  jjPromptInstructions,
	}}, nil
}

const jjPromptInstructions = `This workspace uses jj (Jujutsu) for version control. Use jj commands for all VCS operations:
- jj status: Show working copy status
- jj diff: Show changes
- jj new: Create new change
- jj describe -m "message": Set commit message
- jj bookmark set <name>: Update bookmark

This is an isolated jj workspace - changes don't affect other workspaces.`

// Snapshot creates a named jj bookmark at the current workspace revision.
func (b *JJBackend) Snapshot(repoPath, name, snapshotName string) error {
	if err := ValidateName(snapshotName); err != nil {
		return fmt.Errorf("invalid snapshot name: %w", err)
	}
	bookmarkName := snapshotPrefix + name + "-" + snapshotName
	cmd := exec.Command("jj", "bookmark", "create", bookmarkName, "-r", "@", "-R", repoPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create snapshot bookmark: %s: %w", string(output), err)
	}
	return nil
}

// RestoreSnapshot moves the workspace to a previously bookmarked snapshot.
func (b *JJBackend) RestoreSnapshot(repoPath, name, snapshotName string) error {
	if err := ValidateName(snapshotName); err != nil {
		return fmt.Errorf("invalid snapshot name: %w", err)
	}
	bookmarkName := snapshotPrefix + name + "-" + snapshotName
	cmd := exec.Command("jj", "edit", bookmarkName, "-R", repoPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to restore snapshot: %s: %w", string(output), err)
	}
	return nil
}

// ListSnapshots returns all forage snapshots for a workspace.
func (b *JJBackend) ListSnapshots(repoPath, name string) ([]SnapshotInfo, error) {
	prefix := snapshotPrefix + name + "-"
	cmd := exec.Command("jj", "bookmark", "list", "-R", repoPath)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list bookmarks: %w", err)
	}

	var snapshots []SnapshotInfo
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// jj bookmark list output format: "bookmarkname: changeID commitID"
		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 1 {
			continue
		}
		bname := strings.TrimSpace(parts[0])
		if !strings.HasPrefix(bname, prefix) {
			continue
		}
		snapName := strings.TrimPrefix(bname, prefix)
		var changeID string
		if len(parts) > 1 {
			changeID = strings.TrimSpace(parts[1])
		}
		snapshots = append(snapshots, SnapshotInfo{
			Name:     snapName,
			ChangeID: changeID,
		})
	}
	return snapshots, nil
}

// Ensure JJBackend implements contribution interfaces
var (
	_ injection.PackageContributor = (*JJBackend)(nil)
	_ injection.MountContributor   = (*JJBackend)(nil)
	_ injection.EnvVarContributor  = (*JJBackend)(nil)
	_ injection.PromptContributor  = (*JJBackend)(nil)
	_ Snapshotter                  = (*JJBackend)(nil)
)

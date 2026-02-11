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
	cmd := exec.Command("jj", "workspace", "add", "-R", repoPath, "--name", name, workspacePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create jj workspace: %s: %w", string(output), err)
	}
	return nil
}

func (b *JJBackend) Remove(repoPath, name, workspacePath string) error {
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
		ReadOnly:      false,
	}}

	// jj uses git as its storage backend, so .git must also be mounted
	gitPath := filepath.Join(req.SourceRepo, ".git")
	if _, err := os.Stat(gitPath); err == nil {
		mounts = append(mounts, injection.Mount{
			HostPath:      gitPath,
			ContainerPath: gitPath,
			ReadOnly:      false,
		})
	}

	return mounts, nil
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

// Ensure JJBackend implements contribution interfaces
var (
	_ injection.MountContributor  = (*JJBackend)(nil)
	_ injection.PromptContributor = (*JJBackend)(nil)
)

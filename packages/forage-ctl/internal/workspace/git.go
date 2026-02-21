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

const gitBranchPrefix = "forage-"

// GitBackend implements Backend for git repositories using worktrees
type GitBackend struct{}

// Git returns a new Git worktree workspace backend
func Git() Backend {
	return &GitBackend{}
}

func (b *GitBackend) Name() string {
	return "git-worktree"
}

func (b *GitBackend) IsRepo(path string) bool {
	gitPath := filepath.Join(path, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return false
	}
	// .git can be a directory (normal repo) or a file (worktree)
	return info.IsDir() || info.Mode().IsRegular()
}

func (b *GitBackend) Exists(repoPath, name string) bool {
	// Check if a worktree with this name's branch already exists
	branchName := gitBranchPrefix + name
	return b.branchExists(repoPath, branchName)
}

func (b *GitBackend) Create(repoPath, name, workspacePath string) error {
	if err := ValidateName(name); err != nil {
		return fmt.Errorf("invalid workspace name: %w", err)
	}
	branchName := gitBranchPrefix + name

	// Get the current HEAD to base the new branch on
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "HEAD")
	headOutput, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}
	head := strings.TrimSpace(string(headOutput))

	// Check if branch already exists
	if b.branchExists(repoPath, branchName) {
		// Use existing branch
		cmd = exec.Command("git", "-C", repoPath, "worktree", "add", workspacePath, branchName)
	} else {
		// Create new branch from HEAD
		cmd = exec.Command("git", "-C", repoPath, "worktree", "add", "-b", branchName, workspacePath, head)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create git worktree: %s: %w", string(output), err)
	}
	return nil
}

func (b *GitBackend) Remove(repoPath, name, workspacePath string) error {
	if err := ValidateName(name); err != nil {
		return fmt.Errorf("invalid workspace name: %w", err)
	}
	branchName := gitBranchPrefix + name

	// Remove the worktree
	if workspacePath != "" {
		if err := b.removeWorktree(repoPath, workspacePath); err != nil {
			return fmt.Errorf("failed to remove worktree: %w", err)
		}
	}

	// Delete the branch
	if err := b.deleteBranch(repoPath, branchName); err != nil {
		// Branch deletion failure is not fatal - worktree is already gone
		// The branch might have been merged or deleted manually
	}

	return nil
}

// BranchName returns the git branch name for a workspace
func (b *GitBackend) BranchName(name string) string {
	return gitBranchPrefix + name
}

func (b *GitBackend) branchExists(repoPath, branchName string) bool {
	cmd := exec.Command("git", "-C", repoPath, "show-ref", "--verify", "--quiet", "refs/heads/"+branchName)
	return cmd.Run() == nil
}

func (b *GitBackend) removeWorktree(repoPath, worktreePath string) error {
	// First try normal remove
	cmd := exec.Command("git", "-C", repoPath, "worktree", "remove", worktreePath)
	if err := cmd.Run(); err != nil {
		// Try force remove
		cmd = exec.Command("git", "-C", repoPath, "worktree", "remove", "--force", worktreePath)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %w", string(output), err)
		}
	}
	return nil
}

func (b *GitBackend) deleteBranch(repoPath, branchName string) error {
	// First try safe delete
	cmd := exec.Command("git", "-C", repoPath, "branch", "-d", branchName)
	if err := cmd.Run(); err != nil {
		// Try force delete
		cmd = exec.Command("git", "-C", repoPath, "branch", "-D", branchName)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %w", string(output), err)
		}
	}
	return nil
}

// WorktreeExists checks if a worktree exists at the given path
func WorktreeExists(repoPath, worktreePath string) bool {
	cmd := exec.Command("git", "-C", repoPath, "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	absPath, _ := filepath.Abs(worktreePath)
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			path := strings.TrimPrefix(line, "worktree ")
			if path == absPath {
				return true
			}
		}
	}
	return false
}

// ContributeMounts returns nil - git worktrees don't need extra mounts.
// The worktree directory already contains a .git file pointing to the main repo.
func (b *GitBackend) ContributeMounts(ctx context.Context, req *injection.MountRequest) ([]injection.Mount, error) {
	return nil, nil
}

// ContributePromptFragments returns git worktree-specific VCS instructions.
func (b *GitBackend) ContributePromptFragments(ctx context.Context) ([]injection.PromptFragment, error) {
	return []injection.PromptFragment{{
		Section:  injection.PromptSectionVCS,
		Priority: 10,
		Content:  gitWorktreePromptInstructions,
	}}, nil
}

const gitWorktreePromptInstructions = `This workspace is a git worktree with its own working directory and branch.
Use standard git commands for VCS operations:
- git status: Show working tree status
- git diff: Show changes
- git add -p: Stage changes interactively
- git commit -m "message": Create commit on this branch
- git push -u origin <branch>: Push branch

This is an isolated git worktree - commits on this branch don't affect other worktrees.
When done, merge your branch or create a pull request.`

// Snapshot creates a git tag at the current HEAD of the worktree.
func (b *GitBackend) Snapshot(repoPath, name, snapshotName string) error {
	if err := ValidateName(snapshotName); err != nil {
		return fmt.Errorf("invalid snapshot name: %w", err)
	}
	tagName := snapshotPrefix + name + "-" + snapshotName
	// Tag the current HEAD in the main repo, referencing the worktree branch
	branchName := gitBranchPrefix + name
	cmd := exec.Command("git", "-C", repoPath, "tag", tagName, branchName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create snapshot tag: %s: %w", string(output), err)
	}
	return nil
}

// RestoreSnapshot checks out a previously tagged snapshot in the worktree.
func (b *GitBackend) RestoreSnapshot(repoPath, name, snapshotName string) error {
	if err := ValidateName(snapshotName); err != nil {
		return fmt.Errorf("invalid snapshot name: %w", err)
	}
	tagName := snapshotPrefix + name + "-" + snapshotName
	branchName := gitBranchPrefix + name
	// Reset the worktree branch to the tagged commit
	cmd := exec.Command("git", "-C", repoPath, "update-ref", "refs/heads/"+branchName, tagName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to restore snapshot: %s: %w", string(output), err)
	}
	return nil
}

// ListSnapshots returns all forage snapshots for a workspace.
func (b *GitBackend) ListSnapshots(repoPath, name string) ([]SnapshotInfo, error) {
	prefix := snapshotPrefix + name + "-"
	cmd := exec.Command("git", "-C", repoPath, "tag", "-l", prefix+"*")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %w", err)
	}

	var snapshots []SnapshotInfo
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		snapName := strings.TrimPrefix(line, prefix)

		// Get the commit hash for the tag
		hashCmd := exec.Command("git", "-C", repoPath, "rev-parse", "--short", line)
		hashOutput, err := hashCmd.Output()
		var changeID string
		if err == nil {
			changeID = strings.TrimSpace(string(hashOutput))
		}

		snapshots = append(snapshots, SnapshotInfo{
			Name:     snapName,
			ChangeID: changeID,
		})
	}
	return snapshots, nil
}

// Ensure GitBackend implements contribution interfaces
var (
	_ injection.MountContributor  = (*GitBackend)(nil)
	_ injection.PromptContributor = (*GitBackend)(nil)
	_ Snapshotter                 = (*GitBackend)(nil)
)

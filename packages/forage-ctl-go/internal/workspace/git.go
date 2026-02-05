package workspace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

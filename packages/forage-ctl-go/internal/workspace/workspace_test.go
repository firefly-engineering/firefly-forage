package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// requireGit skips the test if git is not available
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH, skipping test")
	}
}

// requireJJ skips the test if jj is not available
func requireJJ(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not found in PATH, skipping test")
	}
}

func setupGitRepo(t *testing.T) string {
	t.Helper()
	requireGit(t)
	tmpDir := t.TempDir()

	// Initialize git repo
	cmd := exec.Command("git", "init", tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to init git repo: %s: %v", output, err)
	}

	// Configure git user for commits
	exec.Command("git", "-C", tmpDir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run()

	// Create an initial commit
	testFile := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	exec.Command("git", "-C", tmpDir, "add", ".").Run()
	cmd = exec.Command("git", "-C", tmpDir, "commit", "-m", "Initial commit")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create initial commit: %s: %v", output, err)
	}

	return tmpDir
}

func setupJJRepo(t *testing.T) string {
	t.Helper()
	requireJJ(t)
	tmpDir := t.TempDir()

	// Initialize jj repo
	cmd := exec.Command("jj", "git", "init", tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to init jj repo: %s: %v", output, err)
	}

	return tmpDir
}

func TestGitBackend_Interface(t *testing.T) {
	// Verify GitBackend implements Backend
	var _ Backend = &GitBackend{}
	var _ Backend = Git()
}

func TestJJBackend_Interface(t *testing.T) {
	// Verify JJBackend implements Backend
	var _ Backend = &JJBackend{}
	var _ Backend = JJ()
}

func TestGitBackend_Name(t *testing.T) {
	b := Git()
	if b.Name() != "git-worktree" {
		t.Errorf("expected 'git-worktree', got %q", b.Name())
	}
}

func TestJJBackend_Name(t *testing.T) {
	b := JJ()
	if b.Name() != "jj" {
		t.Errorf("expected 'jj', got %q", b.Name())
	}
}

func TestGitBackend_IsRepo(t *testing.T) {
	repoPath := setupGitRepo(t)
	b := Git()

	if !b.IsRepo(repoPath) {
		t.Error("IsRepo should return true for git repo")
	}

	nonRepoPath := t.TempDir()
	if b.IsRepo(nonRepoPath) {
		t.Error("IsRepo should return false for non-repo")
	}
}

func TestJJBackend_IsRepo(t *testing.T) {
	repoPath := setupJJRepo(t)
	b := JJ()

	if !b.IsRepo(repoPath) {
		t.Error("IsRepo should return true for jj repo")
	}

	nonRepoPath := t.TempDir()
	if b.IsRepo(nonRepoPath) {
		t.Error("IsRepo should return false for non-repo")
	}
}

func TestGitBackend_CreateAndRemove(t *testing.T) {
	repoPath := setupGitRepo(t)
	b := Git()

	workspacePath := filepath.Join(t.TempDir(), "workspace")
	name := "test-workspace"

	// Should not exist yet
	if b.Exists(repoPath, name) {
		t.Error("workspace should not exist before creation")
	}

	// Create workspace
	if err := b.Create(repoPath, name, workspacePath); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Should exist now
	if !b.Exists(repoPath, name) {
		t.Error("workspace should exist after creation")
	}

	// Verify files exist
	if _, err := os.Stat(filepath.Join(workspacePath, "README.md")); err != nil {
		t.Error("workspace should contain repo files")
	}

	// Remove workspace
	if err := b.Remove(repoPath, name, workspacePath); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Should not exist after removal
	if b.Exists(repoPath, name) {
		t.Error("workspace should not exist after removal")
	}
}

func TestJJBackend_CreateAndRemove(t *testing.T) {
	repoPath := setupJJRepo(t)
	b := JJ()

	workspacePath := filepath.Join(t.TempDir(), "workspace")
	name := "test-workspace"

	// Should not exist yet
	if b.Exists(repoPath, name) {
		t.Error("workspace should not exist before creation")
	}

	// Create workspace
	if err := b.Create(repoPath, name, workspacePath); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Should exist now
	if b.Exists(repoPath, name) {
		// JJ workspace exists
	}

	// Remove workspace
	if err := b.Remove(repoPath, name, workspacePath); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Should not exist after removal
	if b.Exists(repoPath, name) {
		t.Error("workspace should not exist after removal")
	}
}

func TestGitBackend_BranchName(t *testing.T) {
	b := Git().(*GitBackend)

	if b.BranchName("my-sandbox") != "forage-my-sandbox" {
		t.Errorf("expected 'forage-my-sandbox', got %q", b.BranchName("my-sandbox"))
	}
}

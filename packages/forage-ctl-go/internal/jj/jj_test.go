package jj

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsRepo(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()

	// Test non-repo directory
	if IsRepo(tmpDir) {
		t.Error("IsRepo should return false for non-repo directory")
	}

	// Create .jj/repo structure
	jjRepoPath := filepath.Join(tmpDir, ".jj", "repo")
	if err := os.MkdirAll(jjRepoPath, 0755); err != nil {
		t.Fatalf("Failed to create .jj/repo: %v", err)
	}

	// Test repo directory
	if !IsRepo(tmpDir) {
		t.Error("IsRepo should return true for directory with .jj/repo")
	}

	// Test nonexistent path
	if IsRepo("/nonexistent/path") {
		t.Error("IsRepo should return false for nonexistent path")
	}
}

func TestIsRepo_FileInsteadOfDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .jj as a file instead of directory
	jjPath := filepath.Join(tmpDir, ".jj")
	if err := os.WriteFile(jjPath, []byte("not a directory"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	if IsRepo(tmpDir) {
		t.Error("IsRepo should return false when .jj is a file")
	}
}

func TestIsRepo_EmptyJJDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .jj directory without repo subdirectory
	jjPath := filepath.Join(tmpDir, ".jj")
	if err := os.MkdirAll(jjPath, 0755); err != nil {
		t.Fatalf("Failed to create .jj: %v", err)
	}

	if IsRepo(tmpDir) {
		t.Error("IsRepo should return false when .jj/repo doesn't exist")
	}
}

// Note: The following tests would require jj to be installed and would be
// integration tests rather than unit tests. They're structured to be skipped
// if jj is not available.

func TestWorkspaceExists_Integration(t *testing.T) {
	// Skip if jj is not installed
	if _, err := os.Stat("/run/current-system/sw/bin/jj"); os.IsNotExist(err) {
		t.Skip("jj not installed, skipping integration test")
	}

	// This would need a real jj repo to test properly
	// For unit testing, we'd mock the exec.Command
	t.Skip("Integration test - requires real jj repo")
}

func TestCreateWorkspace_Integration(t *testing.T) {
	// Skip if jj is not installed
	if _, err := os.Stat("/run/current-system/sw/bin/jj"); os.IsNotExist(err) {
		t.Skip("jj not installed, skipping integration test")
	}

	t.Skip("Integration test - requires real jj repo")
}

func TestForgetWorkspace_Integration(t *testing.T) {
	// Skip if jj is not installed
	if _, err := os.Stat("/run/current-system/sw/bin/jj"); os.IsNotExist(err) {
		t.Skip("jj not installed, skipping integration test")
	}

	t.Skip("Integration test - requires real jj repo")
}

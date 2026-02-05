package jj

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// IsRepo checks if a path is a jj repository
func IsRepo(path string) bool {
	jjPath := filepath.Join(path, ".jj", "repo")
	info, err := os.Stat(jjPath)
	return err == nil && info.IsDir()
}

// WorkspaceExists checks if a workspace name already exists in the repo
func WorkspaceExists(repoPath, workspaceName string) bool {
	cmd := exec.Command("jj", "workspace", "list", "-R", repoPath)
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) > 0 && strings.TrimSpace(parts[0]) == workspaceName {
			return true
		}
	}
	return false
}

// CreateWorkspace creates a new jj workspace
func CreateWorkspace(repoPath, workspaceName, workspacePath string) error {
	cmd := exec.Command("jj", "workspace", "add", "-R", repoPath, "--name", workspaceName, workspacePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create jj workspace: %s: %w", string(output), err)
	}
	return nil
}

// ForgetWorkspace removes a workspace from jj
func ForgetWorkspace(repoPath, workspaceName string) error {
	cmd := exec.Command("jj", "workspace", "forget", workspaceName, "-R", repoPath)
	return cmd.Run()
}

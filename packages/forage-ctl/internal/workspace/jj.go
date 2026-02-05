package workspace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

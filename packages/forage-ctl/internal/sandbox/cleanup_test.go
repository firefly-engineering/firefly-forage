package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
)

func TestCleanup_RemovesPermissionsFiles(t *testing.T) {
	tmpDir := t.TempDir()
	sandboxesDir := filepath.Join(tmpDir, "sandboxes")
	os.MkdirAll(sandboxesDir, 0755)

	paths := &config.Paths{
		SandboxesDir:  sandboxesDir,
		SecretsDir:    filepath.Join(tmpDir, "secrets"),
		WorkspacesDir: filepath.Join(tmpDir, "workspaces"),
	}

	name := "test-sandbox"
	metadata := &config.SandboxMetadata{
		Name:          name,
		Template:      "claude",
		NetworkSlot:   1,
		Workspace:     "/tmp/workspace",
		WorkspaceMode: "direct",
	}

	// Create permissions files
	permFiles := []string{
		name + ".claude-permissions.json",
		name + ".copilot-permissions.json",
	}
	for _, f := range permFiles {
		path := filepath.Join(sandboxesDir, f)
		if err := os.WriteFile(path, []byte(`{}`), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", f, err)
		}
	}

	// Create a permissions file for a different sandbox (should NOT be removed)
	otherPerm := filepath.Join(sandboxesDir, "other.claude-permissions.json")
	if err := os.WriteFile(otherPerm, []byte(`{}`), 0644); err != nil {
		t.Fatalf("Failed to create other permissions file: %v", err)
	}

	// Run cleanup with only permissions enabled
	opts := CleanupOptions{
		CleanupPermissions: true,
	}
	Cleanup(metadata, paths, opts, nil)

	// Verify permissions files were removed
	for _, f := range permFiles {
		path := filepath.Join(sandboxesDir, f)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("permissions file %s should have been removed", f)
		}
	}

	// Verify other sandbox's permissions file was NOT removed
	if _, err := os.Stat(otherPerm); os.IsNotExist(err) {
		t.Error("other sandbox's permissions file should not have been removed")
	}
}

func TestDefaultCleanupOptions_IncludesPermissions(t *testing.T) {
	opts := DefaultCleanupOptions()
	if !opts.CleanupPermissions {
		t.Error("DefaultCleanupOptions should have CleanupPermissions = true")
	}
}

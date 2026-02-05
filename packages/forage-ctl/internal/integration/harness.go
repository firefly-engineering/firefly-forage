// Package integration provides a test harness for integration tests
// that require actual container runtime support.
//
// Integration tests are skipped unless the FORAGE_INTEGRATION_TESTS
// environment variable is set. These tests require:
// - NixOS with systemd-nspawn support
// - sudo access for extra-container
// - Available ports in the configured range
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/health"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
)

// TestHarness provides utilities for integration testing with real containers.
type TestHarness struct {
	t          *testing.T
	tempDir    string
	paths      *config.Paths
	hostConfig *config.HostConfig
	rt         runtime.Runtime
	sandboxes  []string // Track created sandboxes for cleanup
}

// NewHarness creates a new test harness.
// It will skip the test if FORAGE_INTEGRATION_TESTS is not set.
func NewHarness(t *testing.T) *TestHarness {
	t.Helper()

	if os.Getenv("FORAGE_INTEGRATION_TESTS") == "" {
		t.Skip("integration tests disabled (set FORAGE_INTEGRATION_TESTS=1 to enable)")
	}

	tempDir := t.TempDir()

	paths := &config.Paths{
		ConfigDir:     filepath.Join(tempDir, "config"),
		StateDir:      filepath.Join(tempDir, "state"),
		SecretsDir:    filepath.Join(tempDir, "secrets"),
		SandboxesDir:  filepath.Join(tempDir, "state", "sandboxes"),
		WorkspacesDir: filepath.Join(tempDir, "state", "workspaces"),
		TemplatesDir:  filepath.Join(tempDir, "config", "templates"),
	}

	// Create directories
	for _, dir := range []string{
		paths.ConfigDir,
		paths.StateDir,
		paths.SecretsDir,
		paths.SandboxesDir,
		paths.WorkspacesDir,
		paths.TemplatesDir,
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}

	// Try to detect the real runtime
	rt := runtime.Global()
	if rt == nil {
		t.Skip("no container runtime available")
	}

	// Load host config from system if available
	hostConfig, err := loadHostConfig()
	if err != nil {
		t.Skipf("failed to load host config: %v", err)
	}

	h := &TestHarness{
		t:          t,
		tempDir:    tempDir,
		paths:      paths,
		hostConfig: hostConfig,
		rt:         rt,
		sandboxes:  make([]string, 0),
	}

	t.Cleanup(h.Cleanup)

	return h
}

// loadHostConfig loads the host configuration from the default location.
func loadHostConfig() (*config.HostConfig, error) {
	paths := config.DefaultPaths()
	return config.LoadHostConfig(paths.ConfigDir)
}

// Paths returns the test paths.
func (h *TestHarness) Paths() *config.Paths {
	return h.paths
}

// HostConfig returns the host configuration.
func (h *TestHarness) HostConfig() *config.HostConfig {
	return h.hostConfig
}

// Runtime returns the container runtime.
func (h *TestHarness) Runtime() runtime.Runtime {
	return h.rt
}

// AddTemplate adds a template to the test environment.
func (h *TestHarness) AddTemplate(name string, template *config.Template) {
	h.t.Helper()

	if template.Name == "" {
		template.Name = name
	}

	data, err := json.MarshalIndent(template, "", "  ")
	if err != nil {
		h.t.Fatalf("Failed to marshal template: %v", err)
	}

	path := filepath.Join(h.paths.TemplatesDir, name+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		h.t.Fatalf("Failed to write template: %v", err)
	}
}

// CreateWorkspace creates a test workspace directory.
func (h *TestHarness) CreateWorkspace(name string) string {
	h.t.Helper()

	path := filepath.Join(h.tempDir, "workspaces", name)
	if err := os.MkdirAll(path, 0755); err != nil {
		h.t.Fatalf("Failed to create workspace: %v", err)
	}

	// Create a simple file to verify the workspace exists
	testFile := filepath.Join(path, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test Workspace\n"), 0644); err != nil {
		h.t.Fatalf("Failed to create test file: %v", err)
	}

	return path
}

// WaitForSSH waits for SSH to be ready on a sandbox.
func (h *TestHarness) WaitForSSH(port int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("SSH not ready after %v", timeout)
		case <-ticker.C:
			if health.CheckSSH(port) {
				return nil
			}
		}
	}
}

// TrackSandbox tracks a sandbox for cleanup.
func (h *TestHarness) TrackSandbox(name string) {
	h.sandboxes = append(h.sandboxes, name)
}

// Cleanup removes all created sandboxes and resources.
func (h *TestHarness) Cleanup() {
	ctx := context.Background()

	// Destroy all tracked sandboxes
	for _, name := range h.sandboxes {
		if err := h.rt.Destroy(ctx, name); err != nil {
			h.t.Logf("Warning: failed to destroy sandbox %s: %v", name, err)
		}
	}

	// Clean up metadata files
	for _, name := range h.sandboxes {
		metaPath := filepath.Join(h.paths.SandboxesDir, name+".json")
		os.Remove(metaPath)

		configPath := filepath.Join(h.paths.SandboxesDir, name+".nix")
		os.Remove(configPath)

		skillsPath := filepath.Join(h.paths.SandboxesDir, name+".skills.md")
		os.Remove(skillsPath)

		secretsPath := filepath.Join(h.paths.SecretsDir, name)
		os.RemoveAll(secretsPath)
	}
}

// RequireRunning skips the test if the named container is not running.
func (h *TestHarness) RequireRunning(name string) {
	h.t.Helper()

	running, err := h.rt.IsRunning(context.Background(), name)
	if err != nil {
		h.t.Skipf("failed to check if %s is running: %v", name, err)
	}
	if !running {
		h.t.Skipf("sandbox %s is not running", name)
	}
}

// GetSandboxMetadata loads sandbox metadata.
func (h *TestHarness) GetSandboxMetadata(name string) (*config.SandboxMetadata, error) {
	return config.LoadSandboxMetadata(h.paths.SandboxesDir, name)
}

// DefaultTemplate returns a basic template suitable for integration tests.
func DefaultTemplate() *config.Template {
	return &config.Template{
		Name:        "integration-test",
		Description: "Template for integration tests",
		Network:     "none", // Restrict network in tests
		Agents: map[string]config.AgentConfig{
			"test": {
				PackagePath: "pkgs.hello", // Use a minimal package
				SecretName:  "test-secret",
				AuthEnvVar:  "TEST_API_KEY",
			},
		},
	}
}

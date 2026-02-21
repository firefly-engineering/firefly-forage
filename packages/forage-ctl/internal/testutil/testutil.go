// Package testutil provides test utilities for integration tests
package testutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/app"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
)

// TestEnv holds the test environment
type TestEnv struct {
	T          *testing.T
	TmpDir     string
	Paths      *config.Paths
	HostConfig *config.HostConfig
	Runtime    *runtime.MockRuntime
	App        *app.App
	cleanup    func()
}

// NewTestEnv creates a new test environment with mock runtime
func NewTestEnv(t *testing.T) *TestEnv {
	t.Helper()

	tmpDir := t.TempDir()

	paths := &config.Paths{
		ConfigDir:     filepath.Join(tmpDir, "config"),
		StateDir:      filepath.Join(tmpDir, "state"),
		SecretsDir:    filepath.Join(tmpDir, "secrets"),
		SandboxesDir:  filepath.Join(tmpDir, "state", "sandboxes"),
		WorkspacesDir: filepath.Join(tmpDir, "state", "workspaces"),
		TemplatesDir:  filepath.Join(tmpDir, "config", "templates"),
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

	// Create a secret file for testing (simulates /run/secrets/anthropic-api-key)
	secretFile := filepath.Join(tmpDir, "secret-anthropic")
	if err := os.WriteFile(secretFile, []byte("sk-test-key"), 0600); err != nil {
		t.Fatalf("Failed to write test secret file: %v", err)
	}

	hostConfig := &config.HostConfig{
		User:               "testuser",
		UID:                1000,
		GID:                100,
		AuthorizedKeys:     []string{"ssh-rsa AAAA... test@test"},
		Secrets:            map[string]string{"anthropic": secretFile},
		StateDir:           paths.StateDir,
		ExtraContainerPath: "/nix/store/fake/extra-container",
		NixpkgsRev:         "abc123",
	}

	// Write host config
	configData, _ := json.MarshalIndent(hostConfig, "", "  ")
	if err := os.WriteFile(filepath.Join(paths.ConfigDir, "config.json"), configData, 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	mockRuntime := runtime.NewMockRuntime()

	testApp := app.New(
		app.WithPaths(paths),
		app.WithRuntime(mockRuntime),
		app.WithHostConfig(hostConfig),
	)

	// Save original default and set test app
	originalDefault := app.Default
	app.SetDefault(testApp)

	env := &TestEnv{
		T:          t,
		TmpDir:     tmpDir,
		Paths:      paths,
		HostConfig: hostConfig,
		Runtime:    mockRuntime,
		App:        testApp,
		cleanup: func() {
			app.SetDefault(originalDefault)
		},
	}

	return env
}

// Cleanup restores the original app default
func (e *TestEnv) Cleanup() {
	if e.cleanup != nil {
		e.cleanup()
	}
}

// AddTemplate adds a template to the test environment
func (e *TestEnv) AddTemplate(name string, template *config.Template) {
	e.T.Helper()

	if template.Name == "" {
		template.Name = name
	}

	data, err := json.MarshalIndent(template, "", "  ")
	if err != nil {
		e.T.Fatalf("Failed to marshal template: %v", err)
	}

	path := filepath.Join(e.Paths.TemplatesDir, name+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		e.T.Fatalf("Failed to write template: %v", err)
	}
}

// AddSandbox adds a sandbox to the test environment
func (e *TestEnv) AddSandbox(metadata *config.SandboxMetadata) {
	e.T.Helper()

	if err := config.SaveSandboxMetadata(e.Paths.SandboxesDir, metadata); err != nil {
		e.T.Fatalf("Failed to save sandbox metadata: %v", err)
	}

	// Also add to mock runtime if running
	if metadata.NetworkSlot > 0 {
		e.Runtime.AddContainer(metadata.Name, runtime.StatusRunning)
	}
}

// CreateWorkspace creates a workspace directory
func (e *TestEnv) CreateWorkspace(name string) string {
	e.T.Helper()

	path := filepath.Join(e.TmpDir, "workspaces", name)
	if err := os.MkdirAll(path, 0755); err != nil {
		e.T.Fatalf("Failed to create workspace: %v", err)
	}
	return path
}

// CreateJJRepo creates a fake JJ repository
func (e *TestEnv) CreateJJRepo(name string) string {
	e.T.Helper()

	path := filepath.Join(e.TmpDir, "repos", name)
	jjPath := filepath.Join(path, ".jj", "repo")
	if err := os.MkdirAll(jjPath, 0755); err != nil {
		e.T.Fatalf("Failed to create JJ repo: %v", err)
	}
	return path
}

// GetSandbox loads a sandbox metadata
func (e *TestEnv) GetSandbox(name string) *config.SandboxMetadata {
	e.T.Helper()

	metadata, err := config.LoadSandboxMetadata(e.Paths.SandboxesDir, name)
	if err != nil {
		return nil
	}
	return metadata
}

// SandboxExists checks if a sandbox exists
func (e *TestEnv) SandboxExists(name string) bool {
	return config.SandboxExists(e.Paths.SandboxesDir, name)
}

// DefaultTemplate returns a basic template for testing
func DefaultTemplate() *config.Template {
	return &config.Template{
		Name:        "test",
		Description: "Test template",
		Network:     "full",
		Agents: map[string]config.AgentConfig{
			"claude": {
				PackagePath: "pkgs.claude-code",
				SecretName:  "anthropic",
				AuthEnvVar:  "ANTHROPIC_API_KEY",
			},
		},
	}
}

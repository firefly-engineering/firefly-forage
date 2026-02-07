package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/testutil"
)

func TestCreator_Create_InvalidName(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	env.AddTemplate("test", testutil.DefaultTemplate())

	creator := &Creator{
		paths:      env.Paths,
		hostConfig: env.HostConfig,
		rt:         env.Runtime,
	}

	// Test invalid sandbox names
	invalidNames := []string{
		"",                  // empty
		"../escape",         // path traversal
		"My-Project",        // uppercase
		"has spaces",        // spaces
		"-starts-with-dash", // starts with dash
		"has;semicolon",     // special characters
	}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			_, err := creator.Create(context.Background(), CreateOptions{
				Name:     name,
				Template: "test",
				RepoPath: env.TmpDir,
				Direct:   true,
			})
			if err == nil {
				t.Errorf("Create(%q) should have failed with invalid name", name)
			}
		})
	}
}

func TestCreator_Create_ValidName(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	// IMPORTANT: Set mock runtime as global runtime so runtime.Create() uses it
	runtime.SetGlobal(env.Runtime)
	defer runtime.SetGlobal(nil)

	env.AddTemplate("test", testutil.DefaultTemplate())

	// Create a workspace directory
	workspacePath := env.CreateWorkspace("myproject")

	creator := &Creator{
		paths:      env.Paths,
		hostConfig: env.HostConfig,
		rt:         env.Runtime,
	}

	// Test valid sandbox name
	result, err := creator.Create(context.Background(), CreateOptions{
		Name:     "myproject",
		Template: "test",
		RepoPath: workspacePath,
		Direct:   true,
	})

	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	if result.Name != "myproject" {
		t.Errorf("Name = %q, want %q", result.Name, "myproject")
	}

	// Verify ContainerIP is derived from NetworkSlot
	if result.ContainerIP == "" {
		t.Error("ContainerIP should not be empty")
	}
	if result.Metadata.NetworkSlot < 1 || result.Metadata.NetworkSlot > 254 {
		t.Errorf("NetworkSlot %d not in valid range [1, 254]",
			result.Metadata.NetworkSlot)
	}

	// Verify sandbox metadata was saved
	if !env.SandboxExists("myproject") {
		t.Error("Sandbox metadata was not saved")
	}

	// Verify runtime.Create was called
	if _, exists := env.Runtime.Containers["myproject"]; !exists {
		t.Error("Container was not created via runtime")
	}
}

func TestCreator_Create_DuplicateName(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	env.AddTemplate("test", testutil.DefaultTemplate())

	// Create an existing sandbox
	env.AddSandbox(&config.SandboxMetadata{
		Name:        "existing",
		Template:    "test",
		NetworkSlot: 1,
	})

	workspacePath := env.CreateWorkspace("existing")

	creator := &Creator{
		paths:      env.Paths,
		hostConfig: env.HostConfig,
		rt:         env.Runtime,
	}

	_, err := creator.Create(context.Background(), CreateOptions{
		Name:     "existing",
		Template: "test",
		RepoPath: workspacePath,
		Direct:   true,
	})

	if err == nil {
		t.Error("Create() should have failed for duplicate name")
	}
}

func TestCreator_Create_MissingTemplate(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	workspacePath := env.CreateWorkspace("myproject")

	creator := &Creator{
		paths:      env.Paths,
		hostConfig: env.HostConfig,
		rt:         env.Runtime,
	}

	_, err := creator.Create(context.Background(), CreateOptions{
		Name:     "myproject",
		Template: "nonexistent",
		RepoPath: workspacePath,
		Direct:   true,
	})

	if err == nil {
		t.Error("Create() should have failed for missing template")
	}
}

func TestCreator_Create_MissingWorkspace(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	env.AddTemplate("test", testutil.DefaultTemplate())

	creator := &Creator{
		paths:      env.Paths,
		hostConfig: env.HostConfig,
		rt:         env.Runtime,
	}

	_, err := creator.Create(context.Background(), CreateOptions{
		Name:     "myproject",
		Template: "test",
		RepoPath: "/nonexistent/workspace",
		Direct:   true,
	})

	if err == nil {
		t.Error("Create() should have failed for missing workspace")
	}
}

func TestCreator_setupSecrets(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	creator := &Creator{
		paths:      env.Paths,
		hostConfig: env.HostConfig,
		rt:         env.Runtime,
	}

	template := testutil.DefaultTemplate()
	secretsPath := filepath.Join(env.TmpDir, "test-secrets")

	err := creator.setupSecrets(secretsPath, template)
	if err != nil {
		t.Fatalf("setupSecrets() failed: %v", err)
	}

	// Verify secrets directory was created
	if _, statErr := os.Stat(secretsPath); os.IsNotExist(statErr) {
		t.Error("Secrets directory was not created")
	}

	// Verify secret file was created with correct permissions
	secretFile := filepath.Join(secretsPath, "anthropic")
	info, err := os.Stat(secretFile)
	if os.IsNotExist(err) {
		t.Error("Secret file was not created")
	} else if info.Mode().Perm() != 0600 {
		t.Errorf("Secret file permissions = %o, want %o", info.Mode().Perm(), 0600)
	}

	// Verify secret content
	content, err := os.ReadFile(secretFile)
	if err != nil {
		t.Fatalf("Failed to read secret file: %v", err)
	}
	if string(content) != "sk-test-key" {
		t.Errorf("Secret content = %q, want %q", string(content), "sk-test-key")
	}
}

func TestCreator_setupSecrets_MissingSecret(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	// Remove the secret from host config
	env.HostConfig.Secrets = map[string]string{}

	creator := &Creator{
		paths:      env.Paths,
		hostConfig: env.HostConfig,
		rt:         env.Runtime,
	}

	template := testutil.DefaultTemplate()
	secretsPath := filepath.Join(env.TmpDir, "test-secrets")

	// Should not fail, just skip the missing secret
	err := creator.setupSecrets(secretsPath, template)
	if err != nil {
		t.Fatalf("setupSecrets() should not fail for missing secret: %v", err)
	}

	// Secret file should not exist
	secretFile := filepath.Join(secretsPath, "anthropic")
	if _, err := os.Stat(secretFile); !os.IsNotExist(err) {
		t.Error("Secret file should not exist when secret is missing from config")
	}
}

func TestCreator_cleanup(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	env.AddTemplate("test", testutil.DefaultTemplate())

	creator := &Creator{
		paths:      env.Paths,
		hostConfig: env.HostConfig,
		rt:         env.Runtime,
	}

	// Create some resources that cleanup should remove
	metadata := &config.SandboxMetadata{
		Name:        "cleanup-test",
		Template:    "test",
		NetworkSlot: 1,
		Workspace:   filepath.Join(env.TmpDir, "workspace"),
	}

	// Save metadata
	config.SaveSandboxMetadata(env.Paths.SandboxesDir, metadata)

	// Create secrets directory
	secretsPath := filepath.Join(env.Paths.SecretsDir, "cleanup-test")
	os.MkdirAll(secretsPath, 0700)
	os.WriteFile(filepath.Join(secretsPath, "test-secret"), []byte("secret"), 0600)

	// Create config file
	configPath := filepath.Join(env.Paths.SandboxesDir, "cleanup-test.nix")
	os.WriteFile(configPath, []byte("# nix config"), 0644)

	// Add container to mock runtime
	env.Runtime.AddContainer("cleanup-test", runtime.StatusRunning)

	// Run cleanup
	creator.cleanup(metadata)

	// Verify resources were cleaned up
	if env.SandboxExists("cleanup-test") {
		t.Error("Sandbox metadata was not cleaned up")
	}

	if _, err := os.Stat(secretsPath); !os.IsNotExist(err) {
		t.Error("Secrets directory was not cleaned up")
	}

	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Error("Config file was not cleaned up")
	}
}

func TestWorkspaceBackendFor(t *testing.T) {
	tests := []struct {
		mode     WorkspaceMode
		wantName string
		wantNil  bool
	}{
		{WorkspaceModeJJ, "jj", false},
		{WorkspaceModeGitWorktree, "git-worktree", false},
		{WorkspaceModeDirect, "", true},
		{"", "", true},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			backend := workspaceBackendFor(tt.mode)
			if tt.wantNil {
				if backend != nil {
					t.Errorf("workspaceBackendFor(%q) = %v, want nil", tt.mode, backend)
				}
			} else {
				if backend == nil {
					t.Errorf("workspaceBackendFor(%q) = nil, want non-nil", tt.mode)
				} else if backend.Name() != tt.wantName {
					t.Errorf("workspaceBackendFor(%q).Name() = %q, want %q",
						tt.mode, backend.Name(), tt.wantName)
				}
			}
		})
	}
}

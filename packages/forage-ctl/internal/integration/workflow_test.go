// Package integration provides workflow tests that exercise complete code paths
// without requiring actual container infrastructure.
//
// These tests verify that all components work together correctly:
// - Config loading and validation
// - Port allocation across sandboxes
// - Workspace setup
// - Nix config generation
// - Metadata persistence
// - Cleanup operations
package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/generator"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/injection"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/port"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/reproducibility"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/sandbox"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/testutil"
)

// testContributions creates a minimal set of contributions for testing.
func testContributions(workspacePath, secretsPath string) *injection.Contributions {
	return &injection.Contributions{
		Mounts: []injection.Mount{
			{HostPath: "/nix/store", ContainerPath: "/nix/store", ReadOnly: true},
			{HostPath: workspacePath, ContainerPath: "/workspace", ReadOnly: false},
			{HostPath: secretsPath, ContainerPath: "/run/secrets", ReadOnly: true},
		},
		TmpfilesRules: []string{
			"d /home/agent/.config 0755 agent users -",
		},
	}
}

// TestWorkflow_CreateSandboxWithDirectWorkspace tests creating a sandbox
// with a direct workspace path (no jj or git-worktree).
func TestWorkflow_CreateSandboxWithDirectWorkspace(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	// Set up test template
	env.AddTemplate("test-template", &config.Template{
		Name:        "test-template",
		Description: "Test template",
		Network:     "full",
		Agents: map[string]config.AgentConfig{
			"test-agent": {
				PackagePath: "pkgs.hello",
				SecretName:  "test-secret",
				AuthEnvVar:  "TEST_KEY",
			},
		},
	})

	// Create a workspace directory
	workspacePath := env.CreateWorkspace("test-project")
	// Add a file to simulate a real project
	if err := os.WriteFile(filepath.Join(workspacePath, "README.md"), []byte("# Test"), 0644); err != nil {
		t.Fatalf("failed to create README: %v", err)
	}

	// Test slot allocation
	networkSlot, err := port.AllocateSlot(nil)
	if err != nil {
		t.Fatalf("slot allocation failed: %v", err)
	}
	if networkSlot < 1 || networkSlot > 254 {
		t.Errorf("network slot %d outside valid range", networkSlot)
	}

	// Test config generation
	template, err := config.LoadTemplate(env.Paths.TemplatesDir, "test-template")
	if err != nil {
		t.Fatalf("failed to load template: %v", err)
	}

	secretsPath := filepath.Join(env.Paths.SecretsDir, "test-sandbox")
	containerCfg := &generator.ContainerConfig{
		Name:            "test-sandbox",
		NetworkSlot:     networkSlot,
		AuthorizedKeys:  []string{"ssh-rsa AAAA... test@test"},
		Template:        template,
		UID:             env.HostConfig.UID,
		GID:             env.HostConfig.GID,
		Contributions:   testContributions(workspacePath, secretsPath),
		Reproducibility: reproducibility.NewNixReproducibility(),
	}

	nixConfig, err := generator.GenerateNixConfig(containerCfg)
	if err != nil {
		t.Fatalf("config generation failed: %v", err)
	}

	// Verify generated config contains expected elements
	checks := []string{
		"containers.forage-test-sandbox",
		workspacePath,
		"ssh-rsa AAAA",
	}
	for _, check := range checks {
		if !strings.Contains(nixConfig, check) {
			t.Errorf("generated config missing: %s", check)
		}
	}

	// Test metadata persistence
	metadata := &config.SandboxMetadata{
		Name:          "test-sandbox",
		Template:      "test-template",
		Workspace:     workspacePath,
		NetworkSlot:   networkSlot,
		WorkspaceMode: "direct",
		CreatedAt:     "2024-01-01T00:00:00Z",
	}

	if err := config.SaveSandboxMetadata(env.Paths.SandboxesDir, metadata); err != nil {
		t.Fatalf("failed to save metadata: %v", err)
	}

	// Verify we can load it back
	loaded, err := config.LoadSandboxMetadata(env.Paths.SandboxesDir, "test-sandbox")
	if err != nil {
		t.Fatalf("failed to load metadata: %v", err)
	}
	if loaded.NetworkSlot != networkSlot {
		t.Errorf("loaded NetworkSlot = %d, want %d", loaded.NetworkSlot, networkSlot)
	}
}

// TestWorkflow_MultipleSandboxesSlotAllocation tests that slot allocation
// works correctly across multiple sandboxes.
func TestWorkflow_MultipleSandboxesSlotAllocation(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	// Create multiple sandboxes and track slots
	var sandboxes []*config.SandboxMetadata
	usedSlots := make(map[int]bool)

	for i := 0; i < 5; i++ {
		slot, err := port.AllocateSlot(sandboxes)
		if err != nil {
			t.Fatalf("allocation %d failed: %v", i, err)
		}

		if usedSlots[slot] {
			t.Errorf("slot %d allocated twice", slot)
		}
		usedSlots[slot] = true

		meta := &config.SandboxMetadata{
			Name:        "sandbox-" + string(rune('a'+i)),
			NetworkSlot: slot,
		}
		sandboxes = append(sandboxes, meta)

		// Save to disk
		if err := config.SaveSandboxMetadata(env.Paths.SandboxesDir, meta); err != nil {
			t.Fatalf("failed to save metadata: %v", err)
		}
	}

	// Verify all sandboxes are listed
	listed, err := config.ListSandboxes(env.Paths.SandboxesDir)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(listed) != 5 {
		t.Errorf("expected 5 sandboxes, got %d", len(listed))
	}
}

// TestWorkflow_CleanupRemovesAllArtifacts tests that cleanup properly
// removes all sandbox artifacts.
func TestWorkflow_CleanupRemovesAllArtifacts(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	sandboxName := "cleanup-test"

	// Create sandbox artifacts manually
	metadataPath := filepath.Join(env.Paths.SandboxesDir, sandboxName+".json")
	configPath := filepath.Join(env.Paths.SandboxesDir, sandboxName+".nix")
	skillsPath := filepath.Join(env.Paths.SandboxesDir, sandboxName+".skills.md")
	secretsPath := filepath.Join(env.Paths.SecretsDir, sandboxName)

	// Create the files
	if err := os.WriteFile(metadataPath, []byte(`{"name":"cleanup-test"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("{ }"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillsPath, []byte("# Skills"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(secretsPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secretsPath, "api-key"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}

	// Verify files exist
	for _, path := range []string{metadataPath, configPath, skillsPath, secretsPath} {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Fatalf("expected %s to exist before cleanup", path)
		}
	}

	// Run cleanup (with nil runtime since we're not testing container cleanup)
	metadata := &config.SandboxMetadata{
		Name:     sandboxName,
		Template: "test",
	}
	sandbox.Cleanup(metadata, env.Paths, sandbox.CleanupOptions{
		DestroyContainer: false, // No container to destroy
		CleanupWorkspace: false, // No VCS workspace
		CleanupSecrets:   true,
		CleanupConfig:    true,
		CleanupSkills:    true,
		CleanupMetadata:  true,
	}, nil)

	// Verify files are removed
	for _, path := range []string{metadataPath, configPath, skillsPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed after cleanup", path)
		}
	}
	if _, err := os.Stat(secretsPath); !os.IsNotExist(err) {
		t.Errorf("expected secrets dir to be removed after cleanup")
	}
}

// TestWorkflow_NetworkModeConfigs tests that different network modes
// generate correct configurations.
func TestWorkflow_NetworkModeConfigs(t *testing.T) {
	tests := []struct {
		name         string
		networkMode  string
		allowedHosts []string
		wantContains []string
	}{
		{
			name:         "full network",
			networkMode:  "full",
			wantContains: []string{"defaultGateway", "nameservers"},
		},
		{
			name:         "no network",
			networkMode:  "none",
			wantContains: []string{"defaultGateway = null", "nameservers = []"},
		},
		{
			name:         "restricted network",
			networkMode:  "restricted",
			allowedHosts: []string{"api.anthropic.com", "github.com"},
			wantContains: []string{"nftables", "api.anthropic.com", "github.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			template := &config.Template{
				Name:         "test",
				Network:      tt.networkMode,
				AllowedHosts: tt.allowedHosts,
				Agents: map[string]config.AgentConfig{
					"test": {PackagePath: "pkgs.hello", SecretName: "test", AuthEnvVar: "TEST_KEY"},
				},
			}

			cfg := &generator.ContainerConfig{
				Name:            "test",
				NetworkSlot:     1,
				AuthorizedKeys:  []string{"ssh-rsa AAAA..."},
				Template:        template,
				UID:             1000,
				GID:             100,
				Contributions:   testContributions("/workspace", "/secrets"),
				Reproducibility: reproducibility.NewNixReproducibility(),
			}

			result, err := generator.GenerateNixConfig(cfg)
			if err != nil {
				t.Fatalf("generation failed: %v", err)
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("config should contain %q for network mode %s", want, tt.networkMode)
				}
			}
		})
	}
}

// TestWorkflow_SandboxNameValidation tests that invalid sandbox names
// are properly rejected throughout the workflow.
func TestWorkflow_SandboxNameValidation(t *testing.T) {
	invalidNames := []string{
		"../escape",
		"has spaces",
		"Has-Uppercase",
		"-starts-dash",
		"has;semicolon",
		"has\nnewline",
		"",
	}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			err := config.ValidateSandboxName(name)
			if err == nil {
				t.Errorf("expected validation error for %q", name)
			}
		})
	}

	validNames := []string{
		"my-project",
		"test123",
		"sandbox_1",
		"a",
	}

	for _, name := range validNames {
		t.Run(name, func(t *testing.T) {
			err := config.ValidateSandboxName(name)
			if err != nil {
				t.Errorf("unexpected validation error for %q: %v", name, err)
			}
		})
	}
}

// TestWorkflow_RuntimeMockIntegration tests that the mock runtime
// correctly simulates container operations.
func TestWorkflow_RuntimeMockIntegration(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	ctx := context.Background()

	// Initially, no containers should be running
	running, err := env.Runtime.IsRunning(ctx, "test-sandbox")
	if err != nil {
		t.Fatalf("IsRunning failed: %v", err)
	}
	if running {
		t.Error("sandbox should not be running initially")
	}

	// Start a container
	env.Runtime.AddContainer("test-sandbox", runtime.StatusRunning)

	running, err = env.Runtime.IsRunning(ctx, "test-sandbox")
	if err != nil {
		t.Fatalf("IsRunning failed: %v", err)
	}
	if !running {
		t.Error("sandbox should be running after creation")
	}

	// Stop the container
	if err := env.Runtime.Stop(ctx, "test-sandbox"); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	running, err = env.Runtime.IsRunning(ctx, "test-sandbox")
	if err != nil {
		t.Fatalf("IsRunning failed: %v", err)
	}
	if running {
		t.Error("sandbox should not be running after stop")
	}
}

// TestWorkflow_TemplateValidation tests template loading and validation.
func TestWorkflow_TemplateValidation(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	// Add a valid template
	env.AddTemplate("valid", &config.Template{
		Name:        "valid",
		Description: "Valid template",
		Network:     "full",
		Agents: map[string]config.AgentConfig{
			"test": {PackagePath: "pkgs.hello", SecretName: "test", AuthEnvVar: "TEST_KEY"},
		},
	})

	// Load it
	tmpl, err := config.LoadTemplate(env.Paths.TemplatesDir, "valid")
	if err != nil {
		t.Fatalf("failed to load valid template: %v", err)
	}
	if tmpl.Name != "valid" {
		t.Errorf("template name = %q, want %q", tmpl.Name, "valid")
	}

	// Try to load non-existent template
	_, err = config.LoadTemplate(env.Paths.TemplatesDir, "nonexistent")
	if err == nil {
		t.Error("expected error loading nonexistent template")
	}
}

// TestWorkflow_SkillsGeneration tests that skills are generated correctly
// for different project types.
func TestWorkflow_SkillsGeneration(t *testing.T) {
	metadata := &config.SandboxMetadata{
		Name:          "test-sandbox",
		Template:      "claude",
		WorkspaceMode: "direct",
	}

	template := &config.Template{
		Name:    "claude",
		Network: "full",
		Agents: map[string]config.AgentConfig{
			"claude": {AuthEnvVar: "ANTHROPIC_API_KEY"},
		},
	}

	skills := generator.GenerateSkills(metadata, template)

	// Verify skills content
	if !strings.Contains(skills, "test-sandbox") {
		t.Error("skills should contain sandbox name")
	}
	if !strings.Contains(skills, "claude") {
		t.Error("skills should contain agent name")
	}
	if !strings.Contains(skills, "/workspace") {
		t.Error("skills should mention workspace")
	}
}

// TestWorkflow_JJModeConfig tests JJ workspace mode configuration.
func TestWorkflow_JJModeConfig(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	// Create a fake JJ repo
	repoPath := env.CreateJJRepo("test-repo")

	template := &config.Template{
		Name:    "test",
		Network: "full",
		Agents: map[string]config.AgentConfig{
			"test": {PackagePath: "pkgs.hello", SecretName: "test", AuthEnvVar: "TEST_KEY"},
		},
	}

	workspacePath := filepath.Join(env.TmpDir, "workspaces", "jj-sandbox")
	secretsPath := filepath.Join(env.Paths.SecretsDir, "jj-sandbox")
	jjPath := filepath.Join(repoPath, ".jj")

	// Create contributions with JJ mount
	contributions := testContributions(workspacePath, secretsPath)
	contributions.Mounts = append(contributions.Mounts, injection.Mount{
		HostPath:      jjPath,
		ContainerPath: jjPath,
		ReadOnly:      false,
	})

	cfg := &generator.ContainerConfig{
		Name:            "jj-sandbox",
		NetworkSlot:     1,
		AuthorizedKeys:  []string{"ssh-rsa AAAA..."},
		Template:        template,
		UID:             env.HostConfig.UID,
		GID:             env.HostConfig.GID,
		Contributions:   contributions,
		Reproducibility: reproducibility.NewNixReproducibility(),
	}

	nixConfig, err := generator.GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("config generation failed: %v", err)
	}

	// JJ mode should include .jj bind mount
	if !strings.Contains(nixConfig, ".jj") {
		t.Error("JJ mode config should contain .jj bind mount")
	}
}

// TestWorkflow_ProxyModeConfig tests proxy mode configuration.
func TestWorkflow_ProxyModeConfig(t *testing.T) {
	template := &config.Template{
		Name:    "test",
		Network: "full",
		Agents: map[string]config.AgentConfig{
			"claude": {
				PackagePath: "pkgs.claude-code",
				SecretName:  "anthropic",
				AuthEnvVar:  "ANTHROPIC_API_KEY",
			},
		},
	}

	// Create contributions with proxy env vars (instead of secret reading)
	contributions := testContributions("/workspace", "/secrets")
	contributions.EnvVars = []injection.EnvVar{
		{Name: "ANTHROPIC_BASE_URL", Value: `"http://10.100.1.1:8080"`},
		{Name: "ANTHROPIC_CUSTOM_HEADERS", Value: `"X-Forage-Sandbox: proxy-sandbox"`},
	}

	cfg := &generator.ContainerConfig{
		Name:            "proxy-sandbox",
		NetworkSlot:     1,
		AuthorizedKeys:  []string{"ssh-rsa AAAA..."},
		Template:        template,
		UID:             1000,
		GID:             100,
		Contributions:   contributions,
		Reproducibility: reproducibility.NewNixReproducibility(),
	}

	nixConfig, err := generator.GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("config generation failed: %v", err)
	}

	// Proxy mode should include base URL and headers
	if !strings.Contains(nixConfig, "ANTHROPIC_BASE_URL") {
		t.Error("proxy mode config should contain ANTHROPIC_BASE_URL")
	}
	if !strings.Contains(nixConfig, "X-Forage-Sandbox") {
		t.Error("proxy mode config should contain X-Forage-Sandbox header")
	}
	// Should NOT contain direct secret reading
	if strings.Contains(nixConfig, "cat /run/secrets/anthropic") {
		t.Error("proxy mode should not read secrets directly")
	}
}

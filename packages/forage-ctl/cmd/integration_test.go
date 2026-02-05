package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
)

// These tests verify the business logic of commands
// They use file-based state to verify behavior

func TestTemplatesCommand_ListsTemplates(t *testing.T) {
	env := setupTestEnv(t)

	// Add some templates with all required fields
	env.addTemplate(t, "claude", &config.Template{
		Name:        "claude",
		Description: "Claude Code sandbox",
		Network:     "full",
		Agents: map[string]config.AgentConfig{
			"claude": {
				PackagePath: "/nix/store/test-claude",
				SecretName:  "anthropic",
				AuthEnvVar:  "ANTHROPIC_API_KEY",
			},
		},
	})

	env.addTemplate(t, "multi", &config.Template{
		Name:        "multi",
		Description: "Multi-agent sandbox",
		Network:     "full",
		Agents: map[string]config.AgentConfig{
			"claude": {
				PackagePath: "/nix/store/test-claude",
				SecretName:  "anthropic",
				AuthEnvVar:  "ANTHROPIC_API_KEY",
			},
			"opencode": {
				PackagePath: "/nix/store/test-opencode",
				SecretName:  "openai",
				AuthEnvVar:  "OPENAI_API_KEY",
			},
		},
	})

	// Verify templates can be loaded
	templates, err := config.ListTemplates(filepath.Join(env.configDir, "templates"))
	if err != nil {
		t.Fatalf("Failed to list templates: %v", err)
	}

	if len(templates) != 2 {
		t.Errorf("Expected 2 templates, got %d", len(templates))
	}
}

func TestSandboxMetadata_Lifecycle(t *testing.T) {
	env := setupTestEnv(t)
	sandboxesDir := filepath.Join(env.stateDir, "sandboxes")

	// Create metadata
	meta := &config.SandboxMetadata{
		Name:          "test-sandbox",
		Template:      "claude",
		Port:          2200,
		Workspace:     "/home/user/project",
		NetworkSlot:   1,
		CreatedAt:     "2024-01-01T00:00:00Z",
		WorkspaceMode: "direct",
	}

	// Save
	if err := config.SaveSandboxMetadata(sandboxesDir, meta); err != nil {
		t.Fatalf("Failed to save metadata: %v", err)
	}

	// Verify exists
	if !config.SandboxExists(sandboxesDir, "test-sandbox") {
		t.Error("Sandbox should exist after save")
	}

	// Load
	loaded, err := config.LoadSandboxMetadata(sandboxesDir, "test-sandbox")
	if err != nil {
		t.Fatalf("Failed to load metadata: %v", err)
	}

	if loaded.Name != meta.Name {
		t.Errorf("Name = %q, want %q", loaded.Name, meta.Name)
	}
	if loaded.Port != meta.Port {
		t.Errorf("Port = %d, want %d", loaded.Port, meta.Port)
	}

	// Delete
	if err := config.DeleteSandboxMetadata(sandboxesDir, "test-sandbox"); err != nil {
		t.Fatalf("Failed to delete metadata: %v", err)
	}

	// Verify gone
	if config.SandboxExists(sandboxesDir, "test-sandbox") {
		t.Error("Sandbox should not exist after delete")
	}
}

func TestSandboxMetadata_JJMode(t *testing.T) {
	env := setupTestEnv(t)
	sandboxesDir := filepath.Join(env.stateDir, "sandboxes")

	meta := &config.SandboxMetadata{
		Name:            "jj-sandbox",
		Template:        "claude",
		Port:            2201,
		Workspace:       "/var/lib/forage/workspaces/jj-sandbox",
		NetworkSlot:     2,
		CreatedAt:       "2024-01-01T00:00:00Z",
		WorkspaceMode:   "jj",
		SourceRepo:      "/home/user/myrepo",
		JJWorkspaceName: "jj-sandbox",
	}

	if err := config.SaveSandboxMetadata(sandboxesDir, meta); err != nil {
		t.Fatalf("Failed to save metadata: %v", err)
	}

	loaded, err := config.LoadSandboxMetadata(sandboxesDir, "jj-sandbox")
	if err != nil {
		t.Fatalf("Failed to load metadata: %v", err)
	}

	if loaded.WorkspaceMode != "jj" {
		t.Errorf("WorkspaceMode = %q, want %q", loaded.WorkspaceMode, "jj")
	}
	if loaded.SourceRepo != meta.SourceRepo {
		t.Errorf("SourceRepo = %q, want %q", loaded.SourceRepo, meta.SourceRepo)
	}
	if loaded.JJWorkspaceName != meta.JJWorkspaceName {
		t.Errorf("JJWorkspaceName = %q, want %q", loaded.JJWorkspaceName, meta.JJWorkspaceName)
	}
}

func TestListSandboxes_MultipleStates(t *testing.T) {
	env := setupTestEnv(t)
	sandboxesDir := filepath.Join(env.stateDir, "sandboxes")

	// Create multiple sandboxes
	sandboxes := []*config.SandboxMetadata{
		{Name: "sandbox-1", Template: "claude", Port: 2200, NetworkSlot: 1},
		{Name: "sandbox-2", Template: "multi", Port: 2201, NetworkSlot: 2},
		{Name: "sandbox-3", Template: "claude", Port: 2202, NetworkSlot: 3},
	}

	for _, sb := range sandboxes {
		if err := config.SaveSandboxMetadata(sandboxesDir, sb); err != nil {
			t.Fatalf("Failed to save sandbox %s: %v", sb.Name, err)
		}
	}

	// List all
	loaded, err := config.ListSandboxes(sandboxesDir)
	if err != nil {
		t.Fatalf("Failed to list sandboxes: %v", err)
	}

	if len(loaded) != 3 {
		t.Errorf("Expected 3 sandboxes, got %d", len(loaded))
	}
}

func TestHostConfig_Loading(t *testing.T) {
	env := setupTestEnv(t)

	cfg, err := config.LoadHostConfig(env.configDir)
	if err != nil {
		t.Fatalf("Failed to load host config: %v", err)
	}

	if cfg.User != "testuser" {
		t.Errorf("User = %q, want %q", cfg.User, "testuser")
	}

	if cfg.PortRange.From != 2200 {
		t.Errorf("PortRange.From = %d, want %d", cfg.PortRange.From, 2200)
	}

	if cfg.PortRange.To != 2299 {
		t.Errorf("PortRange.To = %d, want %d", cfg.PortRange.To, 2299)
	}

	if cfg.NixpkgsRev != "test123" {
		t.Errorf("NixpkgsRev = %q, want %q", cfg.NixpkgsRev, "test123")
	}
}

func TestHostConfig_Secrets(t *testing.T) {
	env := setupTestEnv(t)

	cfg, err := config.LoadHostConfig(env.configDir)
	if err != nil {
		t.Fatalf("Failed to load host config: %v", err)
	}

	secret, ok := cfg.Secrets["anthropic"]
	if !ok {
		t.Error("Secret 'anthropic' should exist")
	}

	if secret != "sk-test" {
		t.Errorf("Secret = %q, want %q", secret, "sk-test")
	}
}

func TestTemplate_Loading(t *testing.T) {
	env := setupTestEnv(t)

	env.addTemplate(t, "test-template", &config.Template{
		Name:        "test-template",
		Description: "Test template for testing",
		Network:     "restricted",
		AllowedHosts: []string{
			"api.anthropic.com",
			"github.com",
		},
		Agents: map[string]config.AgentConfig{
			"claude": {
				PackagePath: "pkgs.claude-code",
				SecretName:  "anthropic",
				AuthEnvVar:  "ANTHROPIC_API_KEY",
			},
		},
		ExtraPackages: []string{"ripgrep", "fd"},
	})

	tmpl, err := config.LoadTemplate(filepath.Join(env.configDir, "templates"), "test-template")
	if err != nil {
		t.Fatalf("Failed to load template: %v", err)
	}

	if tmpl.Network != "restricted" {
		t.Errorf("Network = %q, want %q", tmpl.Network, "restricted")
	}

	if len(tmpl.AllowedHosts) != 2 {
		t.Errorf("len(AllowedHosts) = %d, want 2", len(tmpl.AllowedHosts))
	}

	if len(tmpl.Agents) != 1 {
		t.Errorf("len(Agents) = %d, want 1", len(tmpl.Agents))
	}

	agent, ok := tmpl.Agents["claude"]
	if !ok {
		t.Fatal("Agent 'claude' not found")
	}

	if agent.AuthEnvVar != "ANTHROPIC_API_KEY" {
		t.Errorf("AuthEnvVar = %q, want %q", agent.AuthEnvVar, "ANTHROPIC_API_KEY")
	}
}

func TestContainerName(t *testing.T) {
	tests := []struct {
		sandbox string
		want    string
	}{
		{"myproject", "forage-myproject"},
		{"test-123", "forage-test-123"},
		{"sandbox_with_underscore", "forage-sandbox_with_underscore"},
	}

	for _, tt := range tests {
		t.Run(tt.sandbox, func(t *testing.T) {
			got := config.ContainerName(tt.sandbox)
			if got != tt.want {
				t.Errorf("ContainerName(%q) = %q, want %q", tt.sandbox, got, tt.want)
			}
		})
	}
}

func TestSecretsDirectory_Setup(t *testing.T) {
	env := setupTestEnv(t)

	// Simulate creating secrets for a sandbox
	sandboxName := "test-sandbox"
	secretsPath := filepath.Join(env.secretsDir, sandboxName)

	if err := os.MkdirAll(secretsPath, 0700); err != nil {
		t.Fatalf("Failed to create secrets dir: %v", err)
	}

	// Write a secret
	secretFile := filepath.Join(secretsPath, "anthropic")
	if err := os.WriteFile(secretFile, []byte("sk-secret-key"), 0600); err != nil {
		t.Fatalf("Failed to write secret: %v", err)
	}

	// Verify permissions
	info, err := os.Stat(secretsPath)
	if err != nil {
		t.Fatalf("Failed to stat secrets dir: %v", err)
	}

	if info.Mode().Perm() != 0700 {
		t.Errorf("Secrets dir permissions = %o, want %o", info.Mode().Perm(), 0700)
	}

	// Read secret back
	data, err := os.ReadFile(secretFile)
	if err != nil {
		t.Fatalf("Failed to read secret: %v", err)
	}

	if string(data) != "sk-secret-key" {
		t.Errorf("Secret content = %q, want %q", string(data), "sk-secret-key")
	}
}

func TestNixConfigFile_Creation(t *testing.T) {
	env := setupTestEnv(t)

	// Simulate creating a nix config file for a sandbox
	sandboxName := "test-sandbox"
	configPath := filepath.Join(env.stateDir, "sandboxes", sandboxName+".nix")

	nixConfig := `{ pkgs, ... }: {
  containers.forage-test-sandbox = {
    autoStart = true;
  };
}`

	if err := os.WriteFile(configPath, []byte(nixConfig), 0644); err != nil {
		t.Fatalf("Failed to write nix config: %v", err)
	}

	// Verify file exists and content
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read nix config: %v", err)
	}

	if string(data) != nixConfig {
		t.Error("Nix config content mismatch")
	}
}

func TestSkillsFile_Creation(t *testing.T) {
	env := setupTestEnv(t)

	sandboxName := "test-sandbox"
	skillsPath := filepath.Join(env.stateDir, "sandboxes", sandboxName+".skills.md")

	skillsContent := `# Agent Instructions

You are running in a sandboxed environment.

## Workspace
Your workspace is at /workspace.
`

	if err := os.WriteFile(skillsPath, []byte(skillsContent), 0644); err != nil {
		t.Fatalf("Failed to write skills file: %v", err)
	}

	data, err := os.ReadFile(skillsPath)
	if err != nil {
		t.Fatalf("Failed to read skills file: %v", err)
	}

	if string(data) != skillsContent {
		t.Error("Skills content mismatch")
	}
}

func TestWorkspaceDirectory_Validation(t *testing.T) {
	env := setupTestEnv(t)

	// Non-existent workspace
	nonExistent := filepath.Join(env.tmpDir, "nonexistent")
	_, err := os.Stat(nonExistent)
	if !os.IsNotExist(err) {
		t.Error("Non-existent path should not exist")
	}

	// Create workspace
	workspace := env.createWorkspace(t, "valid-project")

	info, err := os.Stat(workspace)
	if err != nil {
		t.Fatalf("Failed to stat workspace: %v", err)
	}

	if !info.IsDir() {
		t.Error("Workspace should be a directory")
	}
}

func TestJJRepo_Detection(t *testing.T) {
	env := setupTestEnv(t)

	// Not a JJ repo
	notRepo := env.createWorkspace(t, "not-a-repo")
	jjPath := filepath.Join(notRepo, ".jj", "repo")
	_, err := os.Stat(jjPath)
	if !os.IsNotExist(err) {
		t.Error(".jj/repo should not exist in non-repo")
	}

	// Create fake JJ repo
	repo := filepath.Join(env.tmpDir, "real-repo")
	repoJJPath := filepath.Join(repo, ".jj", "repo")
	if err = os.MkdirAll(repoJJPath, 0755); err != nil {
		t.Fatalf("Failed to create .jj/repo: %v", err)
	}

	info, err := os.Stat(repoJJPath)
	if err != nil {
		t.Fatalf("Failed to stat .jj/repo: %v", err)
	}

	if !info.IsDir() {
		t.Error(".jj/repo should be a directory")
	}
}

func TestMultipleTemplates_DifferentNetworkModes(t *testing.T) {
	env := setupTestEnv(t)

	templates := []struct {
		name    string
		network string
	}{
		{"full-network", "full"},
		{"no-network", "none"},
		{"restricted-network", "restricted"},
	}

	for _, tt := range templates {
		env.addTemplate(t, tt.name, &config.Template{
			Name:    tt.name,
			Network: tt.network,
			Agents: map[string]config.AgentConfig{
				"test": {
					PackagePath: "/nix/store/test-agent",
					SecretName:  "test-secret",
					AuthEnvVar:  "TEST_API_KEY",
				},
			},
		})
	}

	// Load and verify each
	for _, tt := range templates {
		tmpl, err := config.LoadTemplate(filepath.Join(env.configDir, "templates"), tt.name)
		if err != nil {
			t.Fatalf("Failed to load template %s: %v", tt.name, err)
		}

		if tmpl.Network != tt.network {
			t.Errorf("Template %s: Network = %q, want %q", tt.name, tmpl.Network, tt.network)
		}
	}
}

func TestSandboxMetadata_JSON_Format(t *testing.T) {
	env := setupTestEnv(t)
	sandboxesDir := filepath.Join(env.stateDir, "sandboxes")

	meta := &config.SandboxMetadata{
		Name:          "json-test",
		Template:      "claude",
		Port:          2200,
		Workspace:     "/workspace",
		NetworkSlot:   1,
		CreatedAt:     "2024-01-01T00:00:00Z",
		WorkspaceMode: "direct",
	}

	if err := config.SaveSandboxMetadata(sandboxesDir, meta); err != nil {
		t.Fatalf("Failed to save metadata: %v", err)
	}

	// Read raw JSON
	path := filepath.Join(sandboxesDir, "json-test.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read JSON: %v", err)
	}

	// Verify it's valid JSON
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	// Check expected fields
	if raw["name"] != "json-test" {
		t.Errorf("name = %v, want %q", raw["name"], "json-test")
	}

	// Port should be a number
	if port, ok := raw["port"].(float64); !ok || int(port) != 2200 {
		t.Errorf("port = %v, want 2200", raw["port"])
	}
}

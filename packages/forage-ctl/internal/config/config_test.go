package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPaths(t *testing.T) {
	paths := DefaultPaths()

	if paths.ConfigDir != DefaultConfigDir {
		t.Errorf("ConfigDir = %q, want %q", paths.ConfigDir, DefaultConfigDir)
	}
	if paths.StateDir != DefaultStateDir {
		t.Errorf("StateDir = %q, want %q", paths.StateDir, DefaultStateDir)
	}
	if paths.SecretsDir != DefaultSecretsDir {
		t.Errorf("SecretsDir = %q, want %q", paths.SecretsDir, DefaultSecretsDir)
	}
	if paths.SandboxesDir != filepath.Join(DefaultStateDir, "sandboxes") {
		t.Errorf("SandboxesDir = %q, want %q", paths.SandboxesDir, filepath.Join(DefaultStateDir, "sandboxes"))
	}
	if paths.WorkspacesDir != filepath.Join(DefaultStateDir, "workspaces") {
		t.Errorf("WorkspacesDir = %q, want %q", paths.WorkspacesDir, filepath.Join(DefaultStateDir, "workspaces"))
	}
	if paths.TemplatesDir != filepath.Join(DefaultConfigDir, "templates") {
		t.Errorf("TemplatesDir = %q, want %q", paths.TemplatesDir, filepath.Join(DefaultConfigDir, "templates"))
	}
}

func TestContainerName(t *testing.T) {
	tests := []struct {
		sandboxName string
		want        string
	}{
		{"myproject", "forage-myproject"},
		{"test-123", "forage-test-123"},
		{"", "forage-"},
	}

	for _, tt := range tests {
		t.Run(tt.sandboxName, func(t *testing.T) {
			got := ContainerName(tt.sandboxName)
			if got != tt.want {
				t.Errorf("ContainerName(%q) = %q, want %q", tt.sandboxName, got, tt.want)
			}
		})
	}
}

func TestLoadHostConfig(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	// Create a test config file
	config := HostConfig{
		User: "testuser",
		PortRange: PortRange{
			From: 2200,
			To:   2299,
		},
		AuthorizedKeys:     []string{"ssh-rsa AAAA..."},
		Secrets:            map[string]string{"anthropic": "sk-test"},
		StateDir:           "/var/lib/forage",
		ExtraContainerPath: "/nix/store/.../extra-container",
		NixpkgsRev:         "abc123",
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Test loading the config
	loaded, err := LoadHostConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadHostConfig failed: %v", err)
	}

	if loaded.User != config.User {
		t.Errorf("User = %q, want %q", loaded.User, config.User)
	}
	if loaded.PortRange.From != config.PortRange.From {
		t.Errorf("PortRange.From = %d, want %d", loaded.PortRange.From, config.PortRange.From)
	}
	if loaded.PortRange.To != config.PortRange.To {
		t.Errorf("PortRange.To = %d, want %d", loaded.PortRange.To, config.PortRange.To)
	}
	if loaded.NixpkgsRev != config.NixpkgsRev {
		t.Errorf("NixpkgsRev = %q, want %q", loaded.NixpkgsRev, config.NixpkgsRev)
	}
}

func TestLoadHostConfig_NotFound(t *testing.T) {
	_, err := LoadHostConfig("/nonexistent/path")
	if err == nil {
		t.Error("Expected error for nonexistent config, got nil")
	}
}

func TestLoadHostConfig_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	if err := os.WriteFile(configPath, []byte("not valid json"), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	_, err := LoadHostConfig(tmpDir)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestLoadTemplate(t *testing.T) {
	tmpDir := t.TempDir()

	template := Template{
		Name:        "claude",
		Description: "Claude Code sandbox",
		Network:     "full",
		Agents: map[string]AgentConfig{
			"claude": {
				PackagePath: "pkgs.claude-code",
				SecretName:  "anthropic",
				AuthEnvVar:  "ANTHROPIC_API_KEY",
			},
		},
		ExtraPackages: []string{"ripgrep", "fd"},
	}

	data, err := json.MarshalIndent(template, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal template: %v", err)
	}

	templatePath := filepath.Join(tmpDir, "claude.json")
	if err := os.WriteFile(templatePath, data, 0644); err != nil {
		t.Fatalf("Failed to write template: %v", err)
	}

	loaded, err := LoadTemplate(tmpDir, "claude")
	if err != nil {
		t.Fatalf("LoadTemplate failed: %v", err)
	}

	if loaded.Name != template.Name {
		t.Errorf("Name = %q, want %q", loaded.Name, template.Name)
	}
	if loaded.Network != template.Network {
		t.Errorf("Network = %q, want %q", loaded.Network, template.Network)
	}
	if len(loaded.Agents) != 1 {
		t.Errorf("len(Agents) = %d, want 1", len(loaded.Agents))
	}
	if agent, ok := loaded.Agents["claude"]; !ok {
		t.Error("Agent 'claude' not found")
	} else if agent.AuthEnvVar != "ANTHROPIC_API_KEY" {
		t.Errorf("AuthEnvVar = %q, want %q", agent.AuthEnvVar, "ANTHROPIC_API_KEY")
	}
}

func TestLoadTemplate_NotFound(t *testing.T) {
	_, err := LoadTemplate("/nonexistent", "missing")
	if err == nil {
		t.Error("Expected error for nonexistent template, got nil")
	}
}

func TestListTemplates(t *testing.T) {
	tmpDir := t.TempDir()

	// Create two valid templates (must have agents with all required fields)
	templates := []Template{
		{
			Name:        "claude",
			Description: "Claude sandbox",
			Agents: map[string]AgentConfig{
				"claude": {PackagePath: "claude-code", SecretName: "anthropic-api-key", AuthEnvVar: "ANTHROPIC_API_KEY"},
			},
		},
		{
			Name:        "multi",
			Description: "Multi-agent sandbox",
			Agents: map[string]AgentConfig{
				"agent1": {PackagePath: "agent1", SecretName: "key1", AuthEnvVar: "API_KEY"},
			},
		},
	}

	for _, tmpl := range templates {
		data, _ := json.MarshalIndent(tmpl, "", "  ")
		path := filepath.Join(tmpDir, tmpl.Name+".json")
		os.WriteFile(path, data, 0644)
	}

	// Create a non-json file (should be ignored)
	os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("ignore me"), 0644)

	// Create a directory (should be ignored)
	os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755)

	loaded, err := ListTemplates(tmpDir)
	if err != nil {
		t.Fatalf("ListTemplates failed: %v", err)
	}

	if len(loaded) != 2 {
		t.Errorf("len(loaded) = %d, want 2", len(loaded))
	}
}

func TestSandboxMetadata(t *testing.T) {
	tmpDir := t.TempDir()

	metadata := &SandboxMetadata{
		Name:            "test-sandbox",
		Template:        "claude",
		Port:            2200,
		Workspace:       "/home/user/project",
		NetworkSlot:     1,
		CreatedAt:       "2024-01-01T00:00:00Z",
		WorkspaceMode:   "jj",
		SourceRepo:      "/home/user/repo",
		JJWorkspaceName: "test-sandbox",
	}

	// Test save
	if err := SaveSandboxMetadata(tmpDir, metadata); err != nil {
		t.Fatalf("SaveSandboxMetadata failed: %v", err)
	}

	// Verify file exists
	metaPath := filepath.Join(tmpDir, "test-sandbox.json")
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		t.Error("Metadata file was not created")
	}

	// Test load
	loaded, err := LoadSandboxMetadata(tmpDir, "test-sandbox")
	if err != nil {
		t.Fatalf("LoadSandboxMetadata failed: %v", err)
	}

	if loaded.Name != metadata.Name {
		t.Errorf("Name = %q, want %q", loaded.Name, metadata.Name)
	}
	if loaded.Port != metadata.Port {
		t.Errorf("Port = %d, want %d", loaded.Port, metadata.Port)
	}
	if loaded.WorkspaceMode != metadata.WorkspaceMode {
		t.Errorf("WorkspaceMode = %q, want %q", loaded.WorkspaceMode, metadata.WorkspaceMode)
	}

	// Test exists
	if !SandboxExists(tmpDir, "test-sandbox") {
		t.Error("SandboxExists returned false for existing sandbox")
	}
	if SandboxExists(tmpDir, "nonexistent") {
		t.Error("SandboxExists returned true for nonexistent sandbox")
	}

	// Test delete
	if err := DeleteSandboxMetadata(tmpDir, "test-sandbox"); err != nil {
		t.Fatalf("DeleteSandboxMetadata failed: %v", err)
	}

	if SandboxExists(tmpDir, "test-sandbox") {
		t.Error("Sandbox still exists after delete")
	}
}

func TestLoadSandboxMetadata_DefaultWorkspaceMode(t *testing.T) {
	tmpDir := t.TempDir()

	// Create metadata without WorkspaceMode (simulating old format)
	data := `{"name": "old-sandbox", "template": "claude", "port": 2200}`
	metaPath := filepath.Join(tmpDir, "old-sandbox.json")
	if err := os.WriteFile(metaPath, []byte(data), 0644); err != nil {
		t.Fatalf("Failed to write metadata: %v", err)
	}

	loaded, err := LoadSandboxMetadata(tmpDir, "old-sandbox")
	if err != nil {
		t.Fatalf("LoadSandboxMetadata failed: %v", err)
	}

	if loaded.WorkspaceMode != "direct" {
		t.Errorf("WorkspaceMode = %q, want %q (default)", loaded.WorkspaceMode, "direct")
	}
}

func TestListSandboxes(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some sandbox metadata
	sandboxes := []*SandboxMetadata{
		{Name: "sandbox-1", Template: "claude", Port: 2200},
		{Name: "sandbox-2", Template: "multi", Port: 2201},
	}

	for _, sb := range sandboxes {
		SaveSandboxMetadata(tmpDir, sb)
	}

	// Create a non-json file (should be ignored)
	os.WriteFile(filepath.Join(tmpDir, "notes.txt"), []byte("ignore"), 0644)

	loaded, err := ListSandboxes(tmpDir)
	if err != nil {
		t.Fatalf("ListSandboxes failed: %v", err)
	}

	if len(loaded) != 2 {
		t.Errorf("len(loaded) = %d, want 2", len(loaded))
	}
}

func TestListSandboxes_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	loaded, err := ListSandboxes(tmpDir)
	if err != nil {
		t.Fatalf("ListSandboxes failed: %v", err)
	}

	if len(loaded) != 0 {
		t.Errorf("len(loaded) = %d, want 0", len(loaded))
	}
}

func TestListSandboxes_NonexistentDir(t *testing.T) {
	loaded, err := ListSandboxes("/nonexistent/path")
	if err != nil {
		t.Fatalf("ListSandboxes should not error for nonexistent dir: %v", err)
	}

	if loaded != nil {
		t.Errorf("loaded = %v, want nil", loaded)
	}
}

func TestSafePath(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		base    string
		fname   string
		suffix  string
		wantErr bool
	}{
		{"valid name", tmpDir, "sandbox1", ".json", false},
		{"valid with dash", tmpDir, "my-sandbox", ".json", false},
		{"path traversal", tmpDir, "../escape", ".json", true},
		{"deep traversal", tmpDir, "../../etc/passwd", "", true},
		{"absolute escape", tmpDir, "/etc/passwd", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := safePath(tt.base, tt.fname, tt.suffix)
			if (err != nil) != tt.wantErr {
				t.Errorf("safePath(%q, %q, %q) error = %v, wantErr %v",
					tt.base, tt.fname, tt.suffix, err, tt.wantErr)
			}
		})
	}
}

func TestLoadSandboxMetadata_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()

	// Attempt to load with path traversal attack
	_, err := LoadSandboxMetadata(tmpDir, "../../../etc/passwd")
	if err == nil {
		t.Error("Expected error for path traversal, got nil")
	}
}

func TestValidateSandboxName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		// Valid names
		{"myproject", false},
		{"my-project", false},
		{"my_project", false},
		{"project123", false},
		{"123project", false},
		{"a", false},
		{"a-b-c", false},
		{"test_sandbox_1", false},

		// Invalid names
		{"", true},                             // empty
		{"My-Project", true},                   // uppercase
		{"my project", true},                   // space
		{"../../../etc/passwd", true},          // path traversal
		{"/absolute/path", true},               // absolute path
		{"my.project", true},                   // dots
		{"-starts-with-dash", true},            // starts with dash
		{"_starts_with_underscore", true},      // starts with underscore
		{"has@special", true},                  // special characters
		{"has$dollar", true},                   // special characters
		{"has;semicolon", true},                // injection attempt
		{"a" + string(make([]byte, 64)), true}, // too long (64+ chars)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSandboxName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSandboxName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

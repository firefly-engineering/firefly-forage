package config

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestContainerNameForSlot(t *testing.T) {
	tests := []struct {
		slot int
		want string
	}{
		{1, "f1"},
		{42, "f42"},
		{254, "f254"},
	}

	for _, tt := range tests {
		got := ContainerNameForSlot(tt.slot)
		if got != tt.want {
			t.Errorf("ContainerNameForSlot(%d) = %q, want %q", tt.slot, got, tt.want)
		}
	}
}

func TestResolvedContainerName(t *testing.T) {
	// New sandbox with ContainerName set
	meta := &SandboxMetadata{Name: "review", ContainerName: "f5"}
	if got := meta.ResolvedContainerName(); got != "f5" {
		t.Errorf("ResolvedContainerName() = %q, want %q", got, "f5")
	}

	// Legacy sandbox without ContainerName
	legacy := &SandboxMetadata{Name: "review"}
	if got := legacy.ResolvedContainerName(); got != "forage-review" {
		t.Errorf("ResolvedContainerName() = %q, want %q", got, "forage-review")
	}
}

func TestLoadHostConfig(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	// Create a test config file
	config := HostConfig{
		User:               "testuser",
		UID:                1000,
		GID:                100,
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
	if err = os.WriteFile(configPath, data, 0644); err != nil {
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
	if err = os.WriteFile(templatePath, data, 0644); err != nil {
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
	if loaded.NetworkSlot != metadata.NetworkSlot {
		t.Errorf("NetworkSlot = %d, want %d", loaded.NetworkSlot, metadata.NetworkSlot)
	}
	if loaded.WorkspaceMode != metadata.WorkspaceMode {
		t.Errorf("WorkspaceMode = %q, want %q", loaded.WorkspaceMode, metadata.WorkspaceMode)
	}

	// Test ContainerIP
	expectedIP := "10.100.1.2"
	if loaded.ContainerIP() != expectedIP {
		t.Errorf("ContainerIP() = %q, want %q", loaded.ContainerIP(), expectedIP)
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
	data := `{"name": "old-sandbox", "template": "claude", "networkSlot": 1}`
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
		{Name: "sandbox-1", Template: "claude", NetworkSlot: 1},
		{Name: "sandbox-2", Template: "multi", NetworkSlot: 2},
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

func TestListSandboxes_SkipsPermissionsFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid sandbox metadata
	SaveSandboxMetadata(tmpDir, &SandboxMetadata{
		Name: "test", Template: "claude", NetworkSlot: 1, Workspace: "/w",
	})

	// Create permissions files that should be skipped
	os.WriteFile(filepath.Join(tmpDir, "test.claude-permissions.json"), []byte(`{}`), 0644)
	os.WriteFile(filepath.Join(tmpDir, "test.copilot-permissions.json"), []byte(`{}`), 0644)

	// Create another dotted JSON file that should be skipped
	os.WriteFile(filepath.Join(tmpDir, "some.other.json"), []byte(`{}`), 0644)

	loaded, err := ListSandboxes(tmpDir)
	if err != nil {
		t.Fatalf("ListSandboxes failed: %v", err)
	}

	if len(loaded) != 1 {
		t.Errorf("len(loaded) = %d, want 1 (permissions files should be skipped)", len(loaded))
	}
	if len(loaded) > 0 && loaded[0].Name != "test" {
		t.Errorf("loaded[0].Name = %q, want %q", loaded[0].Name, "test")
	}
}

func TestIsSandboxMetadataFile(t *testing.T) {
	tests := []struct {
		filename string
		want     bool
	}{
		{"sandbox-1.json", true},
		{"my-project.json", true},
		{"test.claude-permissions.json", false},
		{"test.copilot-permissions.json", false},
		{"some.other.json", false},
		{"notes.txt", false},
		{"readme.md", false},
		{".json", true}, // edge case: empty name before .json
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := IsSandboxMetadataFile(tt.filename)
			if got != tt.want {
				t.Errorf("IsSandboxMetadataFile(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
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

	// Valid names should resolve within the base directory
	for _, tt := range []struct {
		name   string
		fname  string
		suffix string
	}{
		{"valid name", "sandbox1", ".json"},
		{"valid with dash", "my-sandbox", ".json"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err := safePath(tmpDir, tt.fname, tt.suffix)
			if err != nil {
				t.Errorf("safePath(%q, %q, %q) unexpected error: %v", tmpDir, tt.fname, tt.suffix, err)
			}
			if !strings.HasPrefix(result, tmpDir) {
				t.Errorf("safePath result %q escapes base %q", result, tmpDir)
			}
		})
	}

	// Traversal attempts must be rejected
	for _, tt := range []struct {
		name   string
		fname  string
		suffix string
	}{
		{"path traversal", "../escape", ".json"},
		{"deep traversal", "../../etc/passwd", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err := safePath(tmpDir, tt.fname, tt.suffix)
			if err != nil {
				return // error is expected
			}
			// If no error, the result must still be contained within the base
			if !strings.HasPrefix(result, tmpDir) {
				t.Errorf("safePath(%q, %q, %q) = %q escapes base directory", tmpDir, tt.fname, tt.suffix, result)
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

func TestAgentConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		agent   AgentConfig
		wantErr string
	}{
		{
			name: "valid basic config",
			agent: AgentConfig{
				PackagePath: "/nix/store/abc-claude",
				SecretName:  "anthropic",
				AuthEnvVar:  "ANTHROPIC_API_KEY",
			},
			wantErr: "",
		},
		{
			name: "valid with host config dir",
			agent: AgentConfig{
				PackagePath:        "/nix/store/abc-claude",
				SecretName:         "anthropic",
				AuthEnvVar:         "ANTHROPIC_API_KEY",
				HostConfigDir:      "/home/user/.claude",
				ContainerConfigDir: "/home/agent/.claude",
			},
			wantErr: "",
		},
		{
			name: "valid with read-only config dir",
			agent: AgentConfig{
				PackagePath:           "/nix/store/abc-claude",
				SecretName:            "anthropic",
				AuthEnvVar:            "ANTHROPIC_API_KEY",
				HostConfigDir:         "/home/user/.claude",
				ContainerConfigDir:    "/home/agent/.claude",
				HostConfigDirReadOnly: true,
			},
			wantErr: "",
		},
		{
			name: "valid credential mount only (no secret)",
			agent: AgentConfig{
				PackagePath:        "/nix/store/abc-claude",
				HostConfigDir:      "/home/user/.claude",
				ContainerConfigDir: "/home/agent/.claude",
			},
			wantErr: "",
		},
		{
			name: "missing packagePath",
			agent: AgentConfig{
				SecretName: "anthropic",
				AuthEnvVar: "ANTHROPIC_API_KEY",
			},
			wantErr: "packagePath is required",
		},
		{
			name: "no auth method",
			agent: AgentConfig{
				PackagePath: "/nix/store/abc-claude",
			},
			wantErr: "either secretName/authEnvVar or hostConfigDir is required",
		},
		{
			name: "secretName without authEnvVar",
			agent: AgentConfig{
				PackagePath: "/nix/store/abc-claude",
				SecretName:  "anthropic",
			},
			wantErr: "secretName and authEnvVar must both be set or both be empty",
		},
		{
			name: "authEnvVar without secretName",
			agent: AgentConfig{
				PackagePath: "/nix/store/abc-claude",
				AuthEnvVar:  "ANTHROPIC_API_KEY",
			},
			wantErr: "secretName and authEnvVar must both be set or both be empty",
		},
		{
			name: "relative hostConfigDir",
			agent: AgentConfig{
				PackagePath:        "/nix/store/abc-claude",
				SecretName:         "anthropic",
				AuthEnvVar:         "ANTHROPIC_API_KEY",
				HostConfigDir:      ".claude",
				ContainerConfigDir: "/home/agent/.claude",
			},
			wantErr: "hostConfigDir must be an absolute path",
		},
		{
			name: "relative containerConfigDir",
			agent: AgentConfig{
				PackagePath:        "/nix/store/abc-claude",
				SecretName:         "anthropic",
				AuthEnvVar:         "ANTHROPIC_API_KEY",
				HostConfigDir:      "/home/user/.claude",
				ContainerConfigDir: ".claude",
			},
			wantErr: "containerConfigDir must be an absolute path",
		},
		{
			name: "hostConfigDir without containerConfigDir",
			agent: AgentConfig{
				PackagePath:   "/nix/store/abc-claude",
				SecretName:    "anthropic",
				AuthEnvVar:    "ANTHROPIC_API_KEY",
				HostConfigDir: "/home/user/.claude",
			},
			wantErr: "containerConfigDir is required when hostConfigDir is set",
		},
		{
			name: "valid permissions skipAll",
			agent: AgentConfig{
				PackagePath: "/nix/store/abc-claude",
				SecretName:  "anthropic",
				AuthEnvVar:  "ANTHROPIC_API_KEY",
				Permissions: &AgentPermissions{SkipAll: true},
			},
			wantErr: "",
		},
		{
			name: "valid permissions allow and deny",
			agent: AgentConfig{
				PackagePath: "/nix/store/abc-claude",
				SecretName:  "anthropic",
				AuthEnvVar:  "ANTHROPIC_API_KEY",
				Permissions: &AgentPermissions{
					Allow: []string{"Read", "Glob"},
					Deny:  []string{"Bash(rm -rf *)"},
				},
			},
			wantErr: "",
		},
		{
			name: "valid nil permissions",
			agent: AgentConfig{
				PackagePath: "/nix/store/abc-claude",
				SecretName:  "anthropic",
				AuthEnvVar:  "ANTHROPIC_API_KEY",
			},
			wantErr: "",
		},
		{
			name: "invalid skipAll with allow",
			agent: AgentConfig{
				PackagePath: "/nix/store/abc-claude",
				SecretName:  "anthropic",
				AuthEnvVar:  "ANTHROPIC_API_KEY",
				Permissions: &AgentPermissions{
					SkipAll: true,
					Allow:   []string{"Read"},
				},
			},
			wantErr: "permissions: skipAll cannot be combined with allow or deny",
		},
		{
			name: "valid permissions with hostConfigDir",
			agent: AgentConfig{
				PackagePath:        "/nix/store/abc-claude",
				SecretName:         "anthropic",
				AuthEnvVar:         "ANTHROPIC_API_KEY",
				HostConfigDir:      "/home/user/.claude",
				ContainerConfigDir: "/home/agent/.claude",
				Permissions:        &AgentPermissions{SkipAll: true},
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.agent.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Validate() error = %q, want containing %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestAgentIdentity_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()

	metadata := &SandboxMetadata{
		Name:        "identity-test",
		Template:    "claude",
		Workspace:   "/workspace",
		NetworkSlot: 1,
		AgentIdentity: &AgentIdentity{
			GitUser:    "Agent Bot",
			GitEmail:   "agent@example.com",
			SSHKeyPath: "/run/secrets/agent-key",
		},
	}

	if err := SaveSandboxMetadata(tmpDir, metadata); err != nil {
		t.Fatalf("SaveSandboxMetadata failed: %v", err)
	}

	loaded, err := LoadSandboxMetadata(tmpDir, "identity-test")
	if err != nil {
		t.Fatalf("LoadSandboxMetadata failed: %v", err)
	}

	if loaded.AgentIdentity == nil {
		t.Fatal("AgentIdentity should not be nil after round-trip")
	}
	if loaded.AgentIdentity.GitUser != "Agent Bot" {
		t.Errorf("GitUser = %q, want %q", loaded.AgentIdentity.GitUser, "Agent Bot")
	}
	if loaded.AgentIdentity.GitEmail != "agent@example.com" {
		t.Errorf("GitEmail = %q, want %q", loaded.AgentIdentity.GitEmail, "agent@example.com")
	}
	if loaded.AgentIdentity.SSHKeyPath != "/run/secrets/agent-key" {
		t.Errorf("SSHKeyPath = %q, want %q", loaded.AgentIdentity.SSHKeyPath, "/run/secrets/agent-key")
	}
}

func TestAgentIdentity_BackwardCompat(t *testing.T) {
	tmpDir := t.TempDir()

	// JSON without agentIdentity (old format)
	data := `{"name": "old-sandbox", "template": "claude", "networkSlot": 1, "workspace": "/w"}`
	metaPath := filepath.Join(tmpDir, "old-sandbox.json")
	if err := os.WriteFile(metaPath, []byte(data), 0644); err != nil {
		t.Fatalf("Failed to write metadata: %v", err)
	}

	loaded, err := LoadSandboxMetadata(tmpDir, "old-sandbox")
	if err != nil {
		t.Fatalf("LoadSandboxMetadata failed: %v", err)
	}

	if loaded.AgentIdentity != nil {
		t.Error("AgentIdentity should be nil for old format without identity")
	}
}

func TestAgentIdentity_NilOmittedInJSON(t *testing.T) {
	metadata := &SandboxMetadata{
		Name:        "no-identity",
		Template:    "claude",
		Workspace:   "/w",
		NetworkSlot: 1,
	}

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	if strings.Contains(string(data), "agentIdentity") {
		t.Error("nil AgentIdentity should be omitted from JSON")
	}
}

func TestValidateAgentIdentity(t *testing.T) {
	// Create temp files for SSH key tests
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "id_ed25519")
	pubPath := keyPath + ".pub"
	os.WriteFile(keyPath, []byte("private-key"), 0600)
	os.WriteFile(pubPath, []byte("ssh-ed25519 AAAA..."), 0644)

	tests := []struct {
		name    string
		id      *AgentIdentity
		wantErr string
	}{
		{
			name:    "nil identity",
			id:      nil,
			wantErr: "",
		},
		{
			name:    "git only (no SSH key)",
			id:      &AgentIdentity{GitUser: "Agent", GitEmail: "a@b.com"},
			wantErr: "",
		},
		{
			name:    "empty identity",
			id:      &AgentIdentity{},
			wantErr: "",
		},
		{
			name:    "valid SSH key",
			id:      &AgentIdentity{SSHKeyPath: keyPath},
			wantErr: "",
		},
		{
			name:    "relative SSH path",
			id:      &AgentIdentity{SSHKeyPath: "relative/path"},
			wantErr: "sshKeyPath must be an absolute path",
		},
		{
			name:    "nonexistent SSH key",
			id:      &AgentIdentity{SSHKeyPath: "/nonexistent/key"},
			wantErr: "sshKeyPath \"/nonexistent/key\"",
		},
		{
			name: "missing .pub companion",
			id: &AgentIdentity{SSHKeyPath: func() string {
				kp := filepath.Join(tmpDir, "no_pub_key")
				os.WriteFile(kp, []byte("key"), 0600)
				return kp
			}()},
			wantErr: "sshKeyPath companion",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAgentIdentity(tt.id)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("ValidateAgentIdentity() unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateAgentIdentity() expected error containing %q, got nil", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("ValidateAgentIdentity() error = %q, want containing %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestHostConfig_AgentIdentity_RoundTrip(t *testing.T) {
	config := HostConfig{
		User:           "testuser",
		UID:            1000,
		GID:            100,
		AuthorizedKeys: []string{"ssh-rsa AAAA..."},
		Secrets:        map[string]string{"anthropic": "sk-test"},
		StateDir:       "/var/lib/forage",
		AgentIdentity: &AgentIdentity{
			GitUser:  "Host Agent",
			GitEmail: "host@example.com",
		},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var loaded HostConfig
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if loaded.AgentIdentity == nil {
		t.Fatal("AgentIdentity should not be nil")
	}
	if loaded.AgentIdentity.GitUser != "Host Agent" {
		t.Errorf("GitUser = %q, want %q", loaded.AgentIdentity.GitUser, "Host Agent")
	}
}

// sandboxEnv returns environment variables suitable for running git/jj in a
// hermetic temp directory. This is needed because the nix build sandbox sets
// HOME=/homeless-shelter (non-writable), and jj's "secure config" feature
// writes to ~/.config/jj/repos/.
func sandboxEnv(homeDir string) []string {
	return []string{
		"HOME=" + homeDir,
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"JJ_CONFIG=" + filepath.Join(homeDir, ".config", "jj", "config.toml"),
		"PATH=" + os.Getenv("PATH"),
	}
}

func TestReadGitIdentity(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}

	t.Run("reads identity from git repo", func(t *testing.T) {
		tmpDir := t.TempDir()
		env := sandboxEnv(tmpDir)

		// Initialize a git repo and set identity
		for _, args := range [][]string{
			{"init"},
			{"config", "user.name", "Test User"},
			{"config", "user.email", "test@example.com"},
		} {
			cmd := exec.Command("git", args...)
			cmd.Dir = tmpDir
			cmd.Env = env
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v failed: %v\n%s", args, err, out)
			}
		}

		result := readGitIdentity(tmpDir)
		if result == nil {
			t.Fatal("expected non-nil identity")
		}
		if result.GitUser != "Test User" {
			t.Errorf("GitUser = %q, want %q", result.GitUser, "Test User")
		}
		if result.GitEmail != "test@example.com" {
			t.Errorf("GitEmail = %q, want %q", result.GitEmail, "test@example.com")
		}
		if result.SSHKeyPath != "" {
			t.Errorf("SSHKeyPath = %q, want empty", result.SSHKeyPath)
		}
	})

	t.Run("name with spaces", func(t *testing.T) {
		tmpDir := t.TempDir()
		env := sandboxEnv(tmpDir)
		for _, args := range [][]string{
			{"init"},
			{"config", "user.name", "Yann Hodique"},
			{"config", "user.email", "yann@example.com"},
		} {
			cmd := exec.Command("git", args...)
			cmd.Dir = tmpDir
			cmd.Env = env
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v failed: %v\n%s", args, err, out)
			}
		}

		result := readGitIdentity(tmpDir)
		if result == nil {
			t.Fatal("expected non-nil identity")
		}
		if result.GitUser != "Yann Hodique" {
			t.Errorf("GitUser = %q, want %q", result.GitUser, "Yann Hodique")
		}
	})

	t.Run("no identity configured", func(t *testing.T) {
		tmpDir := t.TempDir()
		env := sandboxEnv(tmpDir)
		cmd := exec.Command("git", "init")
		cmd.Dir = tmpDir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git init failed: %v\n%s", err, out)
		}

		result := readGitIdentity(tmpDir)
		// Result may or may not be nil depending on global git config;
		// we just verify it doesn't crash
		_ = result
	})
}

func TestReadJJIdentity(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not in PATH")
	}

	t.Run("reads identity from jj repo", func(t *testing.T) {
		homeDir := t.TempDir()
		repoDir := t.TempDir()
		env := sandboxEnv(homeDir)
		// jj needs a writable HOME for its secure-config repo tracking,
		// both during setup and when readJJIdentity calls jj config get.
		t.Setenv("HOME", homeDir)

		// Initialize a jj repo
		cmd := exec.Command("jj", "git", "init")
		cmd.Dir = repoDir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("jj git init failed: %v\n%s", err, out)
		}

		// Set identity via jj config
		for _, args := range [][]string{
			{"config", "set", "--repo", "user.name", "JJ User"},
			{"config", "set", "--repo", "user.email", "jj@example.com"},
		} {
			cmd := exec.Command("jj", args...)
			cmd.Dir = repoDir
			cmd.Env = env
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("jj %v failed: %v\n%s", args, err, out)
			}
		}

		result := readJJIdentity(repoDir)
		if result == nil {
			t.Fatal("expected non-nil identity")
		}
		if result.GitUser != "JJ User" {
			t.Errorf("GitUser = %q, want %q", result.GitUser, "JJ User")
		}
		if result.GitEmail != "jj@example.com" {
			t.Errorf("GitEmail = %q, want %q", result.GitEmail, "jj@example.com")
		}
	})

	t.Run("name with spaces", func(t *testing.T) {
		homeDir := t.TempDir()
		repoDir := t.TempDir()
		env := sandboxEnv(homeDir)
		t.Setenv("HOME", homeDir)

		cmd := exec.Command("jj", "git", "init")
		cmd.Dir = repoDir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("jj git init failed: %v\n%s", err, out)
		}
		for _, args := range [][]string{
			{"config", "set", "--repo", "user.name", "Yann Hodique"},
			{"config", "set", "--repo", "user.email", "yann@firefly.engineering"},
		} {
			cmd := exec.Command("jj", args...)
			cmd.Dir = repoDir
			cmd.Env = env
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("jj %v failed: %v\n%s", args, err, out)
			}
		}

		result := readJJIdentity(repoDir)
		if result == nil {
			t.Fatal("expected non-nil identity")
		}
		if result.GitUser != "Yann Hodique" {
			t.Errorf("GitUser = %q, want %q", result.GitUser, "Yann Hodique")
		}
		if result.GitEmail != "yann@firefly.engineering" {
			t.Errorf("GitEmail = %q, want %q", result.GitEmail, "yann@firefly.engineering")
		}
	})
}

func TestTemplate_AgentIdentity_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()

	template := Template{
		Name:    "test-template",
		Network: "full",
		Agents: map[string]AgentConfig{
			"claude": {
				PackagePath: "/nix/store/abc-claude",
				SecretName:  "anthropic",
				AuthEnvVar:  "ANTHROPIC_API_KEY",
			},
		},
		AgentIdentity: &AgentIdentity{
			GitUser:  "Template Agent",
			GitEmail: "template@example.com",
		},
	}

	data, err := json.MarshalIndent(template, "", "  ")
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	templatePath := filepath.Join(tmpDir, "test-template.json")
	if err = os.WriteFile(templatePath, data, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	loaded, err := LoadTemplate(tmpDir, "test-template")
	if err != nil {
		t.Fatalf("LoadTemplate failed: %v", err)
	}

	if loaded.AgentIdentity == nil {
		t.Fatal("AgentIdentity should not be nil after round-trip")
	}
	if loaded.AgentIdentity.GitUser != "Template Agent" {
		t.Errorf("GitUser = %q, want %q", loaded.AgentIdentity.GitUser, "Template Agent")
	}
	if loaded.AgentIdentity.GitEmail != "template@example.com" {
		t.Errorf("GitEmail = %q, want %q", loaded.AgentIdentity.GitEmail, "template@example.com")
	}
}

func TestTemplate_AgentIdentity_BackwardCompat(t *testing.T) {
	tmpDir := t.TempDir()

	// Template JSON without agentIdentity (old format)
	data := `{"name": "old-template", "network": "full", "agents": {"claude": {"packagePath": "/nix/store/abc", "secretName": "anthropic", "authEnvVar": "ANTHROPIC_API_KEY"}}}`
	templatePath := filepath.Join(tmpDir, "old-template.json")
	if err := os.WriteFile(templatePath, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadTemplate(tmpDir, "old-template")
	if err != nil {
		t.Fatalf("LoadTemplate failed: %v", err)
	}

	if loaded.AgentIdentity != nil {
		t.Error("AgentIdentity should be nil for old format without identity")
	}
}

func TestSandboxMetadata_Multiplexer_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()

	metadata := &SandboxMetadata{
		Name:        "mux-test",
		Template:    "claude",
		Workspace:   "/workspace",
		NetworkSlot: 1,
		Multiplexer: "wezterm",
	}

	if err := SaveSandboxMetadata(tmpDir, metadata); err != nil {
		t.Fatalf("SaveSandboxMetadata failed: %v", err)
	}

	loaded, err := LoadSandboxMetadata(tmpDir, "mux-test")
	if err != nil {
		t.Fatalf("LoadSandboxMetadata failed: %v", err)
	}

	if loaded.Multiplexer != "wezterm" {
		t.Errorf("Multiplexer = %q, want %q", loaded.Multiplexer, "wezterm")
	}
}

func TestSandboxMetadata_Multiplexer_BackwardCompat(t *testing.T) {
	tmpDir := t.TempDir()

	// JSON without multiplexer (old format)
	data := `{"name": "old-sandbox", "template": "claude", "networkSlot": 1, "workspace": "/w"}`
	metaPath := filepath.Join(tmpDir, "old-sandbox.json")
	if err := os.WriteFile(metaPath, []byte(data), 0644); err != nil {
		t.Fatalf("Failed to write metadata: %v", err)
	}

	loaded, err := LoadSandboxMetadata(tmpDir, "old-sandbox")
	if err != nil {
		t.Fatalf("LoadSandboxMetadata failed: %v", err)
	}

	if loaded.Multiplexer != "" {
		t.Errorf("Multiplexer = %q, want empty for old format", loaded.Multiplexer)
	}
}

func TestTemplate_Multiplexer_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()

	tmpl := Template{
		Name:        "wez-template",
		Network:     "full",
		Multiplexer: "wezterm",
		Agents: map[string]AgentConfig{
			"claude": {
				PackagePath: "/nix/store/abc-claude",
				SecretName:  "anthropic",
				AuthEnvVar:  "ANTHROPIC_API_KEY",
			},
		},
	}

	data, err := json.MarshalIndent(tmpl, "", "  ")
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	templatePath := filepath.Join(tmpDir, "wez-template.json")
	if err = os.WriteFile(templatePath, data, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	loaded, err := LoadTemplate(tmpDir, "wez-template")
	if err != nil {
		t.Fatalf("LoadTemplate failed: %v", err)
	}

	if loaded.Multiplexer != "wezterm" {
		t.Errorf("Multiplexer = %q, want %q", loaded.Multiplexer, "wezterm")
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

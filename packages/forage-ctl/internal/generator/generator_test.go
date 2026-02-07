package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
)

// validTestConfig returns a valid ContainerConfig for testing
func validTestConfig() *ContainerConfig {
	return &ContainerConfig{
		Name:        "test-sandbox",
		NetworkSlot: 1,
		Workspace:   "/home/user/project",
		SecretsPath: "/run/secrets/test-sandbox",
		AuthorizedKeys: []string{
			"ssh-rsa AAAA... user@host",
		},
		Template: &config.Template{
			Name:        "claude",
			Description: "Claude sandbox",
			Network:     "full",
			Agents: map[string]config.AgentConfig{
				"claude": {
					PackagePath: "pkgs.claude-code",
					SecretName:  "anthropic",
					AuthEnvVar:  "ANTHROPIC_API_KEY",
				},
			},
		},
		HostConfig: &config.HostConfig{
			User: "testuser",
			UID:  1000,
			GID:  100,
		},
		WorkspaceMode: "direct",
		NixpkgsRev:    "abc123def",
		UID:           1000,
		GID:           100,
	}
}

func TestGenerateNixConfig(t *testing.T) {
	cfg := validTestConfig()

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	// Check container name
	if !strings.Contains(result, "containers.forage-test-sandbox") {
		t.Error("Config should contain container name with prefix")
	}

	// Check NO port forwarding (we use direct container IP access now)
	if strings.Contains(result, "forwardPorts") {
		t.Error("Config should NOT contain forwardPorts (using direct IP access)")
	}

	// Check network address
	if !strings.Contains(result, "hostAddress = \"10.100.1.1\"") {
		t.Error("Config should contain host address based on network slot")
	}
	if !strings.Contains(result, "localAddress = \"10.100.1.2\"") {
		t.Error("Config should contain local address based on network slot")
	}

	// Check bind mounts
	if !strings.Contains(result, "/nix/store") {
		t.Error("Config should mount nix store")
	}
	if !strings.Contains(result, "/workspace") {
		t.Error("Config should mount workspace")
	}
	if !strings.Contains(result, "/run/secrets") {
		t.Error("Config should mount secrets")
	}

	// Check authorized keys
	if !strings.Contains(result, "ssh-rsa AAAA") {
		t.Error("Config should contain authorized keys")
	}

	// Check nixpkgs registry
	if !strings.Contains(result, "abc123def") {
		t.Error("Config should contain nixpkgs revision")
	}

	// Check packages
	if !strings.Contains(result, "jujutsu") {
		t.Error("Config should include jujutsu package")
	}
}

func TestGenerateNixConfig_HostConfigDir(t *testing.T) {
	cfg := validTestConfig()
	cfg.Template.Agents["claude"] = config.AgentConfig{
		PackagePath:        "pkgs.claude-code",
		SecretName:         "anthropic",
		AuthEnvVar:         "ANTHROPIC_API_KEY",
		HostConfigDir:      "/home/user/.claude",
		ContainerConfigDir: "/home/agent/.claude",
	}

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	// Check that the bind mount is present
	if !strings.Contains(result, "/home/agent/.claude") {
		t.Error("Config should contain container config dir path")
	}
	if !strings.Contains(result, "/home/user/.claude") {
		t.Error("Config should contain host config dir path")
	}
}

func TestGenerateNixConfig_HostConfigDirReadOnly(t *testing.T) {
	cfg := validTestConfig()
	cfg.Template.Agents["claude"] = config.AgentConfig{
		PackagePath:           "pkgs.claude-code",
		SecretName:            "anthropic",
		AuthEnvVar:            "ANTHROPIC_API_KEY",
		HostConfigDir:         "/home/user/.claude",
		ContainerConfigDir:    "/home/agent/.claude",
		HostConfigDirReadOnly: true,
	}

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	// Check that the bind mount is present with read-only flag
	if !strings.Contains(result, "/home/agent/.claude") {
		t.Error("Config should contain container config dir path")
	}
	// The mount should be read-only - look for isReadOnly = true pattern near our mount
	if !strings.Contains(result, "isReadOnly = true") {
		t.Error("Config should have at least one read-only mount")
	}
}

func TestGenerateNixConfig_MultipleAgentsWithConfigDirs(t *testing.T) {
	cfg := validTestConfig()
	cfg.Template.Agents = map[string]config.AgentConfig{
		"claude": {
			PackagePath:        "pkgs.claude-code",
			SecretName:         "anthropic",
			AuthEnvVar:         "ANTHROPIC_API_KEY",
			HostConfigDir:      "/home/user/.claude",
			ContainerConfigDir: "/home/agent/.claude",
		},
		"aider": {
			PackagePath:        "pkgs.aider",
			SecretName:         "openai",
			AuthEnvVar:         "OPENAI_API_KEY",
			HostConfigDir:      "/home/user/.aider",
			ContainerConfigDir: "/home/agent/.aider",
		},
	}

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	// Check both agent config dirs are mounted
	if !strings.Contains(result, "/home/agent/.claude") {
		t.Error("Config should contain claude container config dir path")
	}
	if !strings.Contains(result, "/home/user/.claude") {
		t.Error("Config should contain claude host config dir path")
	}
	if !strings.Contains(result, "/home/agent/.aider") {
		t.Error("Config should contain aider container config dir path")
	}
	if !strings.Contains(result, "/home/user/.aider") {
		t.Error("Config should contain aider host config dir path")
	}
}

func TestGenerateNixConfig_JJMode(t *testing.T) {
	cfg := validTestConfig()
	cfg.Workspace = "/var/lib/forage/workspaces/test-sandbox"
	cfg.WorkspaceMode = "jj"
	cfg.SourceRepo = "/home/user/myrepo"

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	// Check JJ bind mount
	if !strings.Contains(result, "/home/user/myrepo/.jj") {
		t.Error("Config should contain .jj bind mount for jj mode")
	}
}

func TestGenerateNixConfig_ClaudeDirMount(t *testing.T) {
	// Create a temp dir to act as the source repo with a .claude/ directory
	sourceRepo := t.TempDir()
	claudeDir := filepath.Join(sourceRepo, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("failed to create .claude dir: %v", err)
	}

	tests := []struct {
		name      string
		mode      string
		wantMount bool
	}{
		{"jj mode", "jj", true},
		{"git-worktree mode", "git-worktree", true},
		{"direct mode", "direct", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validTestConfig()
			cfg.Workspace = "/var/lib/forage/workspaces/test-sandbox"
			cfg.WorkspaceMode = tt.mode
			cfg.SourceRepo = sourceRepo

			result, err := GenerateNixConfig(cfg)
			if err != nil {
				t.Fatalf("GenerateNixConfig failed: %v", err)
			}

			hasMount := strings.Contains(result, "/workspace/.claude")
			if tt.wantMount && !hasMount {
				t.Errorf("expected .claude bind mount in %s mode, but not found", tt.mode)
			}
			if !tt.wantMount && hasMount {
				t.Errorf("did not expect .claude bind mount in %s mode, but found it", tt.mode)
			}
		})
	}
}

func TestGenerateNixConfig_NonClaudeAgent_NoClaudeDir(t *testing.T) {
	// A non-claude agent should NOT get .claude/ mounted even if it exists
	sourceRepo := t.TempDir()
	claudeDir := filepath.Join(sourceRepo, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("failed to create .claude dir: %v", err)
	}

	cfg := validTestConfig()
	cfg.Workspace = "/var/lib/forage/workspaces/test-sandbox"
	cfg.WorkspaceMode = "jj"
	cfg.SourceRepo = sourceRepo
	// Replace claude agent with a non-claude agent
	cfg.Template.Agents = map[string]config.AgentConfig{
		"opencode": {
			PackagePath: "pkgs.opencode",
			SecretName:  "openai",
			AuthEnvVar:  "OPENAI_API_KEY",
		},
	}

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	if strings.Contains(result, "/workspace/.claude") {
		t.Error("should not mount .claude for non-claude agent")
	}
}

func TestGenerateNixConfig_ClaudeDirMount_NoDir(t *testing.T) {
	// Source repo exists but has no .claude/ directory — mount should not appear
	sourceRepo := t.TempDir()

	cfg := validTestConfig()
	cfg.Workspace = "/var/lib/forage/workspaces/test-sandbox"
	cfg.WorkspaceMode = "jj"
	cfg.SourceRepo = sourceRepo

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	if strings.Contains(result, "/workspace/.claude") {
		t.Error("should not mount .claude when directory does not exist in source repo")
	}
}

func TestGenerateNixConfig_NetworkModes(t *testing.T) {
	tests := []struct {
		network      string
		allowedHosts []string
		shouldHave   []string
		shouldntHave []string
	}{
		{
			network:    "full",
			shouldHave: []string{"defaultGateway", "nameservers"},
		},
		{
			network:    "none",
			shouldHave: []string{"nameservers = []", "defaultGateway = null", "OUTPUT -j DROP"},
		},
		{
			network:      "restricted",
			allowedHosts: []string{"api.anthropic.com"},
			shouldHave:   []string{"nftables", "dnsmasq", "api.anthropic.com", "allowed_ipv4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.network, func(t *testing.T) {
			cfg := validTestConfig()
			cfg.Template.Network = tt.network
			cfg.Template.AllowedHosts = tt.allowedHosts

			result, err := GenerateNixConfig(cfg)
			if err != nil {
				t.Fatalf("GenerateNixConfig failed: %v", err)
			}

			for _, s := range tt.shouldHave {
				if !strings.Contains(result, s) {
					t.Errorf("Network mode %q should contain %q", tt.network, s)
				}
			}

			for _, s := range tt.shouldntHave {
				if strings.Contains(result, s) {
					t.Errorf("Network mode %q should not contain %q", tt.network, s)
				}
			}
		})
	}
}

func TestGenerateNixConfig_NoNixpkgsRev(t *testing.T) {
	cfg := validTestConfig()
	cfg.NixpkgsRev = "" // No revision

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	// Should not contain registry config when no revision
	if strings.Contains(result, "registry.json") {
		t.Error("Config should not contain registry config when NixpkgsRev is empty")
	}
}

func TestGenerateNixConfig_ProxyMode(t *testing.T) {
	cfg := validTestConfig()
	cfg.ProxyURL = "http://10.100.1.1:8080"

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	// Should contain proxy environment variables
	if !strings.Contains(result, "ANTHROPIC_BASE_URL") {
		t.Error("Config should contain ANTHROPIC_BASE_URL when proxy is enabled")
	}
	if !strings.Contains(result, "http://10.100.1.1:8080") {
		t.Error("Config should contain proxy URL")
	}
	if !strings.Contains(result, "X-Forage-Sandbox") {
		t.Error("Config should contain X-Forage-Sandbox header")
	}
	if !strings.Contains(result, "test-sandbox") {
		t.Error("Config should contain sandbox name in header")
	}
	// Should NOT contain direct secret reading when proxy is enabled
	if strings.Contains(result, "cat /run/secrets/anthropic") {
		t.Error("Config should not read secrets directly when proxy is enabled")
	}
}

func TestGenerateNixConfig_NoProxy(t *testing.T) {
	cfg := validTestConfig()
	cfg.ProxyURL = "" // No proxy

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	// Should contain direct secret reading
	if !strings.Contains(result, "cat /run/secrets/anthropic") {
		t.Error("Config should read secrets directly when proxy is disabled")
	}
	// Should NOT contain proxy URL
	if strings.Contains(result, "ANTHROPIC_BASE_URL") {
		t.Error("Config should not contain ANTHROPIC_BASE_URL when proxy is disabled")
	}
}

func TestContainerConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*ContainerConfig)
		wantErr string
	}{
		{
			name:    "valid config",
			modify:  func(c *ContainerConfig) {},
			wantErr: "",
		},
		{
			name:    "missing name",
			modify:  func(c *ContainerConfig) { c.Name = "" },
			wantErr: "container name is required",
		},
		{
			name:    "invalid network slot (zero)",
			modify:  func(c *ContainerConfig) { c.NetworkSlot = 0 },
			wantErr: "invalid network slot",
		},
		{
			name:    "invalid network slot (too high)",
			modify:  func(c *ContainerConfig) { c.NetworkSlot = 300 },
			wantErr: "invalid network slot",
		},
		{
			name:    "missing workspace",
			modify:  func(c *ContainerConfig) { c.Workspace = "" },
			wantErr: "workspace path is required",
		},
		// SecretsPath is optional - no validation test needed
		{
			name:    "missing authorized keys",
			modify:  func(c *ContainerConfig) { c.AuthorizedKeys = nil },
			wantErr: "at least one authorized key is required",
		},
		{
			name:    "missing template",
			modify:  func(c *ContainerConfig) { c.Template = nil },
			wantErr: "template is required",
		},
		{
			name:    "invalid workspace mode",
			modify:  func(c *ContainerConfig) { c.WorkspaceMode = "invalid" },
			wantErr: "invalid workspace mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validTestConfig()
			tt.modify(cfg)

			err := cfg.Validate()
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

func TestGenerateSkills(t *testing.T) {
	metadata := &config.SandboxMetadata{
		Name:          "test-sandbox",
		Template:      "claude",
		WorkspaceMode: "direct",
	}

	template := &config.Template{
		Name:        "claude",
		Description: "Claude sandbox",
		Network:     "full",
		Agents: map[string]config.AgentConfig{
			"claude": {
				AuthEnvVar: "ANTHROPIC_API_KEY",
			},
		},
	}

	result := GenerateSkills(metadata, template)

	// Check basic content
	if !strings.Contains(result, "test-sandbox") {
		t.Error("Skills should contain sandbox name")
	}
	if !strings.Contains(result, "claude") {
		t.Error("Skills should contain template name")
	}
	if !strings.Contains(result, "/workspace") {
		t.Error("Skills should mention workspace")
	}
}

func TestGenerateSkills_JJMode(t *testing.T) {
	metadata := &config.SandboxMetadata{
		Name:          "test-sandbox",
		Template:      "claude",
		WorkspaceMode: "jj",
		SourceRepo:    "/home/user/myrepo",
	}

	template := &config.Template{
		Name:    "claude",
		Network: "full",
	}

	result := GenerateSkills(metadata, template)

	// Check JJ-specific content
	if !strings.Contains(result, "jj status") {
		t.Error("Skills should contain jj commands for jj mode")
	}
	if !strings.Contains(result, "jj diff") {
		t.Error("Skills should contain jj diff command")
	}
	if !strings.Contains(result, "isolated jj workspace") {
		t.Error("Skills should explain jj isolation")
	}
}

func TestGenerateSkills_NetworkModes(t *testing.T) {
	tests := []struct {
		network    string
		shouldHave string
	}{
		{"full", "Full network access"},
		{"none", "No network access"},
		{"restricted", "Restricted network"},
	}

	for _, tt := range tests {
		t.Run(tt.network, func(t *testing.T) {
			metadata := &config.SandboxMetadata{
				Name:     "test",
				Template: "test",
			}
			template := &config.Template{
				Network: tt.network,
			}

			result := GenerateSkills(metadata, template)

			if !strings.Contains(result, tt.shouldHave) {
				t.Errorf("Skills for network %q should contain %q", tt.network, tt.shouldHave)
			}
		})
	}
}

func TestGenerateSkills_RestrictedHosts(t *testing.T) {
	metadata := &config.SandboxMetadata{
		Name:     "test",
		Template: "test",
	}
	template := &config.Template{
		Network:      "restricted",
		AllowedHosts: []string{"api.anthropic.com", "github.com"},
	}

	result := GenerateSkills(metadata, template)

	if !strings.Contains(result, "api.anthropic.com") {
		t.Error("Skills should list allowed hosts")
	}
	if !strings.Contains(result, "github.com") {
		t.Error("Skills should list allowed hosts")
	}
}

func TestGenerateSkills_WithAgents(t *testing.T) {
	metadata := &config.SandboxMetadata{
		Name:     "test",
		Template: "multi",
	}
	template := &config.Template{
		Network: "full",
		Agents: map[string]config.AgentConfig{
			"claude": {
				AuthEnvVar: "ANTHROPIC_API_KEY",
			},
			"opencode": {
				AuthEnvVar: "OPENAI_API_KEY",
			},
		},
	}

	result := GenerateSkills(metadata, template)

	if !strings.Contains(result, "Available Agents") {
		t.Error("Skills should have agents section")
	}
	if !strings.Contains(result, "claude") {
		t.Error("Skills should list claude agent")
	}
	if !strings.Contains(result, "opencode") {
		t.Error("Skills should list opencode agent")
	}
}

// Golden test configuration helpers

func goldenTestConfig() *ContainerConfig {
	return &ContainerConfig{
		Name:        "test-sandbox",
		NetworkSlot: 1,
		Workspace:   "/home/user/project",
		SecretsPath: "/run/secrets/test-sandbox",
		AuthorizedKeys: []string{
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExample user@host",
		},
		Template: &config.Template{
			Name:        "claude",
			Description: "Claude sandbox",
			Network:     "full",
			Agents: map[string]config.AgentConfig{
				"claude": {
					PackagePath: "pkgs.claude-code",
					SecretName:  "anthropic",
					AuthEnvVar:  "ANTHROPIC_API_KEY",
				},
			},
		},
		HostConfig: &config.HostConfig{
			User: "testuser",
			UID:  1000,
			GID:  100,
		},
		WorkspaceMode: "direct",
		NixpkgsRev:    "abc123def456",
		UID:           1000,
		GID:           100,
	}
}

func readGoldenFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read golden file %s: %v", path, err)
	}
	return string(data)
}

func TestGenerateNixConfig_Golden(t *testing.T) {
	tests := []struct {
		name       string
		modifyFunc func(*ContainerConfig)
		goldenFile string
	}{
		{
			name:       "basic",
			modifyFunc: func(c *ContainerConfig) {},
			goldenFile: "basic_container.nix",
		},
		{
			name: "jj_mode",
			modifyFunc: func(c *ContainerConfig) {
				c.Workspace = "/var/lib/forage/workspaces/test-sandbox"
				c.WorkspaceMode = "jj"
				c.SourceRepo = "/home/user/myrepo"
			},
			goldenFile: "jj_mode_container.nix",
		},
		{
			name: "proxy_mode",
			modifyFunc: func(c *ContainerConfig) {
				c.ProxyURL = "http://10.100.1.1:8080"
			},
			goldenFile: "proxy_mode_container.nix",
		},
		{
			name: "no_network",
			modifyFunc: func(c *ContainerConfig) {
				c.Template.Network = "none"
			},
			goldenFile: "no_network_container.nix",
		},
		{
			name: "no_registry",
			modifyFunc: func(c *ContainerConfig) {
				c.NixpkgsRev = ""
			},
			goldenFile: "no_registry_container.nix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := goldenTestConfig()
			tt.modifyFunc(cfg)

			result, err := GenerateNixConfig(cfg)
			if err != nil {
				t.Fatalf("GenerateNixConfig failed: %v", err)
			}

			golden := readGoldenFile(t, tt.goldenFile)
			if result != golden {
				t.Errorf("Generated config does not match golden file %s.\nGot:\n%s\nWant:\n%s", tt.goldenFile, result, golden)
			}
		})
	}
}

// TestGenerateNixConfig_RestrictedNetwork tests restricted network mode separately
// because it involves DNS resolution which produces dynamic IP addresses.
func TestGenerateNixConfig_RestrictedNetwork(t *testing.T) {
	cfg := goldenTestConfig()
	cfg.Template.Network = "restricted"
	cfg.Template.AllowedHosts = []string{"api.anthropic.com", "github.com"}

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	// Check for key structural elements (IPs may vary due to DNS resolution)
	required := []string{
		"containers.forage-test-sandbox",
		"nftables",
		"dnsmasq",
		"allowed_ipv4",
		"allowed_ipv6",
		"api.anthropic.com",
		"github.com",
		"server=/api.anthropic.com/1.1.1.1",
		"server=/github.com/1.1.1.1",
	}

	for _, s := range required {
		if !strings.Contains(result, s) {
			t.Errorf("Restricted network config should contain %q", s)
		}
	}
}

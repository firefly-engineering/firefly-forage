package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/injection"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/multiplexer"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/reproducibility"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/skills"
)

// testContributions creates a minimal set of contributions for testing.
func testContributions() *injection.Contributions {
	return &injection.Contributions{
		Mounts: []injection.Mount{
			{HostPath: "/nix/store", ContainerPath: "/nix/store", ReadOnly: true},
			{HostPath: "/home/user/project", ContainerPath: "/workspace", ReadOnly: false},
			{HostPath: "/run/secrets/test-sandbox", ContainerPath: "/run/secrets", ReadOnly: true},
		},
		EnvVars: []injection.EnvVar{
			{Name: "ANTHROPIC_API_KEY", Value: `"$(cat /run/secrets/anthropic 2>/dev/null || echo '')"`},
		},
		Packages: []injection.Package{
			{Name: "claude-code"},
		},
		TmpfilesRules: []string{
			"d /home/agent/.config 0755 agent users -",
		},
	}
}

// validTestConfig returns a valid ContainerConfig for testing
func validTestConfig() *ContainerConfig {
	return &ContainerConfig{
		Name:        "test-sandbox",
		NetworkSlot: 1,
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
		UID:             1000,
		GID:             100,
		Contributions:   testContributions(),
		Reproducibility: reproducibility.NewNixReproducibility(),
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

	// Check nixpkgs registry uses local store path
	if !strings.Contains(result, `"path"; path = "${pkgs.path}"`) {
		t.Error("Config should pin nixpkgs registry to local pkgs.path")
	}

	// Check packages
	if !strings.Contains(result, "jujutsu") {
		t.Error("Config should include jujutsu package")
	}
}

func TestGenerateNixConfig_HostConfigDir(t *testing.T) {
	cfg := validTestConfig()
	// Add agent config dir mount via contributions
	cfg.Contributions.Mounts = append(cfg.Contributions.Mounts, injection.Mount{
		HostPath:      "/home/user/.claude",
		ContainerPath: "/home/agent/.claude",
		ReadOnly:      false,
	})

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
	// Add agent config dir mount via contributions as read-only
	cfg.Contributions.Mounts = append(cfg.Contributions.Mounts, injection.Mount{
		HostPath:      "/home/user/.claude",
		ContainerPath: "/home/agent/.claude",
		ReadOnly:      true,
	})

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
			PackagePath: "pkgs.claude-code",
			SecretName:  "anthropic",
			AuthEnvVar:  "ANTHROPIC_API_KEY",
		},
		"aider": {
			PackagePath: "pkgs.aider",
			SecretName:  "openai",
			AuthEnvVar:  "OPENAI_API_KEY",
		},
	}
	// Add config dir mounts for both agents via contributions
	cfg.Contributions.Mounts = append(cfg.Contributions.Mounts,
		injection.Mount{
			HostPath:      "/home/user/.claude",
			ContainerPath: "/home/agent/.claude",
			ReadOnly:      false,
		},
		injection.Mount{
			HostPath:      "/home/user/.aider",
			ContainerPath: "/home/agent/.aider",
			ReadOnly:      false,
		},
	)

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
	// Add JJ mount to contributions
	cfg.Contributions.Mounts = append(cfg.Contributions.Mounts, injection.Mount{
		HostPath:      "/home/user/myrepo/.jj",
		ContainerPath: "/home/user/myrepo/.jj",
		ReadOnly:      false,
	})

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	// Check JJ bind mount
	if !strings.Contains(result, "/home/user/myrepo/.jj") {
		t.Error("Config should contain .jj bind mount from contributions")
	}
}

func TestGenerateNixConfig_ClaudeDirMount(t *testing.T) {
	// Test that mounts in contributions are applied
	cfg := validTestConfig()
	// Add .claude mount to contributions
	cfg.Contributions.Mounts = append(cfg.Contributions.Mounts, injection.Mount{
		HostPath:      "/home/user/myrepo/.claude",
		ContainerPath: "/workspace/.claude",
		ReadOnly:      false,
	})

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	if !strings.Contains(result, "/workspace/.claude") {
		t.Error("Config should contain .claude bind mount from contributions")
	}
}

func TestGenerateNixConfig_NoMountWithoutContribution(t *testing.T) {
	// Without a mount in contributions, the path should not appear
	cfg := validTestConfig()
	// Don't add any .claude mount

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	if strings.Contains(result, "/workspace/.claude") {
		t.Error("should not mount .claude when not in contributions")
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

func TestGenerateNixConfig_ProxyMode(t *testing.T) {
	cfg := validTestConfig()
	// Add proxy env vars via contributions
	cfg.Contributions.EnvVars = append(cfg.Contributions.EnvVars,
		injection.EnvVar{Name: "ANTHROPIC_BASE_URL", Value: `"http://10.100.1.1:8080"`},
		injection.EnvVar{Name: "ANTHROPIC_AUTH_TOKEN", Value: `"ignored-by-proxy"`},
		injection.EnvVar{Name: "ANTHROPIC_CUSTOM_HEADERS", Value: `"X-Forage-Sandbox: test-sandbox"`},
	)
	// Remove the direct secret reading env var
	cfg.Contributions.EnvVars = []injection.EnvVar{
		{Name: "ANTHROPIC_BASE_URL", Value: `"http://10.100.1.1:8080"`},
		{Name: "ANTHROPIC_AUTH_TOKEN", Value: `"ignored-by-proxy"`},
		{Name: "ANTHROPIC_CUSTOM_HEADERS", Value: `"X-Forage-Sandbox: test-sandbox"`},
	}

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
	// Default testContributions has direct secret reading, no proxy env vars

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
			name:    "missing contributions",
			modify:  func(c *ContainerConfig) { c.Contributions = nil },
			wantErr: "contributions is required",
		},
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
			name:    "missing reproducibility",
			modify:  func(c *ContainerConfig) { c.Reproducibility = nil },
			wantErr: "reproducibility is required",
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

func TestGenerateSystemPrompt(t *testing.T) {
	metadata := &config.SandboxMetadata{
		Name:          "test-sandbox",
		Template:      "claude",
		WorkspaceMode: "direct",
	}

	tmpl := &config.Template{
		Name:        "claude",
		Description: "Claude sandbox",
		Network:     "full",
		Agents: map[string]config.AgentConfig{
			"claude": {
				AuthEnvVar: "ANTHROPIC_API_KEY",
			},
		},
	}

	result := GenerateSystemPrompt(metadata, tmpl)

	if !strings.Contains(result, "test-sandbox") {
		t.Error("System prompt should contain sandbox name")
	}
	if !strings.Contains(result, "claude") {
		t.Error("System prompt should contain template name")
	}
	if !strings.Contains(result, "/workspace") {
		t.Error("System prompt should mention workspace")
	}
	if !strings.Contains(result, "Full network access") {
		t.Error("System prompt should mention network mode")
	}
}

func TestGenerateSystemPrompt_JJMode(t *testing.T) {
	metadata := &config.SandboxMetadata{
		Name:          "test-sandbox",
		Template:      "claude",
		WorkspaceMode: "jj",
		SourceRepo:    "/home/user/myrepo",
	}

	tmpl := &config.Template{
		Name:    "claude",
		Network: "full",
	}

	result := GenerateSystemPrompt(metadata, tmpl)

	if !strings.Contains(result, "jj workspace") {
		t.Error("System prompt should mention jj workspace mode")
	}
	if !strings.Contains(result, "/home/user/myrepo") {
		t.Error("System prompt should mention source repo")
	}
}

func TestGenerateSystemPrompt_NetworkModes(t *testing.T) {
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
			tmpl := &config.Template{
				Network: tt.network,
			}

			result := GenerateSystemPrompt(metadata, tmpl)

			if !strings.Contains(result, tt.shouldHave) {
				t.Errorf("System prompt for network %q should contain %q\nGot:\n%s", tt.network, tt.shouldHave, result)
			}
		})
	}
}

func TestGenerateSystemPrompt_RestrictedHosts(t *testing.T) {
	metadata := &config.SandboxMetadata{
		Name:     "test",
		Template: "test",
	}
	tmpl := &config.Template{
		Network:      "restricted",
		AllowedHosts: []string{"api.anthropic.com", "github.com"},
	}

	result := GenerateSystemPrompt(metadata, tmpl)

	if !strings.Contains(result, "api.anthropic.com") {
		t.Error("System prompt should list allowed hosts")
	}
	if !strings.Contains(result, "github.com") {
		t.Error("System prompt should list allowed hosts")
	}
}

func TestGenerateSystemPrompt_Identity(t *testing.T) {
	metadata := &config.SandboxMetadata{
		Name:     "test",
		Template: "claude",
		AgentIdentity: &config.AgentIdentity{
			GitUser:    "Bot",
			GitEmail:   "bot@test.com",
			SSHKeyPath: "/key",
		},
	}
	tmpl := &config.Template{
		Network: "full",
	}

	result := GenerateSystemPrompt(metadata, tmpl)

	if !strings.Contains(result, "Identity") {
		t.Error("System prompt should have identity info")
	}
	if !strings.Contains(result, "Bot") {
		t.Error("System prompt should contain git user name")
	}
	if !strings.Contains(result, "bot@test.com") {
		t.Error("System prompt should contain git email")
	}
	if !strings.Contains(result, "SSH key available") {
		t.Error("System prompt should mention SSH key")
	}
}

func TestGenerateSystemPrompt_IdentityGitOnly(t *testing.T) {
	metadata := &config.SandboxMetadata{
		Name:     "test",
		Template: "claude",
		AgentIdentity: &config.AgentIdentity{
			GitUser: "Bot",
		},
	}
	tmpl := &config.Template{
		Network: "full",
	}

	result := GenerateSystemPrompt(metadata, tmpl)

	if !strings.Contains(result, "Identity") {
		t.Error("System prompt should have identity info")
	}
	if strings.Contains(result, "SSH key") {
		t.Error("System prompt should not mention SSH key when not set")
	}
}

func TestGenerateSystemPrompt_NoIdentity(t *testing.T) {
	metadata := &config.SandboxMetadata{
		Name:     "test",
		Template: "claude",
	}
	tmpl := &config.Template{
		Network: "full",
	}

	result := GenerateSystemPrompt(metadata, tmpl)

	if strings.Contains(result, "Identity") {
		t.Error("System prompt should not have identity info when none configured")
	}
}

func TestGenerateSystemPrompt_WithAgents(t *testing.T) {
	metadata := &config.SandboxMetadata{
		Name:     "test",
		Template: "multi",
	}
	tmpl := &config.Template{
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

	result := GenerateSystemPrompt(metadata, tmpl)

	if !strings.Contains(result, "Agents") {
		t.Error("System prompt should have agents info")
	}
	if !strings.Contains(result, "claude") {
		t.Error("System prompt should list claude agent")
	}
	if !strings.Contains(result, "opencode") {
		t.Error("System prompt should list opencode agent")
	}
}

func TestGenerateSkillFiles_VCS(t *testing.T) {
	tests := []struct {
		name       string
		metadata   *config.SandboxMetadata
		info       *skills.ProjectInfo
		wantSkill  bool
		shouldHave []string
	}{
		{
			name: "jj mode",
			metadata: &config.SandboxMetadata{
				Name:          "test",
				Template:      "test",
				WorkspaceMode: "jj",
			},
			wantSkill:  true,
			shouldHave: []string{"jj status", "jj diff", "isolated jj workspace"},
		},
		{
			name: "git-worktree mode",
			metadata: &config.SandboxMetadata{
				Name:          "test",
				Template:      "test",
				WorkspaceMode: "git-worktree",
				GitBranch:     "test-branch",
			},
			wantSkill:  true,
			shouldHave: []string{"git status", "test-branch", "Git Worktree"},
		},
		{
			name: "plain git",
			metadata: &config.SandboxMetadata{
				Name:     "test",
				Template: "test",
			},
			info:      &skills.ProjectInfo{HasGit: true},
			wantSkill: false,
		},
		{
			name: "no vcs",
			metadata: &config.SandboxMetadata{
				Name:     "test",
				Template: "test",
			},
			info:      &skills.ProjectInfo{},
			wantSkill: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl := &config.Template{Network: "full"}
			result := GenerateSkillFiles(tt.metadata, tmpl, tt.info)
			vcs, ok := result["forage-vcs"]
			if tt.wantSkill && !ok {
				t.Fatal("expected forage-vcs skill file")
			}
			if !tt.wantSkill && ok {
				t.Fatal("did not expect forage-vcs skill file")
			}
			for _, s := range tt.shouldHave {
				if !strings.Contains(vcs, s) {
					t.Errorf("forage-vcs should contain %q\nGot:\n%s", s, vcs)
				}
			}
		})
	}
}

func TestGenerateSkillFiles_Nix(t *testing.T) {
	tmpl := &config.Template{Network: "full"}
	metadata := &config.SandboxMetadata{Name: "test", Template: "test"}

	info := &skills.ProjectInfo{HasNixFlake: true}
	result := GenerateSkillFiles(metadata, tmpl, info)
	nix, ok := result["forage-nix"]
	if !ok {
		t.Fatal("expected forage-nix skill file")
	}

	for _, s := range []string{"nix build", "nix develop", "nix flake check"} {
		if !strings.Contains(nix, s) {
			t.Errorf("forage-nix should contain %q", s)
		}
	}
}

func TestGenerateSkillFiles_Empty(t *testing.T) {
	tmpl := &config.Template{Network: "full"}
	metadata := &config.SandboxMetadata{Name: "test", Template: "test"}

	result := GenerateSkillFiles(metadata, tmpl, nil)
	if len(result) != 0 {
		t.Errorf("expected empty skill files map, got %d entries", len(result))
	}
}

// Golden test configuration helpers

func goldenTestConfig() *ContainerConfig {
	return &ContainerConfig{
		Name:        "test-sandbox",
		NetworkSlot: 1,
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
		UID:             1000,
		GID:             100,
		Contributions:   testContributions(),
		Reproducibility: reproducibility.NewNixReproducibility(),
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
				// Change workspace mount to jj workspace path and add .jj mount
				c.Contributions.Mounts = []injection.Mount{
					{HostPath: "/nix/store", ContainerPath: "/nix/store", ReadOnly: true},
					{HostPath: "/var/lib/forage/workspaces/test-sandbox", ContainerPath: "/workspace", ReadOnly: false},
					{HostPath: "/run/secrets/test-sandbox", ContainerPath: "/run/secrets", ReadOnly: true},
					{HostPath: "/home/user/myrepo/.jj", ContainerPath: "/home/user/myrepo/.jj", ReadOnly: false},
				}
			},
			goldenFile: "jj_mode_container.nix",
		},
		{
			name: "proxy_mode",
			modifyFunc: func(c *ContainerConfig) {
				// Replace direct secret reading with proxy env vars
				c.Contributions.EnvVars = []injection.EnvVar{
					{Name: "ANTHROPIC_BASE_URL", Value: `"http://10.100.1.1:8080"`},
					{Name: "ANTHROPIC_AUTH_TOKEN", Value: `"ignored-by-proxy"`},
					{Name: "ANTHROPIC_CUSTOM_HEADERS", Value: `"X-Forage-Sandbox: test-sandbox"`},
				}
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
				// To regenerate golden files, run:
				// UPDATE_GOLDEN=1 go test -run TestGenerateNixConfig_Golden ./internal/generator/...
				if os.Getenv("UPDATE_GOLDEN") == "1" {
					path := filepath.Join("testdata", tt.goldenFile)
					if err := os.WriteFile(path, []byte(result), 0644); err != nil {
						t.Fatalf("failed to update golden file: %v", err)
					}
					t.Logf("Updated golden file: %s", path)
					return
				}
				t.Errorf("Generated config does not match golden file %s.\nRun with UPDATE_GOLDEN=1 to regenerate.\nGot:\n%s\nWant:\n%s", tt.goldenFile, result, golden)
			}
		})
	}
}

func TestGenerateNixConfig_PermissionsMounts(t *testing.T) {
	cfg := validTestConfig()
	// Add permissions mount via contributions
	cfg.Contributions.Mounts = append(cfg.Contributions.Mounts, injection.Mount{
		HostPath:      "/var/lib/forage/sandboxes/test-sandbox.claude-permissions.json",
		ContainerPath: "/etc/claude-code/managed-settings.json",
		ReadOnly:      true,
	})
	cfg.Contributions.TmpfilesRules = append(cfg.Contributions.TmpfilesRules,
		"d /etc/claude-code 0755 root root -",
	)

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	// Check bind mount is present
	if !strings.Contains(result, "/etc/claude-code/managed-settings.json") {
		t.Error("Config should contain permissions container path")
	}
	if !strings.Contains(result, "/var/lib/forage/sandboxes/test-sandbox.claude-permissions.json") {
		t.Error("Config should contain permissions host path")
	}
	// Check that the mount is read-only
	if !strings.Contains(result, `"/etc/claude-code/managed-settings.json" = { hostPath = "/var/lib/forage/sandboxes/test-sandbox.claude-permissions.json"; isReadOnly = true; }`) {
		t.Error("Permissions mount should be read-only")
	}

	// Check tmpfiles rule for parent directory
	if !strings.Contains(result, "d /etc/claude-code 0755 root root -") {
		t.Error("Config should contain tmpfiles rule for permissions directory")
	}
}

func TestGenerateNixConfig_NoPermissionsMounts(t *testing.T) {
	cfg := validTestConfig()
	// Default testContributions has no permissions mounts

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	// Should not contain any permissions-related mount
	if strings.Contains(result, "managed-settings.json") {
		t.Error("Config should not contain permissions mount when none configured")
	}
	if strings.Contains(result, "/etc/claude-code") {
		t.Error("Config should not contain claude-code dir when no permissions configured")
	}
}

func TestGenerateNixConfig_IdentityGitOnly(t *testing.T) {
	cfg := validTestConfig()
	cfg.AgentIdentity = &config.AgentIdentity{
		GitUser:  "Agent Bot",
		GitEmail: "agent@example.com",
	}

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	// Should contain identity service with git config
	if !strings.Contains(result, "forage-agent-identity") {
		t.Error("Config should contain forage-agent-identity service")
	}
	if !strings.Contains(result, "user.name") {
		t.Error("Config should set git user.name")
	}
	if !strings.Contains(result, "Agent Bot") {
		t.Error("Config should contain git user name")
	}
	if !strings.Contains(result, "user.email") {
		t.Error("Config should set git user.email")
	}
	if !strings.Contains(result, "agent@example.com") {
		t.Error("Config should contain git user email")
	}
	// Should NOT have SSH key mounts
	if strings.Contains(result, "/home/agent/.ssh/id_ed25519") {
		t.Error("Config should not have SSH key mount without SSHKeyPath")
	}
}

func TestGenerateNixConfig_IdentityWithSSHKey(t *testing.T) {
	// Create temp SSH key files
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "id_ed25519")
	os.WriteFile(keyPath, []byte("key"), 0600)
	os.WriteFile(keyPath+".pub", []byte("pub"), 0644)

	cfg := validTestConfig()
	cfg.AgentIdentity = &config.AgentIdentity{
		GitUser:    "Agent Bot",
		GitEmail:   "agent@example.com",
		SSHKeyPath: keyPath,
	}
	// Add SSH key mounts via contributions (as the identity contributor would)
	cfg.Contributions.Mounts = append(cfg.Contributions.Mounts,
		injection.Mount{
			HostPath:      keyPath,
			ContainerPath: "/home/agent/.ssh/id_ed25519",
			ReadOnly:      true,
		},
		injection.Mount{
			HostPath:      keyPath + ".pub",
			ContainerPath: "/home/agent/.ssh/id_ed25519.pub",
			ReadOnly:      true,
		},
	)
	cfg.Contributions.TmpfilesRules = append(cfg.Contributions.TmpfilesRules,
		"d /home/agent/.ssh 0700 agent users -",
	)

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	// Should contain SSH key bind mounts
	if !strings.Contains(result, "/home/agent/.ssh/id_ed25519") {
		t.Error("Config should mount SSH private key")
	}
	if !strings.Contains(result, keyPath) {
		t.Error("Config should reference host SSH key path")
	}
	if !strings.Contains(result, keyPath+".pub") {
		t.Error("Config should mount SSH public key")
	}
	// SSH config should be written via init commands
	if !strings.Contains(result, "IdentityFile") {
		t.Error("Config should write SSH config with IdentityFile")
	}
	if !strings.Contains(result, "StrictHostKeyChecking accept-new") {
		t.Error("Config should set StrictHostKeyChecking")
	}
	// Should have tmpfiles rule for .ssh directory
	if !strings.Contains(result, "d /home/agent/.ssh 0700 agent users -") {
		t.Error("Config should have tmpfiles rule for .ssh directory")
	}
}

func TestGenerateNixConfig_NoIdentity(t *testing.T) {
	cfg := validTestConfig()
	// AgentIdentity is nil by default

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	if strings.Contains(result, "forage-agent-identity") {
		t.Error("Config should not contain identity service when no identity")
	}
	if strings.Contains(result, "/home/agent/.ssh") {
		t.Error("Config should not have .ssh mount when no identity")
	}
}

func TestGenerateNixConfig_SystemPromptMount(t *testing.T) {
	cfg := validTestConfig()
	// Add system prompt mount via contributions
	cfg.Contributions.Mounts = append(cfg.Contributions.Mounts, injection.Mount{
		HostPath:      "/var/lib/forage/sandboxes/test-sandbox.system-prompt.md",
		ContainerPath: "/home/agent/.config/forage/system-prompt.md",
		ReadOnly:      true,
	})
	cfg.Contributions.TmpfilesRules = append(cfg.Contributions.TmpfilesRules,
		"d /home/agent/.config/forage 0755 agent users -",
	)

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	// System prompt file should be bind-mounted read-only
	if !strings.Contains(result, "/home/agent/.config/forage/system-prompt.md") {
		t.Error("Config should mount system prompt at container path")
	}
	if !strings.Contains(result, "/var/lib/forage/sandboxes/test-sandbox.system-prompt.md") {
		t.Error("Config should reference host system prompt path")
	}
	// Should have tmpfiles rule for parent directory
	if !strings.Contains(result, "d /home/agent/.config/forage 0755 agent users -") {
		t.Error("Config should have tmpfiles rule for forage config directory")
	}
}

func TestGenerateNixConfig_SkillsMount(t *testing.T) {
	cfg := validTestConfig()
	// Add skills mount via contributions
	cfg.Contributions.Mounts = append(cfg.Contributions.Mounts, injection.Mount{
		HostPath:      "/var/lib/forage/sandboxes/test-sandbox.skills",
		ContainerPath: "/home/agent/.claude/skills",
		ReadOnly:      true,
	})
	cfg.Contributions.TmpfilesRules = append(cfg.Contributions.TmpfilesRules,
		"d /home/agent/.claude 0755 agent users -",
	)

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	// Skills directory should be bind-mounted read-only
	if !strings.Contains(result, "/home/agent/.claude/skills") {
		t.Error("Config should mount skills directory")
	}
	if !strings.Contains(result, "/var/lib/forage/sandboxes/test-sandbox.skills") {
		t.Error("Config should reference host skills path")
	}
	// Should have tmpfiles rules
	if !strings.Contains(result, "d /home/agent/.claude 0755 agent users -") {
		t.Error("Config should have tmpfiles rule for .claude directory")
	}
}

func TestGenerateNixConfig_ClaudeWrapper(t *testing.T) {
	cfg := validTestConfig()
	// Remove claude-code from packages since the wrapper replaces it
	cfg.Contributions.Packages = nil
	// Add system prompt mount via contributions - this triggers the wrapper
	cfg.Contributions.Mounts = append(cfg.Contributions.Mounts, injection.Mount{
		HostPath:      "/var/lib/forage/sandboxes/test-sandbox.system-prompt.md",
		ContainerPath: "/home/agent/.config/forage/system-prompt.md",
		ReadOnly:      true,
	})

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	// When a system-prompt.md mount exists and claude agent exists, should emit wrapper
	if !strings.Contains(result, "writeShellScriptBin") {
		t.Error("Config should contain writeShellScriptBin wrapper for claude")
	}
	if !strings.Contains(result, "--append-system-prompt") {
		t.Error("Config should contain --append-system-prompt flag")
	}
	if !strings.Contains(result, "system-prompt.md") {
		t.Error("Config should reference system prompt file in wrapper")
	}
	// Raw claude package should NOT be in systemPackages when wrapper is used
	if strings.Contains(result, "        pkgs.claude-code\n") {
		t.Error("Config should NOT include raw claude package when wrapper is used")
	}
}

func TestGenerateNixConfig_NoClaudeWrapper_WithoutPrompt(t *testing.T) {
	cfg := validTestConfig()
	// No system-prompt.md mount — claude should be added as raw package

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	if strings.Contains(result, "writeShellScriptBin") {
		t.Error("Config should NOT contain wrapper when no system prompt")
	}
	if !strings.Contains(result, "pkgs.claude-code") {
		t.Error("Config should contain raw claude package when no system prompt")
	}
}

func TestGenerateNixConfig_NonClaudeAgent_NoWrapper(t *testing.T) {
	cfg := validTestConfig()
	// Add system prompt mount but replace claude with a non-claude agent
	cfg.Contributions.Mounts = append(cfg.Contributions.Mounts, injection.Mount{
		HostPath:      "/var/lib/forage/sandboxes/test-sandbox.system-prompt.md",
		ContainerPath: "/home/agent/.config/forage/system-prompt.md",
		ReadOnly:      true,
	})
	cfg.Template.Agents = map[string]config.AgentConfig{
		"aider": {
			PackagePath: "pkgs.aider",
			SecretName:  "openai",
			AuthEnvVar:  "OPENAI_API_KEY",
		},
	}
	// Replace claude-code package with aider package
	cfg.Contributions.Packages = []injection.Package{
		{Name: "aider"},
	}

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	// Non-claude agents should not get a wrapper
	if strings.Contains(result, "writeShellScriptBin") {
		t.Error("Config should NOT contain wrapper for non-claude agent")
	}
	if !strings.Contains(result, "pkgs.aider") {
		t.Error("Config should contain raw aider package")
	}
}

func TestGenerateNixConfig_DefaultTmuxWindows(t *testing.T) {
	cfg := validTestConfig()
	cfg.Template.Agents = map[string]config.AgentConfig{
		"claude": {
			PackagePath: "pkgs.claude-code",
			SecretName:  "anthropic",
			AuthEnvVar:  "ANTHROPIC_API_KEY",
		},
		"aider": {
			PackagePath: "pkgs.aider",
			SecretName:  "openai",
			AuthEnvVar:  "OPENAI_API_KEY",
		},
	}

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	// Default: one window per agent, sorted by name (aider, claude)
	if !strings.Contains(result, "new-session -d -s forage -c /workspace -n aider") {
		t.Error("First tmux window should be 'aider' (sorted)")
	}
	if !strings.Contains(result, "new-window -t forage -n claude") {
		t.Error("Second tmux window should be 'claude' (sorted)")
	}
	if !strings.Contains(result, "send-keys -t forage:aider 'aider' Enter") {
		t.Error("Should send-keys for aider window")
	}
	if !strings.Contains(result, "send-keys -t forage:claude 'claude' Enter") {
		t.Error("Should send-keys for claude window")
	}
}

func TestGenerateNixConfig_ExplicitTmuxWindows(t *testing.T) {
	cfg := validTestConfig()
	cfg.Template.TmuxWindows = []config.TmuxWindow{
		{Name: "claude", Command: "claude"},
		{Name: "shell", Command: ""},
	}

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	// First window: claude with command
	if !strings.Contains(result, "new-session -d -s forage -c /workspace -n claude") {
		t.Error("First tmux window should be 'claude'")
	}
	if !strings.Contains(result, "send-keys -t forage:claude 'claude' Enter") {
		t.Error("Should send-keys for claude window")
	}
	// Second window: shell with no command
	if !strings.Contains(result, "new-window -t forage -n shell") {
		t.Error("Second tmux window should be 'shell'")
	}
	// Shell window has empty command — no send-keys
	if strings.Contains(result, "send-keys -t forage:shell") {
		t.Error("Should NOT send-keys for shell window (empty command)")
	}
}

func TestGenerateNixConfig_TmuxWriteShellScript(t *testing.T) {
	cfg := validTestConfig()

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	// Should use writeShellScript, not bash -c
	if !strings.Contains(result, "writeShellScript") {
		t.Error("forage-init should use writeShellScript")
	}
	if strings.Contains(result, "${pkgs.bash}/bin/bash -c") {
		t.Error("forage-init should NOT use bash -c anymore")
	}
}

func TestGenerateNixConfig_WeztermMultiplexer(t *testing.T) {
	cfg := validTestConfig()
	cfg.Mux = multiplexer.New(multiplexer.TypeWezterm)

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	// Should contain wezterm package instead of tmux
	if !strings.Contains(result, "wezterm") {
		t.Error("Config should contain wezterm package")
	}
	if strings.Contains(result, "\n        tmux\n") {
		t.Error("Config should NOT contain tmux package when using wezterm")
	}
	// Should use wezterm-mux-server in init script
	if !strings.Contains(result, "wezterm-mux-server") {
		t.Error("Config should contain wezterm-mux-server in init script")
	}
}

func TestGenerateNixConfig_DefaultMultiplexer(t *testing.T) {
	cfg := validTestConfig()
	// Multiplexer field is empty (default)

	result, err := GenerateNixConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNixConfig failed: %v", err)
	}

	// Should use tmux by default
	if !strings.Contains(result, "tmux") {
		t.Error("Config should contain tmux by default")
	}
	if !strings.Contains(result, "tmux new-session") {
		t.Error("Config should use tmux init script by default")
	}
}

// TestGenerateNixConfig_RestrictedNetwork tests restricted network mode separately
// because it involves DNS resolution which produces dynamic IP addresses.
func TestGenerateNixConfig_RestrictedNetwork(t *testing.T) {
	cfg := validTestConfig()
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

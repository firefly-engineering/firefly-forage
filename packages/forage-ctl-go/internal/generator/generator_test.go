package generator

import (
	"strings"
	"testing"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/config"
)

func TestGenerateNixConfig(t *testing.T) {
	cfg := &ContainerConfig{
		Name:        "test-sandbox",
		Port:        2200,
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
		},
		WorkspaceMode: "direct",
		NixpkgsRev:    "abc123def",
	}

	result := GenerateNixConfig(cfg)

	// Check container name
	if !strings.Contains(result, "containers.forage-test-sandbox") {
		t.Error("Config should contain container name with prefix")
	}

	// Check port forwarding
	if !strings.Contains(result, "hostPort = 2200") {
		t.Error("Config should contain port forwarding")
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

func TestGenerateNixConfig_JJMode(t *testing.T) {
	cfg := &ContainerConfig{
		Name:        "test-sandbox",
		Port:        2200,
		NetworkSlot: 1,
		Workspace:   "/var/lib/forage/workspaces/test-sandbox",
		SecretsPath: "/run/secrets/test-sandbox",
		Template: &config.Template{
			Name:    "claude",
			Network: "full",
		},
		HostConfig:    &config.HostConfig{},
		WorkspaceMode: "jj",
		SourceRepo:    "/home/user/myrepo",
	}

	result := GenerateNixConfig(cfg)

	// Check JJ bind mount
	if !strings.Contains(result, "/home/user/myrepo/.jj") {
		t.Error("Config should contain .jj bind mount for jj mode")
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
			cfg := &ContainerConfig{
				Name:        "test",
				Port:        2200,
				NetworkSlot: 1,
				Template: &config.Template{
					Network:      tt.network,
					AllowedHosts: tt.allowedHosts,
				},
				HostConfig: &config.HostConfig{},
			}

			result := GenerateNixConfig(cfg)

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
	cfg := &ContainerConfig{
		Name:        "test",
		Port:        2200,
		NetworkSlot: 1,
		Template:    &config.Template{Network: "full"},
		HostConfig:  &config.HostConfig{},
		NixpkgsRev:  "", // No revision
	}

	result := GenerateNixConfig(cfg)

	// Should not contain registry config when no revision
	if strings.Contains(result, "registry.json") {
		t.Error("Config should not contain registry config when NixpkgsRev is empty")
	}
}

func TestGenerateNixConfig_ProxyMode(t *testing.T) {
	cfg := &ContainerConfig{
		Name:        "test-sandbox",
		Port:        2200,
		NetworkSlot: 1,
		Template: &config.Template{
			Network: "full",
			Agents: map[string]config.AgentConfig{
				"claude": {
					AuthEnvVar: "ANTHROPIC_API_KEY",
					SecretName: "anthropic-api-key",
				},
			},
		},
		HostConfig: &config.HostConfig{},
		ProxyURL:   "http://10.100.1.1:8080",
	}

	result := GenerateNixConfig(cfg)

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
	if strings.Contains(result, "cat /run/secrets/anthropic-api-key") {
		t.Error("Config should not read secrets directly when proxy is enabled")
	}
}

func TestGenerateNixConfig_NoProxy(t *testing.T) {
	cfg := &ContainerConfig{
		Name:        "test-sandbox",
		Port:        2200,
		NetworkSlot: 1,
		Template: &config.Template{
			Network: "full",
			Agents: map[string]config.AgentConfig{
				"claude": {
					AuthEnvVar: "ANTHROPIC_API_KEY",
					SecretName: "anthropic-api-key",
				},
			},
		},
		HostConfig: &config.HostConfig{},
		ProxyURL:   "", // No proxy
	}

	result := GenerateNixConfig(cfg)

	// Should contain direct secret reading
	if !strings.Contains(result, "cat /run/secrets/anthropic-api-key") {
		t.Error("Config should read secrets directly when proxy is disabled")
	}
	// Should NOT contain proxy URL
	if strings.Contains(result, "ANTHROPIC_BASE_URL") {
		t.Error("Config should not contain ANTHROPIC_BASE_URL when proxy is disabled")
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

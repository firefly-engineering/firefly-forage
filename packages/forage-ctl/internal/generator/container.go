package generator

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/network"
)

// NixOSStateVersion is the NixOS state version used in generated container configs.
const NixOSStateVersion = "24.05"

// ContainerConfig holds the configuration for generating a container
type ContainerConfig struct {
	Name           string
	NetworkSlot    int
	Workspace      string
	SecretsPath    string
	AuthorizedKeys []string
	Template       *config.Template
	HostConfig     *config.HostConfig
	WorkspaceMode  string
	SourceRepo     string
	NixpkgsRev     string
	ProxyURL       string // URL of the forage-proxy server (if using proxy mode)
	UID            int    // Host user's UID for the container agent user
	GID            int    // Host user's GID for the container agent user
}

// Validate checks that the ContainerConfig has all required fields
func (c *ContainerConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("container name is required")
	}
	if c.NetworkSlot < 1 || c.NetworkSlot > 254 {
		return fmt.Errorf("invalid network slot: %d (must be 1-254)", c.NetworkSlot)
	}
	if c.Workspace == "" {
		return fmt.Errorf("workspace path is required")
	}
	// SecretsPath is optional - only needed if agents use secrets
	if len(c.AuthorizedKeys) == 0 {
		return fmt.Errorf("at least one authorized key is required")
	}
	if c.Template == nil {
		return fmt.Errorf("template is required")
	}
	if err := c.Template.Validate(); err != nil {
		return fmt.Errorf("invalid template: %w", err)
	}

	// Validate workspace mode
	validModes := map[string]bool{"": true, "direct": true, "jj": true, "git-worktree": true}
	if !validModes[c.WorkspaceMode] {
		return fmt.Errorf("invalid workspace mode: %s", c.WorkspaceMode)
	}

	// jj mode requires source repo
	if c.WorkspaceMode == "jj" && c.SourceRepo == "" {
		return fmt.Errorf("source repo is required for jj workspace mode")
	}

	return nil
}

// GenerateNixConfig generates the nix configuration for the container.
// Returns the generated config and any validation error.
func GenerateNixConfig(cfg *ContainerConfig) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", fmt.Errorf("invalid container config: %w", err)
	}

	data := buildTemplateData(cfg)

	var buf bytes.Buffer
	if err := containerTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute container template: %w", err)
	}

	return buf.String(), nil
}

// buildTemplateData constructs TemplateData from a ContainerConfig.
func buildTemplateData(cfg *ContainerConfig) *TemplateData {
	data := &TemplateData{
		ContainerName:  config.ContainerName(cfg.Name),
		NetworkSlot:    cfg.NetworkSlot,
		StateVersion:   NixOSStateVersion,
		AuthorizedKeys: cfg.AuthorizedKeys,
		NetworkConfig:  buildNetworkConfig(cfg.Template.Network, cfg.Template.AllowedHosts, cfg.NetworkSlot),
		TmuxSession:    config.TmuxSessionName,
		UID:            cfg.UID,
		GID:            cfg.GID,
	}

	// Build bind mounts
	data.BindMounts = []BindMount{
		{Path: "/nix/store", HostPath: "/nix/store", ReadOnly: true},
		{Path: "/workspace", HostPath: cfg.Workspace, ReadOnly: false},
	}

	// Add secrets mount only if secrets are configured
	if cfg.SecretsPath != "" {
		data.BindMounts = append(data.BindMounts, BindMount{
			Path:     "/run/secrets",
			HostPath: cfg.SecretsPath,
			ReadOnly: true,
		})
	}

	// Add source repo .jj and .git mounts for jj mode
	// jj needs both: .jj for jj state and .git for the git backend
	if cfg.WorkspaceMode == "jj" && cfg.SourceRepo != "" {
		jjPath := filepath.Join(cfg.SourceRepo, ".jj")
		data.BindMounts = append(data.BindMounts, BindMount{
			Path:     jjPath,
			HostPath: jjPath,
			ReadOnly: false,
		})
		gitPath := filepath.Join(cfg.SourceRepo, ".git")
		data.BindMounts = append(data.BindMounts, BindMount{
			Path:     gitPath,
			HostPath: gitPath,
			ReadOnly: false,
		})
	}

	// Add host config directory mounts for agents
	for _, agent := range cfg.Template.Agents {
		if agent.HostConfigDir != "" && agent.ContainerConfigDir != "" {
			data.BindMounts = append(data.BindMounts, BindMount{
				Path:     agent.ContainerConfigDir,
				HostPath: agent.HostConfigDir,
				ReadOnly: agent.HostConfigDirReadOnly,
			})
		}
	}

	// Build agent packages and environment variables
	for _, agent := range cfg.Template.Agents {
		if agent.PackagePath != "" {
			data.AgentPackages = append(data.AgentPackages, agent.PackagePath)
		}
		// When using proxy, don't inject secrets directly - the proxy will inject them
		if cfg.ProxyURL == "" && agent.AuthEnvVar != "" && agent.SecretName != "" {
			data.EnvVars = append(data.EnvVars, EnvVar{
				Name:  agent.AuthEnvVar,
				Value: fmt.Sprintf(`"$(cat /run/secrets/%s 2>/dev/null || echo '')"`, agent.SecretName),
			})
		}
	}

	// Add proxy configuration if enabled
	if cfg.ProxyURL != "" {
		data.EnvVars = append(data.EnvVars, EnvVar{
			Name:  "ANTHROPIC_BASE_URL",
			Value: fmt.Sprintf("%q", cfg.ProxyURL),
		})
		data.EnvVars = append(data.EnvVars, EnvVar{
			Name:  "ANTHROPIC_CUSTOM_HEADERS",
			Value: fmt.Sprintf(`"X-Forage-Sandbox: %s"`, cfg.Name),
		})
	}

	// Build registry config for nix pinning
	if cfg.NixpkgsRev != "" && cfg.NixpkgsRev != "unknown" {
		data.RegistryConfig = RegistryConfig{
			Enabled:    true,
			NixpkgsRev: cfg.NixpkgsRev,
		}
	}

	return data
}

func buildNetworkConfig(networkMode string, allowedHosts []string, slot int) string {
	cfg := &network.Config{
		Mode:         network.Mode(networkMode),
		AllowedHosts: allowedHosts,
		NetworkSlot:  slot,
	}

	// Default to full if not specified
	if cfg.Mode == "" {
		cfg.Mode = network.ModeFull
	}

	return network.GenerateNixNetworkConfig(cfg)
}

package generator

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/injection"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/multiplexer"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/network"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/reproducibility"
)

// NixOSStateVersion is the NixOS state version used in generated container configs.
const NixOSStateVersion = "24.05"

// ContainerConfig holds the configuration for generating a container.
// All mounts, packages, env vars, and tmpfiles rules come from Contributions.
type ContainerConfig struct {
	Name           string
	NetworkSlot    int
	AuthorizedKeys []string
	Template       *config.Template
	UID            int                      // Host user's UID for the container agent user
	GID            int                      // Host user's GID for the container agent user
	Mux            multiplexer.Multiplexer  // Multiplexer instance (created by caller)
	AgentIdentity  *config.AgentIdentity    // Optional agent identity for git authorship (used for Nix template)
	Runtime        string                   // Runtime backend name (e.g. "nspawn", "docker", "podman")

	// Contributions from the injection collector (required).
	// Contains all mounts, packages, env vars, and tmpfiles rules.
	Contributions *injection.Contributions

	// Reproducibility handles package resolution (required).
	// Used to resolve Package{Name, Version} to Nix expressions.
	Reproducibility reproducibility.Reproducibility
}

// Validate checks that the ContainerConfig has all required fields
func (c *ContainerConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("container name is required")
	}
	if c.NetworkSlot < 1 || c.NetworkSlot > 254 {
		return fmt.Errorf("invalid network slot: %d (must be 1-254)", c.NetworkSlot)
	}
	if len(c.AuthorizedKeys) == 0 {
		return fmt.Errorf("at least one authorized key is required")
	}
	if c.Template == nil {
		return fmt.Errorf("template is required")
	}
	if err := c.Template.Validate(); err != nil {
		return fmt.Errorf("invalid template: %w", err)
	}
	if c.Contributions == nil {
		return fmt.Errorf("contributions is required")
	}
	if c.Reproducibility == nil {
		return fmt.Errorf("reproducibility is required")
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
// All mounts, packages, env vars, and tmpfiles rules come from Contributions.
func buildTemplateData(cfg *ContainerConfig) *TemplateData {
	data := &TemplateData{
		ContainerName:  config.ContainerNameForSlot(cfg.NetworkSlot),
		Hostname:       cfg.Name,
		NetworkSlot:    cfg.NetworkSlot,
		StateVersion:   NixOSStateVersion,
		AuthorizedKeys: cfg.AuthorizedKeys,
		NetworkConfig:  buildNetworkConfig(cfg.Template.Network, cfg.Template.AllowedHosts, cfg.NetworkSlot),
		UID:            cfg.UID,
		GID:            cfg.GID,
		SandboxName:    cfg.Name,
		Runtime:        cfg.Runtime,
	}

	// Use provided multiplexer
	mux := cfg.Mux
	if mux == nil {
		mux = multiplexer.New(multiplexer.TypeTmux) // Default fallback
	}
	data.MuxPackages = mux.NixPackages()

	// Compute windows: use explicit config if set, else one window per agent
	var windows []multiplexer.Window
	if len(cfg.Template.TmuxWindows) > 0 {
		for _, w := range cfg.Template.TmuxWindows {
			windows = append(windows, multiplexer.Window{Name: w.Name, Command: w.Command})
		}
	} else {
		names := make([]string, 0, len(cfg.Template.Agents))
		for name := range cfg.Template.Agents {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			windows = append(windows, multiplexer.Window{Name: name, Command: name})
		}
	}
	data.MuxInitScript = mux.InitScript(windows)

	// Apply all contributions (mounts, packages, env vars, tmpfiles rules)
	if cfg.Contributions != nil {
		applyContributions(data, cfg.Contributions, cfg.Reproducibility)
	}

	// Set identity fields from AgentIdentity (for Nix template)
	if cfg.AgentIdentity != nil {
		data.GitUser = cfg.AgentIdentity.GitUser
		data.GitEmail = cfg.AgentIdentity.GitEmail
		if cfg.AgentIdentity.SSHKeyPath != "" {
			data.SSHKeyName = filepath.Base(cfg.AgentIdentity.SSHKeyPath)
		}
	}

	// Check for system prompt file in mounts (for Claude wrapper)
	for _, m := range data.BindMounts {
		if strings.HasSuffix(m.Path, "system-prompt.md") {
			data.SystemPromptFile = m.Path
			// Find Claude package for wrapper
			for name, agent := range cfg.Template.Agents {
				if name == "claude" && agent.PackagePath != "" {
					data.ClaudePackagePath = agent.PackagePath
					break
				}
			}
			break
		}
	}

	// When wrapping Claude, remove the raw package from AgentPackages
	// to avoid buildEnv collision (both provide /bin/claude)
	if data.ClaudePackagePath != "" {
		resolved := "pkgs." + data.ClaudePackagePath
		filtered := data.AgentPackages[:0]
		for _, pkg := range data.AgentPackages {
			if pkg != resolved {
				filtered = append(filtered, pkg)
			}
		}
		data.AgentPackages = filtered
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

// applyContributions populates template data from the injection contributions.
// This is the primary source for mounts, packages, env vars, and tmpfiles rules.
func applyContributions(data *TemplateData, contributions *injection.Contributions, repro reproducibility.Reproducibility) {
	// Add all contributed mounts
	for _, m := range contributions.Mounts {
		data.BindMounts = append(data.BindMounts, BindMount{
			Path:     m.ContainerPath,
			HostPath: m.HostPath,
			ReadOnly: m.ReadOnly,
		})
	}

	// Add all contributed environment variables
	for _, e := range contributions.EnvVars {
		data.EnvVars = append(data.EnvVars, EnvVar{
			Name:  e.Name,
			Value: e.Value,
		})
	}

	// Add contributed tmpfiles rules (deduplicated)
	seen := make(map[string]bool)
	for _, r := range contributions.TmpfilesRules {
		if !seen[r] {
			data.ExtraTmpfilesRules = append(data.ExtraTmpfilesRules, r)
			seen[r] = true
		}
	}

	// Resolve and add contributed packages
	if repro != nil {
		existingPkgs := make(map[string]bool)
		for _, p := range data.MuxPackages {
			existingPkgs[p] = true
		}
		for _, pkg := range contributions.Packages {
			resolved, err := repro.ResolvePackage(pkg)
			if err != nil {
				// Skip packages that can't be resolved
				continue
			}
			if !existingPkgs[resolved] {
				data.AgentPackages = append(data.AgentPackages, resolved)
				existingPkgs[resolved] = true
			}
		}
	}
}

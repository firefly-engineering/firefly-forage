package generator

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/injection"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/multiplexer"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/network"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/reproducibility"
)

// ContainerConfig holds the configuration for generating a container.
// All mounts, packages, env vars, and tmpfiles rules come from Contributions.
type ContainerConfig struct {
	Name           string
	NetworkSlot    int
	AuthorizedKeys []string
	Template       *config.Template
	UID            int                     // Host user's UID for the container agent user
	GID            int                     // Host user's GID for the container agent user
	Mux            multiplexer.Multiplexer // Multiplexer instance (created by caller)
	AgentIdentity  *config.AgentIdentity   // Optional agent identity for git authorship (used for Nix template)
	Runtime        string                  // Runtime backend name (e.g. "nspawn", "docker", "podman")

	// Container user/path configuration (defaults applied if empty)
	Username     string // Container username (default: "agent")
	WorkspaceDir string // Container workspace path (default: "/workspace")
	StateVersion string // NixOS state version (default: "24.11")

	// ResourceLimits are optional cgroup limits for the container.
	ResourceLimits *config.ResourceLimits

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
	username := cfg.Username
	if username == "" {
		username = "agent"
	}
	workspaceDir := cfg.WorkspaceDir
	if workspaceDir == "" {
		workspaceDir = "/workspace"
	}
	stateVersion := cfg.StateVersion
	if stateVersion == "" {
		stateVersion = "24.11"
	}

	data := &TemplateData{
		ContainerName:  config.ContainerNameForSlot(cfg.NetworkSlot),
		Hostname:       cfg.Name,
		NetworkSlot:    cfg.NetworkSlot,
		StateVersion:   stateVersion,
		Username:       username,
		HomeDir:        "/home/" + username,
		WorkspaceDir:   workspaceDir,
		AuthorizedKeys: cfg.AuthorizedKeys,
		NetworkConfig:  buildNetworkConfig(cfg.Template.Network, cfg.Template.AllowedHosts, cfg.NetworkSlot),
		UID:            cfg.UID,
		GID:            cfg.GID,
		SandboxName:    cfg.Name,
		Runtime:        cfg.Runtime,
	}

	// Set resource limits if configured
	if cfg.ResourceLimits != nil && !cfg.ResourceLimits.IsEmpty() {
		data.ResourceLimits = cfg.ResourceLimits
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

	resolveClaudeWrapper(data, cfg)

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

// resolveClaudeWrapper detects whether a system-prompt.md mount exists and, if
// the template includes a "claude" agent, configures a shell-script wrapper
// that passes --append-system-prompt. The raw Claude package is removed from
// AgentPackages to avoid a buildEnv collision (both provide /bin/claude).
func resolveClaudeWrapper(data *TemplateData, cfg *ContainerConfig) {
	for _, m := range data.BindMounts {
		if strings.HasSuffix(m.Path, "system-prompt.md") {
			data.SystemPromptFile = m.Path
			for name, agent := range cfg.Template.Agents {
				if name == "claude" && agent.PackagePath != "" {
					data.ClaudePackagePath = agent.PackagePath
					break
				}
			}
			break
		}
	}

	if data.ClaudePackagePath == "" {
		return
	}

	resolved := "pkgs." + data.ClaudePackagePath
	filtered := data.AgentPackages[:0]
	for _, pkg := range data.AgentPackages {
		if pkg != resolved {
			filtered = append(filtered, pkg)
		}
	}
	data.AgentPackages = filtered
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
				logging.Warn("skipping unresolvable package", "package", pkg.Name, "error", err)
				continue
			}
			if !existingPkgs[resolved] {
				data.AgentPackages = append(data.AgentPackages, resolved)
				existingPkgs[resolved] = true
			}
		}
	}
}

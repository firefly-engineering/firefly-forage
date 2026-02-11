package generator

import (
	"bytes"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/injection"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/multiplexer"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/network"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/reproducibility"
)

// NixOSStateVersion is the NixOS state version used in generated container configs.
const NixOSStateVersion = "24.05"

// backendRepoMounts maps workspace modes to source repo directories
// that must be mounted at their original paths for the VCS to function.
//
// Deprecated: Use workspace backend's ContributeMounts method instead.
// This map is kept for backward compatibility.
var backendRepoMounts = map[string][]string{
	"jj": {".jj", ".git"},
	// git-worktree needs no extra mounts — .git file is in the worktree already
}

// agentProjectDirs maps agent names to source repo directories they need
// mounted into /workspace. These are typically git-ignored directories that
// don't appear in worktrees/workspaces.
//
// Deprecated: Use agent's ContributeMounts method instead.
// This map is kept for backward compatibility.
var agentProjectDirs = map[string][]string{
	"claude": {".claude"},
}

// agentHomeFiles maps agent names to files in the host user's home directory
// that should be bind-mounted into the container agent's home.
//
// Deprecated: Use agent's ContributeMounts method instead.
// This map is kept for backward compatibility.
var agentHomeFiles = map[string][]string{
	"claude": {".claude.json"},
}

// PermissionsMount represents a permissions settings file to bind-mount into the container.
type PermissionsMount struct {
	HostPath      string
	ContainerPath string
}

// ContainerConfig holds the configuration for generating a container
type ContainerConfig struct {
	Name              string
	NetworkSlot       int
	Workspace         string
	SecretsPath       string
	AuthorizedKeys    []string
	Template          *config.Template
	HostConfig        *config.HostConfig
	WorkspaceMode     string
	SourceRepo        string
	ProxyURL          string             // URL of the forage-proxy server (if using proxy mode)
	UID               int                // Host user's UID for the container agent user
	GID               int                // Host user's GID for the container agent user
	NoMuxConfig bool                      // Skip mounting host mux config into the container
	Mux         multiplexer.Multiplexer  // Multiplexer instance (created by caller)
	PermissionsMounts []PermissionsMount // Agent permissions settings files to bind-mount
	AgentIdentity     *config.AgentIdentity // Optional agent identity for git authorship and SSH key
	SystemPromptPath  string             // Host path to .system-prompt.md file (may be empty)
	SkillsPath        string             // Host path to .skills/ directory (may be empty)

	// Contributions from the injection collector (optional).
	// When set, contributions are used in addition to the legacy code paths.
	// This allows for gradual migration to the new injection system.
	Contributions *injection.Contributions

	// Reproducibility handles package resolution (optional).
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
		UID:            cfg.UID,
		GID:            cfg.GID,
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

	// Mount VCS directories required by the workspace backend
	if cfg.SourceRepo != "" {
		for _, dir := range backendRepoMounts[cfg.WorkspaceMode] {
			fullPath := filepath.Join(cfg.SourceRepo, dir)
			data.BindMounts = append(data.BindMounts, BindMount{
				Path:     fullPath,
				HostPath: fullPath,
				ReadOnly: false,
			})
		}
	}

	// Mount agent-specific project directories from source repo
	if (cfg.WorkspaceMode == "jj" || cfg.WorkspaceMode == "git-worktree") && cfg.SourceRepo != "" {
		mounted := map[string]bool{}
		for name := range cfg.Template.Agents {
			for _, dir := range agentProjectDirs[name] {
				if mounted[dir] {
					continue
				}
				fullPath := filepath.Join(cfg.SourceRepo, dir)
				if info, err := os.Stat(fullPath); err == nil && info.IsDir() {
					data.BindMounts = append(data.BindMounts, BindMount{
						Path:     filepath.Join("/workspace", dir),
						HostPath: fullPath,
						ReadOnly: false,
					})
					mounted[dir] = true
				}
			}
		}
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

	// Mount agent-specific home directory files from host
	if cfg.HostConfig != nil {
		homeDir := resolveUserHome(cfg.HostConfig.User)
		if homeDir != "" {
			mounted := map[string]bool{}
			for name := range cfg.Template.Agents {
				for _, file := range agentHomeFiles[name] {
					if mounted[file] {
						continue
					}
					hostPath := filepath.Join(homeDir, file)
					if _, err := os.Stat(hostPath); err == nil {
						data.BindMounts = append(data.BindMounts, BindMount{
							Path:     filepath.Join("/home/agent", file),
							HostPath: hostPath,
							ReadOnly: false,
						})
						mounted[file] = true
					}
				}
			}
		}
	}

	// Detect and mount host multiplexer config
	if !cfg.NoMuxConfig && cfg.HostConfig != nil {
		homeDir := resolveUserHome(cfg.HostConfig.User)
		if homeDir != "" {
			for _, cm := range mux.HostConfigMounts(homeDir) {
				data.BindMounts = append(data.BindMounts, BindMount{
					Path:     cm.ContainerPath,
					HostPath: cm.HostPath,
					ReadOnly: cm.ReadOnly,
				})
			}
		}
	}

	// Add permissions settings file mounts
	for _, pm := range cfg.PermissionsMounts {
		data.BindMounts = append(data.BindMounts, BindMount{
			Path:     pm.ContainerPath,
			HostPath: pm.HostPath,
			ReadOnly: true,
		})
		dir := filepath.Dir(pm.ContainerPath)
		data.ExtraTmpfilesRules = append(data.ExtraTmpfilesRules,
			fmt.Sprintf("d %s 0755 root root -", dir))
	}

	// Mount system prompt file
	if cfg.SystemPromptPath != "" {
		containerPromptPath := "/home/agent/.config/forage/system-prompt.md"
		data.BindMounts = append(data.BindMounts, BindMount{
			Path:     containerPromptPath,
			HostPath: cfg.SystemPromptPath,
			ReadOnly: true,
		})
		data.ExtraTmpfilesRules = append(data.ExtraTmpfilesRules,
			"d /home/agent/.config/forage 0755 agent users -")
		data.SystemPromptFile = containerPromptPath
	}

	// Mount skills directory
	if cfg.SkillsPath != "" {
		data.BindMounts = append(data.BindMounts, BindMount{
			Path:     "/home/agent/.claude/skills",
			HostPath: cfg.SkillsPath,
			ReadOnly: true,
		})
		data.ExtraTmpfilesRules = append(data.ExtraTmpfilesRules,
			"d /home/agent/.claude 0755 agent users -",
			"d /home/agent/.claude/skills 0755 agent users -")
	}

	// Build agent packages and environment variables
	for name, agent := range cfg.Template.Agents {
		if agent.PackagePath != "" {
			// For claude agent with a system prompt, use a wrapper instead of the raw package
			if name == "claude" && cfg.SystemPromptPath != "" {
				data.ClaudePackagePath = agent.PackagePath
			} else {
				data.AgentPackages = append(data.AgentPackages, agent.PackagePath)
			}
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

	// Agent identity: git config and optional SSH key
	if cfg.AgentIdentity != nil {
		data.GitUser = cfg.AgentIdentity.GitUser
		data.GitEmail = cfg.AgentIdentity.GitEmail

		if cfg.AgentIdentity.SSHKeyPath != "" {
			keyName := filepath.Base(cfg.AgentIdentity.SSHKeyPath)
			data.SSHKeyName = keyName

			// Bind-mount private key and .pub companion into /home/agent/.ssh/
			data.BindMounts = append(data.BindMounts,
				BindMount{
					Path:     "/home/agent/.ssh/" + keyName,
					HostPath: cfg.AgentIdentity.SSHKeyPath,
					ReadOnly: true,
				},
				BindMount{
					Path:     "/home/agent/.ssh/" + keyName + ".pub",
					HostPath: cfg.AgentIdentity.SSHKeyPath + ".pub",
					ReadOnly: true,
				},
			)

			// Ensure .ssh directory exists with correct ownership
			data.ExtraTmpfilesRules = append(data.ExtraTmpfilesRules,
				"d /home/agent/.ssh 0700 agent users -")
		}
	}

	// Apply contributions from the injection collector (if available)
	applyContributions(data, cfg.Contributions, cfg.Reproducibility)

	return data
}

// resolveUserHome returns the home directory for the given username.
// Returns empty string if the user cannot be looked up.
func resolveUserHome(username string) string {
	u, err := user.Lookup(username)
	if err != nil {
		return ""
	}
	return u.HomeDir
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

// applyContributions adds contributions from the injection collector to template data.
// This is called after the legacy code paths have populated the initial data.
func applyContributions(data *TemplateData, contributions *injection.Contributions, repro reproducibility.Reproducibility) {
	if contributions == nil {
		return
	}

	// Add contributed mounts (deduplicated by container path)
	existingMounts := make(map[string]bool)
	for _, m := range data.BindMounts {
		existingMounts[m.Path] = true
	}
	for _, m := range contributions.Mounts {
		if !existingMounts[m.ContainerPath] {
			data.BindMounts = append(data.BindMounts, BindMount{
				Path:     m.ContainerPath,
				HostPath: m.HostPath,
				ReadOnly: m.ReadOnly,
			})
			existingMounts[m.ContainerPath] = true
		}
	}

	// Add contributed environment variables (deduplicated by name)
	existingEnvVars := make(map[string]bool)
	for _, e := range data.EnvVars {
		existingEnvVars[e.Name] = true
	}
	for _, e := range contributions.EnvVars {
		if !existingEnvVars[e.Name] {
			data.EnvVars = append(data.EnvVars, EnvVar{
				Name:  e.Name,
				Value: e.Value,
			})
			existingEnvVars[e.Name] = true
		}
	}

	// Add contributed tmpfiles rules (deduplicated)
	existingRules := make(map[string]bool)
	for _, r := range data.ExtraTmpfilesRules {
		existingRules[r] = true
	}
	for _, r := range contributions.TmpfilesRules {
		if !existingRules[r] {
			data.ExtraTmpfilesRules = append(data.ExtraTmpfilesRules, r)
			existingRules[r] = true
		}
	}

	// Resolve and add contributed packages
	if repro != nil && len(contributions.Packages) > 0 {
		existingPkgs := make(map[string]bool)
		for _, p := range data.AgentPackages {
			existingPkgs[p] = true
		}
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

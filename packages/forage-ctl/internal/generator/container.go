package generator

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/network"
)

// NixOSStateVersion is the NixOS state version used in generated container configs.
const NixOSStateVersion = "24.05"

// ContainerConfig holds the configuration for generating a container
type ContainerConfig struct {
	Name           string
	Port           int
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
}

// GenerateNixConfig generates the nix configuration for the container
func GenerateNixConfig(cfg *ContainerConfig) string {
	containerName := config.ContainerName(cfg.Name)

	// Build bind mounts
	bindMounts := fmt.Sprintf(`
      "/nix/store" = { hostPath = "/nix/store"; isReadOnly = true; };
      "/workspace" = { hostPath = "%s"; isReadOnly = false; };
      "/run/secrets" = { hostPath = "%s"; isReadOnly = true; };`,
		cfg.Workspace, cfg.SecretsPath)

	// Add source repo .jj mount for jj mode
	if cfg.WorkspaceMode == "jj" && cfg.SourceRepo != "" {
		jjPath := filepath.Join(cfg.SourceRepo, ".jj")
		bindMounts += fmt.Sprintf(`
      "%s" = { hostPath = "%s"; isReadOnly = false; };`,
			jjPath, jjPath)
	}

	// Build authorized keys
	authKeys := ""
	for _, key := range cfg.AuthorizedKeys {
		authKeys += fmt.Sprintf("        %q\n", key)
	}

	// Build network config
	networkConfig := buildNetworkConfig(cfg.Template.Network, cfg.Template.AllowedHosts, cfg.NetworkSlot)

	// Build agent packages and environment
	agentConfig := buildAgentConfig(cfg.Template.Agents, cfg.Name, cfg.ProxyURL)

	// Build registry config for nix pinning
	registryConfig := ""
	if cfg.NixpkgsRev != "" && cfg.NixpkgsRev != "unknown" {
		registryConfig = fmt.Sprintf(`
    environment.etc."nix/registry.json".text = builtins.toJSON {
      version = 2;
      flakes = [{
        exact = true;
        from = { id = "nixpkgs"; type = "indirect"; };
        to = { type = "github"; owner = "NixOS"; repo = "nixpkgs"; rev = "%s"; };
      }];
    };`, cfg.NixpkgsRev)
	}

	return fmt.Sprintf(`{ pkgs, ... }: {
  containers.%s = {
    autoStart = true;
    ephemeral = true;
    privateNetwork = true;
    hostAddress = "10.100.%d.1";
    localAddress = "10.100.%d.2";

    bindMounts = {%s
    };

    config = { pkgs, ... }: {
      system.stateVersion = "%s";
      %s
      users.users.agent = {
        isNormalUser = true;
        home = "/home/agent";
        extraGroups = [ "wheel" ];
        openssh.authorizedKeys.keys = [
%s      ];
      };

      security.sudo.wheelNeedsPassword = false;

      services.openssh = {
        enable = true;
        ports = [ 22 ];
        settings = {
          PasswordAuthentication = false;
          PermitRootLogin = "no";
        };
      };

      environment.systemPackages = with pkgs; [
        git
        jujutsu
        tmux
        neovim
        ripgrep
        fd
        %s
      ];

      %s
      %s

      networking.firewall.allowedTCPPorts = [ 22 ];

      systemd.services.forage-init = {
        description = "Forage Sandbox Initialization";
        wantedBy = [ "multi-user.target" ];
        after = [ "network.target" ];
        serviceConfig = {
          Type = "oneshot";
          User = "agent";
          WorkingDirectory = "/workspace";
          ExecStart = "${pkgs.bash}/bin/bash -c 'tmux new-session -d -s %s || true'";
        };
      };
    };

    forwardPorts = [
      { containerPort = 22; hostPort = %d; protocol = "tcp"; }
    ];
  };
}
`,
		containerName,
		cfg.NetworkSlot, cfg.NetworkSlot,
		bindMounts,
		NixOSStateVersion,
		networkConfig,
		authKeys,
		agentConfig.packages,
		agentConfig.environment,
		registryConfig,
		config.TmuxSessionName,
		cfg.Port,
	)
}

type agentConfigResult struct {
	packages    string
	environment string
}

func buildAgentConfig(agents map[string]config.AgentConfig, sandboxName string, proxyURL string) agentConfigResult {
	var packages []string
	var envVars []string

	for _, agent := range agents {
		if agent.PackagePath != "" {
			packages = append(packages, agent.PackagePath)
		}
		// When using proxy, don't inject secrets directly - the proxy will inject them
		if proxyURL == "" && agent.AuthEnvVar != "" && agent.SecretName != "" {
			envVars = append(envVars, fmt.Sprintf(`%s = "$(cat /run/secrets/%s 2>/dev/null || echo '')"`,
				agent.AuthEnvVar, agent.SecretName))
		}
	}

	// Add proxy configuration if enabled
	if proxyURL != "" {
		envVars = append(envVars, fmt.Sprintf(`ANTHROPIC_BASE_URL = %q`, proxyURL))
		// Add custom header to identify the sandbox
		envVars = append(envVars, fmt.Sprintf(`ANTHROPIC_CUSTOM_HEADERS = "X-Forage-Sandbox: %s"`, sandboxName))
	}

	envConfig := ""
	if len(envVars) > 0 {
		envConfig = fmt.Sprintf(`
      environment.sessionVariables = {
        %s
      };`, strings.Join(envVars, "\n        "))
	}

	return agentConfigResult{
		packages:    strings.Join(packages, "\n        "),
		environment: envConfig,
	}
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

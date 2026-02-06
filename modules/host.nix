{ self, extra-container, nixpkgs }:
{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.services.firefly-forage;

  inherit (lib)
    mkEnableOption
    mkOption
    types
    mkIf
    mapAttrs
    replaceStrings
    hasPrefix
    ;

  # Resolve ~ to the configured user's home directory
  userHome = config.users.users.${cfg.user}.home or "/home/${cfg.user}";
  resolveTilde = path:
    if path == null then null
    else if hasPrefix "~/" path then
      userHome + (builtins.substring 1 (builtins.stringLength path - 1) path)
    else if path == "~" then
      userHome
    else
      path;

  # Derive container config dir from host path if not specified
  # e.g., ~/.claude -> /home/agent/.claude
  deriveContainerPath = hostPath:
    let
      baseName = baseNameOf hostPath;
    in
      "/home/agent/${baseName}";

  # Agent definition type
  agentType = types.submodule {
    options = {
      package = mkOption {
        type = types.package;
        description = "The agent package to use";
      };

      secretName = mkOption {
        type = types.nullOr types.str;
        default = null;
        description = "Name of the secret (key in services.firefly-forage.secrets). Optional if using hostConfigDir for credentials.";
      };

      authEnvVar = mkOption {
        type = types.nullOr types.str;
        default = null;
        description = "Environment variable name for the auth token. Optional if using hostConfigDir for credentials.";
        example = "ANTHROPIC_API_KEY";
      };

      hostConfigDir = mkOption {
        type = types.nullOr types.str;
        default = null;
        description = "Host directory to mount for persistent agent configuration (supports ~ expansion)";
        example = "~/.claude";
      };

      containerConfigDir = mkOption {
        type = types.nullOr types.str;
        default = null;
        description = "Override container mount point (default: /home/agent/.<dirname>)";
        example = "/home/agent/.claude";
      };

      hostConfigDirReadOnly = mkOption {
        type = types.bool;
        default = false;
        description = "Mount the config directory as read-only (default: false to allow token refresh)";
      };
    };
  };

  # Template definition type
  templateType = types.submodule {
    options = {
      description = mkOption {
        type = types.str;
        default = "";
        description = "Human-readable description of this template";
      };

      agents = mkOption {
        type = types.attrsOf agentType;
        default = { };
        description = "Agents available in this sandbox";
      };

      extraPackages = mkOption {
        type = types.listOf types.package;
        default = [ ];
        description = "Additional packages to include in the sandbox";
      };

      network = mkOption {
        type = types.enum [
          "full"
          "restricted"
          "none"
        ];
        default = "full";
        description = ''
          Network access mode:
          - full: Unrestricted internet access
          - restricted: Only allowed hosts can be accessed
          - none: No network access
        '';
      };

      allowedHosts = mkOption {
        type = types.listOf types.str;
        default = [ ];
        description = "Allowed hosts when network = restricted";
      };
    };
  };

in
{
  options.services.firefly-forage = {
    enable = mkEnableOption "Firefly Forage AI sandbox system";

    user = mkOption {
      type = types.str;
      description = "Host user whose UID/GID will be used in sandboxes";
      example = "myuser";
    };

    authorizedKeys = mkOption {
      type = types.listOf types.str;
      default = [ ];
      description = "SSH public keys authorized to access sandboxes";
    };

    portRange = {
      from = mkOption {
        type = types.port;
        default = 2200;
        description = "Start of port range for sandbox SSH";
      };

      to = mkOption {
        type = types.port;
        default = 2299;
        description = "End of port range for sandbox SSH";
      };
    };

    secrets = mkOption {
      type = types.attrsOf types.path;
      default = { };
      description = "Mapping of secret names to file paths containing API keys";
      example = {
        anthropic = "/run/secrets/anthropic-api-key";
        openai = "/run/secrets/openai-api-key";
      };
    };

    templates = mkOption {
      type = types.attrsOf templateType;
      default = { };
      description = "Sandbox templates";
    };

    stateDir = mkOption {
      type = types.path;
      default = "/var/lib/firefly-forage";
      description = "Directory for forage state and generated configs";
    };

    externalInterface = mkOption {
      type = types.nullOr types.str;
      default = null;
      description = ''
        External network interface for NAT. If null, NAT configuration
        is skipped (useful when using an existing NAT setup or when
        the interface name differs from the default).
      '';
      example = "eth0";
    };
  };

  # Import extra-container module at the module level
  imports = [ extra-container.nixosModules.default ];

  config = mkIf cfg.enable {
    # Validate configuration
    assertions = [
      {
        assertion = cfg.user != "";
        message = "services.firefly-forage.user must be specified";
      }
      {
        assertion = cfg.portRange.from <= cfg.portRange.to;
        message = "services.firefly-forage.portRange.from (${toString cfg.portRange.from}) must be <= portRange.to (${toString cfg.portRange.to})";
      }
      {
        assertion = cfg.portRange.to - cfg.portRange.from >= 9;
        message = "services.firefly-forage.portRange must allow at least 10 ports";
      }
    ] ++ lib.flatten (lib.mapAttrsToList (
      templateName: template:
        lib.mapAttrsToList (
          agentName: agent:
            # Only validate secret reference if secretName is specified
            lib.optional (agent.secretName != null) {
              assertion = cfg.secrets ? ${agent.secretName};
              message = "Template '${templateName}' agent '${agentName}' references secret '${agent.secretName}' which is not defined in services.firefly-forage.secrets";
            }
        ) template.agents
    ) cfg.templates);

    # Ensure state directory exists
    systemd.tmpfiles.rules = [
      "d ${cfg.stateDir} 0750 root root -"
      "d ${cfg.stateDir}/sandboxes 0750 root root -"
      "d ${cfg.stateDir}/workspaces 0750 root root -"
      "d /run/forage-secrets 0700 root root -"
    ];

    # Install forage-ctl
    environment.systemPackages = [
      self.packages.${pkgs.system}.forage-ctl
    ];

    # Enable NAT for container networking (only if externalInterface is set)
    networking.nat = mkIf (cfg.externalInterface != null) {
      enable = true;
      internalInterfaces = [ "ve-+" ];
      externalInterface = cfg.externalInterface;
    };

    # Generate host configuration file and template configurations
    environment.etc =
      {
        "firefly-forage/config.json" = {
          text = builtins.toJSON {
            user = cfg.user;
            portRange = {
              from = cfg.portRange.from;
              to = cfg.portRange.to;
            };
            authorizedKeys = cfg.authorizedKeys;
            secrets = cfg.secrets;
            stateDir = cfg.stateDir;
            # Path to extra-container command
            extraContainerPath = "${extra-container.packages.${pkgs.system}.default}/bin/extra-container";
            # Nixpkgs revision for registry pinning
            nixpkgsRev = nixpkgs.rev or "unknown";
          };
        };
      }
      // mapAttrs (
        name: template: {
          target = "firefly-forage/templates/${name}.json";
          text = builtins.toJSON {
            inherit name;
            inherit (template)
              description
              network
              allowedHosts
              ;
            agents = mapAttrs (
              agentName: agent:
              let
                resolvedHostConfigDir = resolveTilde agent.hostConfigDir;
                resolvedContainerConfigDir =
                  if agent.containerConfigDir != null then agent.containerConfigDir
                  else if resolvedHostConfigDir != null then deriveContainerPath resolvedHostConfigDir
                  else null;
              in {
                inherit (agent) secretName authEnvVar hostConfigDirReadOnly;
                packagePath = agent.package.outPath;
                hostConfigDir = resolvedHostConfigDir;
                containerConfigDir = resolvedContainerConfigDir;
              }
            ) template.agents;
            extraPackages = map (p: p.outPath) template.extraPackages;
          };
        }
      ) cfg.templates;
  };
}

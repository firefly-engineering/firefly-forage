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

      permissions = mkOption {
        type = types.nullOr (types.submodule {
          options = {
            skipAll = mkOption {
              type = types.bool;
              default = false;
              description = "Bypass all permission checks";
            };
            allow = mkOption {
              type = types.listOf types.str;
              default = [];
              description = "Permission rules to auto-approve (agent-specific format)";
              example = [ "Bash(npm run *)" "Edit" "Read" ];
            };
            deny = mkOption {
              type = types.listOf types.str;
              default = [];
              description = "Permission rules to always block";
              example = [ "Bash(rm -rf *)" ];
            };
          };
        });
        default = null;
        description = "Agent permission rules. When null, no permission settings are generated.";
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

      agentIdentity = {
        gitUser = mkOption {
          type = types.nullOr types.str;
          default = null;
          description = "Default git user.name for agents in sandboxes using this template";
          example = "Template Agent";
        };

        gitEmail = mkOption {
          type = types.nullOr types.str;
          default = null;
          description = "Default git user.email for agents in sandboxes using this template";
          example = "template-agent@example.com";
        };

        sshKeyPath = mkOption {
          type = types.nullOr types.path;
          default = null;
          description = "Path to SSH private key for agent push access in this template";
          example = "/run/secrets/template-agent-ssh-key";
        };
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

    agentIdentity = {
      gitUser = mkOption {
        type = types.nullOr types.str;
        default = null;
        description = "Default git user.name for agent commits in sandboxes";
        example = "Forage Agent";
      };

      gitEmail = mkOption {
        type = types.nullOr types.str;
        default = null;
        description = "Default git user.email for agent commits in sandboxes";
        example = "agent@example.com";
      };

      sshKeyPath = mkOption {
        type = types.nullOr types.path;
        default = null;
        description = "Path to SSH private key for agent push access (e.g. sops-nix secret path)";
        example = "/run/secrets/agent-ssh-key";
      };
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
    # The configured user needs access to sandboxes and workspaces directories
    systemd.tmpfiles.rules = [
      "d ${cfg.stateDir} 0755 ${cfg.user} root -"
      "d ${cfg.stateDir}/sandboxes 0755 ${cfg.user} root -"
      "d ${cfg.stateDir}/workspaces 0755 ${cfg.user} root -"
      "d /run/forage-secrets 0700 root root -"
    ];

    # Install forage-ctl
    environment.systemPackages = [
      self.packages.${pkgs.stdenv.hostPlatform.system}.forage-ctl
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
          text = builtins.toJSON ({
            user = cfg.user;
            uid = config.users.users.${cfg.user}.uid;
            gid = config.users.groups.${config.users.users.${cfg.user}.group}.gid;
            authorizedKeys = cfg.authorizedKeys;
            secrets = cfg.secrets;
            stateDir = cfg.stateDir;
            # Path to extra-container command
            extraContainerPath = "${extra-container.packages.${pkgs.stdenv.hostPlatform.system}.default}/bin/extra-container";
            # Nixpkgs path for extra-container --nixpkgs-path
            nixpkgsPath = "${pkgs.path}";
            # Nixpkgs revision for registry pinning
            nixpkgsRev = nixpkgs.rev or "unknown";
          } // lib.optionalAttrs (
            cfg.agentIdentity.gitUser != null
            || cfg.agentIdentity.gitEmail != null
            || cfg.agentIdentity.sshKeyPath != null
          ) {
            agentIdentity = lib.filterAttrs (_: v: v != null) {
              gitUser = cfg.agentIdentity.gitUser;
              gitEmail = cfg.agentIdentity.gitEmail;
              sshKeyPath =
                if cfg.agentIdentity.sshKeyPath != null
                then resolveTilde (toString cfg.agentIdentity.sshKeyPath)
                else null;
            };
          });
        };
      }
      // mapAttrs (
        name: template: {
          target = "firefly-forage/templates/${name}.json";
          text = builtins.toJSON ({
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
                packagePath = agent.package.pname;
                hostConfigDir = resolvedHostConfigDir;
                containerConfigDir = resolvedContainerConfigDir;
                permissions =
                  if agent.permissions != null then {
                    inherit (agent.permissions) skipAll allow deny;
                  } else null;
              }
            ) template.agents;
            extraPackages = map (p: p.pname) template.extraPackages;
          } // lib.optionalAttrs (
            template.agentIdentity.gitUser != null
            || template.agentIdentity.gitEmail != null
            || template.agentIdentity.sshKeyPath != null
          ) {
            agentIdentity = lib.filterAttrs (_: v: v != null) {
              gitUser = template.agentIdentity.gitUser;
              gitEmail = template.agentIdentity.gitEmail;
              sshKeyPath =
                if template.agentIdentity.sshKeyPath != null
                then resolveTilde (toString template.agentIdentity.sshKeyPath)
                else null;
            };
          });
        }
      ) cfg.templates;
  };
}

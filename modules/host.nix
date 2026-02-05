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
    ;

  # Agent definition type
  agentType = types.submodule {
    options = {
      package = mkOption {
        type = types.package;
        description = "The agent package to use";
      };

      secretName = mkOption {
        type = types.str;
        description = "Name of the secret (key in services.firefly-forage.secrets)";
      };

      authEnvVar = mkOption {
        type = types.str;
        description = "Environment variable name for the auth token";
        example = "ANTHROPIC_API_KEY";
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
          agentName: agent: {
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
              agentName: agent: {
                inherit (agent) secretName authEnvVar;
                packagePath = agent.package.outPath;
              }
            ) template.agents;
            extraPackages = map (p: p.outPath) template.extraPackages;
          };
        }
      ) cfg.templates;
  };
}

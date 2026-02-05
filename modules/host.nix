{ self }:
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
          - restricted: Only allowed hosts (not yet implemented)
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
  };

  config = mkIf cfg.enable {
    # Ensure state directory exists
    systemd.tmpfiles.rules = [
      "d ${cfg.stateDir} 0750 root root -"
      "d ${cfg.stateDir}/templates 0750 root root -"
    ];

    # Install forage-ctl
    environment.systemPackages = [
      self.packages.${pkgs.system}.forage-ctl
    ];

    # Generate template configurations
    environment.etc = mapAttrs (
      name: template:
      {
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
              packagePath = agent.package;
              wrapperPath = self.lib.mkAgentWrapper {
                inherit pkgs;
                name = agentName;
                package = agent.package;
                authEnvVar = agent.authEnvVar;
                secretPath = "/run/secrets/${agent.secretName}";
              };
            }
          ) template.agents;
          extraPackages = map (p: p.outPath) template.extraPackages;
        };
      }
    ) cfg.templates;

    # Base packages needed in all sandboxes
    # These are made available via the shared nix store
  };
}

# nix-darwin module for services.firefly-forage.
# Uses shared option definitions (options.nix) and config generation (config-gen.nix)
# to avoid duplication with the NixOS module (host.nix).
#
# Key differences from the NixOS module:
# - No extra-container / systemd-nspawn (macOS uses Apple Container or Docker)
# - No networking.nat (container runtimes handle their own networking)
# - Uses system.activationScripts instead of systemd.tmpfiles
# - Uses launchd.daemons instead of systemd.services
# - No uid/gid/authorizedKeys/nixpkgs in config.json (NixOS-specific)
{ self }:
{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.services.firefly-forage;

  sharedOptions = import ./options.nix { inherit lib; };
  configGen = import ./config-gen.nix { inherit lib; };

  inherit (lib) mkIf;

  # On macOS, home directories are under /Users
  userHome = "/Users/${cfg.user}";
  resolveTilde' = configGen.resolveTilde userHome;

  configDir = "firefly-forage";
  forage-ctl = self.packages.${pkgs.stdenv.hostPlatform.system}.forage-ctl;

in
{
  options.services.firefly-forage = sharedOptions.mkOptions {
    defaultStateDir = "/var/lib/firefly-forage";
  };

  config = mkIf cfg.enable {
    # Validate configuration
    assertions = [
      {
        assertion = cfg.user != "";
        message = "services.firefly-forage.user must be specified";
      }
    ]
    ++ lib.flatten (
      lib.mapAttrsToList (
        templateName: template:
        lib.mapAttrsToList (
          agentName: agent:
          lib.optional (agent.secretName != null) {
            assertion = cfg.secrets ? ${agent.secretName};
            message = "Template '${templateName}' agent '${agentName}' references secret '${agent.secretName}' which is not defined in services.firefly-forage.secrets";
          }
        ) template.agents
      ) cfg.templates
    );

    # Install forage-ctl
    environment.systemPackages = [ forage-ctl ];

    # Generate host configuration file and template configurations
    environment.etc = {
      "${configDir}/config.json" = {
        text = builtins.toJSON (
          configGen.mkHostConfigJSON {
            inherit cfg resolveTilde';
          }
        );
      };
    }
    // configGen.mkTemplateEtcEntries {
      inherit cfg resolveTilde';
      inherit configDir;
    };

    # Ensure state directory exists via activation script
    system.activationScripts.postActivation.text = ''
      mkdir -p "${cfg.stateDir}" "${cfg.stateDir}/sandboxes" "${cfg.stateDir}/workspaces"
      chown ${cfg.user} "${cfg.stateDir}" "${cfg.stateDir}/sandboxes" "${cfg.stateDir}/workspaces"
      chmod 750 "${cfg.stateDir}" "${cfg.stateDir}/sandboxes" "${cfg.stateDir}/workspaces"
    '';

    # Health monitor launchd service
    launchd.daemons.forage-monitor = mkIf cfg.monitor.enable {
      serviceConfig = {
        Label = "com.firefly.forage-monitor";
        ProgramArguments = [
          "${forage-ctl}/bin/forage-ctl"
          "monitor"
          "--interval"
          cfg.monitor.interval
        ]
        ++ lib.optionals cfg.monitor.autoRestart [ "--auto-restart" ];
        RunAtLoad = true;
        KeepAlive = true;
        UserName = cfg.user;
        StandardOutPath = "/var/log/forage-monitor.log";
        StandardErrorPath = "/var/log/forage-monitor.log";
      };
    };
  };
}

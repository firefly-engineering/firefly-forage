{
  self,
  nixpkgs,
}:
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

  # Resolve ~ to the configured user's home directory
  userHome = config.users.users.${cfg.user}.home or "/home/${cfg.user}";
  resolveTilde' = configGen.resolveTilde userHome;

in
{
  options.services.firefly-forage =
    sharedOptions.mkOptions { defaultStateDir = "/var/lib/firefly-forage"; }
    // {
      externalInterface = lib.mkOption {
        type = lib.types.nullOr lib.types.str;
        default = null;
        description = ''
          External network interface for NAT. If null, NAT configuration
          is skipped (useful when using an existing NAT setup or when
          the interface name differs from the default).
        '';
        example = "eth0";
      };
    };

  config = lib.mkMerge [
    {
      # Allow dynamically-installed systemd units (container service files)
      # to be picked up by systemd from the mutable directory.
      boot.extraSystemdUnitPaths = [ "/etc/systemd-mutable/system" ];

      # Ensure the mutable services directory exists at boot.
      systemd.tmpfiles.rules = [
        "d /etc/systemd-mutable/system 0755 root root -"
      ];
    }
    (mkIf cfg.enable {
      # Validate configuration
      assertions = [
        {
          assertion = cfg.user != "";
          message = "services.firefly-forage.user must be specified";
        }
        {
          assertion = lib.hasPrefix "/run/" "/run/forage-secrets";
          message = "Secrets directory must be under /run (tmpfs) to prevent secrets from persisting on disk";
        }
      ]
      ++ lib.flatten (
        lib.mapAttrsToList (
          templateName: template:
          lib.mapAttrsToList (
            agentName: agent:
            # Only validate secret reference if secretName is specified
            lib.optional (agent.secretName != null) {
              assertion = cfg.secrets ? ${agent.secretName};
              message = "Template '${templateName}' agent '${agentName}' references secret '${agent.secretName}' which is not defined in services.firefly-forage.secrets";
            }
          ) template.agents
        ) cfg.templates
      );

      # Ensure state directory exists
      # The configured user needs access to sandboxes and workspaces directories
      systemd.tmpfiles.rules = [
        "d ${cfg.stateDir} 0750 ${cfg.user} root -"
        "d ${cfg.stateDir}/sandboxes 0750 ${cfg.user} root -"
        "d ${cfg.stateDir}/workspaces 0750 ${cfg.user} root -"
        # Secrets directory is under /run (tmpfs on NixOS) so secrets
        # are never persisted to disk. Do not move this outside /run.
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
    environment.etc = {
      "firefly-forage/config.json" = {
        mode = "0644";
        text = builtins.toJSON (
          configGen.mkHostConfigJSON {
            inherit cfg resolveTilde';
            extraAttrs = {
              uid = config.users.users.${cfg.user}.uid;
              gid = config.users.groups.${config.users.users.${cfg.user}.group}.gid;
              authorizedKeys = cfg.authorizedKeys;
              nixpkgsPath = "${nixpkgs}";
              # Nixpkgs revision for registry pinning
              nixpkgsRev = nixpkgs.rev or "unknown";
            };
          }
        );
      };

      # Health monitor systemd service
      systemd.services.forage-monitor = mkIf cfg.monitor.enable {
        description = "Firefly Forage Health Monitor";
        wantedBy = [ "multi-user.target" ];
        after = [ "network.target" ];
        serviceConfig = {
          ExecStart = "${
            self.packages.${pkgs.stdenv.hostPlatform.system}.forage-ctl
          }/bin/forage-ctl monitor --interval ${cfg.monitor.interval}${
            if cfg.monitor.autoRestart then " --auto-restart" else ""
          }";
          Restart = "on-failure";
          RestartSec = "10s";
          User = cfg.user;
        };
      };
    })
  ];
}

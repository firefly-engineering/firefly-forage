# Generate NixOS container configuration for a sandbox
{ lib }:
{
  pkgs,
  name,
  template,
  workspace,
  sshPort,
  hostUid,
  hostGid,
  authorizedKeys,
  secretsPaths,
  networkSlot,
}:
let
  inherit (lib) mkForce optionalString;

  # Container IP based on network slot (192.168.100.x)
  containerIp = "192.168.100.${toString (networkSlot + 10)}";
  hostIp = "192.168.100.1";

  # Build agent wrappers from template
  agentWrappers = lib.mapAttrsToList (
    agentName: agentConfig:
    pkgs.writeShellApplication {
      name = agentName;
      runtimeInputs = [ (builtins.storePath agentConfig.packagePath) ];
      text = ''
        # Load auth from secret file
        if [ -f "/run/secrets/${agentConfig.secretName}" ]; then
          export ${agentConfig.authEnvVar}="$(cat "/run/secrets/${agentConfig.secretName}")"
        else
          echo "Warning: Secret file not found: /run/secrets/${agentConfig.secretName}" >&2
        fi
        exec ${lib.getExe (builtins.storePath agentConfig.packagePath)} "$@"
      '';
    }
  ) template.agents;

  # Extra packages from template (stored as paths in JSON)
  extraPackages = map builtins.storePath template.extraPackages;
in
{
  # This is the container configuration for extra-container
  containers."forage-${name}" = {
    # Ephemeral = tmpfs root, container state is not persisted
    ephemeral = true;

    # Private network with NAT
    privateNetwork = true;
    hostAddress = hostIp;
    localAddress = containerIp;

    # Forward SSH port from host to container
    forwardPorts = [
      {
        containerPort = 22;
        hostPort = sshPort;
        protocol = "tcp";
      }
    ];

    # Bind mounts
    bindMounts = {
      # Read-only nix store
      "/nix/store" = {
        hostPath = "/nix/store";
        isReadOnly = true;
      };

      # Workspace directory (read-write)
      "/workspace" = {
        hostPath = workspace;
        isReadOnly = false;
      };

      # Secrets directory (read-only)
      "/run/secrets" = {
        hostPath = "/run/forage-secrets/${name}";
        isReadOnly = true;
      };
    };

    # Allow network access based on template
    allowedDevices =
      if template.network == "none" then
        [ ]
      else
        [
          {
            node = "/dev/net/tun";
            modifier = "rw";
          }
        ];

    # Container NixOS configuration
    config =
      { config, pkgs, ... }:
      {
        # System basics
        system.stateVersion = "24.11";

        # No bootloader in container
        boot.isContainer = true;

        # Network configuration
        networking = {
          hostName = "forage-${name}";
          firewall.allowedTCPPorts = [ 22 ];

          # NAT for outbound connections
          useHostResolvConf = mkForce true;
          defaultGateway = hostIp;
        };

        # Create agent user with host UID/GID
        users.users.agent = {
          isNormalUser = true;
          uid = hostUid;
          group = "agent";
          home = "/home/agent";
          shell = pkgs.bash;
          openssh.authorizedKeys.keys = authorizedKeys;
          extraGroups = [ "wheel" ];
        };

        users.groups.agent = {
          gid = hostGid;
        };

        # SSH server
        services.openssh = {
          enable = true;
          settings = {
            PermitRootLogin = "no";
            PasswordAuthentication = false;
          };
        };

        # Tmux session service
        systemd.services.forage-tmux = {
          description = "Forage tmux session for agent";
          after = [ "multi-user.target" ];
          wantedBy = [ "multi-user.target" ];
          serviceConfig = {
            Type = "forking";
            User = "agent";
            Group = "agent";
            WorkingDirectory = "/workspace";
            ExecStart = "${pkgs.tmux}/bin/tmux new-session -d -s forage -c /workspace";
            ExecStop = "${pkgs.tmux}/bin/tmux kill-session -t forage";
            Restart = "on-failure";
            RestartSec = "5s";
          };
        };

        # Packages available in the container
        environment.systemPackages = [
          pkgs.tmux
          pkgs.git
          pkgs.curl
          pkgs.vim
          pkgs.coreutils
          pkgs.bash
        ]
        ++ agentWrappers
        ++ extraPackages;

        # Set PATH for agent
        environment.variables = {
          WORKSPACE = "/workspace";
        };

        # Sudo without password for agent (needed for some operations)
        security.sudo = {
          enable = true;
          wheelNeedsPassword = false;
        };
      };
  };
}

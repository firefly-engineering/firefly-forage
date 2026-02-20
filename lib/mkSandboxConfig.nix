# Generate NixOS container configuration for a sandbox
{ lib, mkAgentWrapper }:
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
  username ? "agent",
  workspacePath ? "/workspace",
}:
let
  inherit (lib) mkForce optionalString;

  homeDir = "/home/${username}";

  # Container IP based on network slot (192.168.100.x)
  containerIp = "192.168.100.${toString (networkSlot + 10)}";
  hostIp = "192.168.100.1";

  # Build agent wrappers from template using shared mkAgentWrapper
  agentWrappers = lib.mapAttrsToList (
    agentName: agentConfig:
    mkAgentWrapper {
      inherit pkgs;
      name = agentName;
      package = builtins.storePath agentConfig.packagePath;
      authEnvVar = agentConfig.authEnvVar;
      secretPath = "/run/secrets/${agentConfig.secretName}";
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
      "${workspacePath}" = {
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
        users.users.${username} = {
          isNormalUser = true;
          uid = hostUid;
          group = username;
          home = homeDir;
          shell = pkgs.bash;
          openssh.authorizedKeys.keys = authorizedKeys;
        };

        users.groups.${username} = {
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
          description = "Forage tmux session for ${username}";
          after = [ "multi-user.target" ];
          wantedBy = [ "multi-user.target" ];
          serviceConfig = {
            Type = "forking";
            User = username;
            Group = username;
            WorkingDirectory = workspacePath;
            ExecStart = "${pkgs.tmux}/bin/tmux new-session -d -s forage -c ${workspacePath}";
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
          WORKSPACE = workspacePath;
        };

        # Sudo is disabled for the agent user by default.
        # If specific privileged operations are needed, add fine-grained
        # sudoers rules instead of blanket passwordless access.
        security.sudo.enable = false;
      };
  };
}

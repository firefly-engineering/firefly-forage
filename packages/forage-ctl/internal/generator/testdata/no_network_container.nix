{ pkgs, ... }:
{
  containers.f1 = {
    autoStart = true;
    ephemeral = true;
    privateNetwork = true;
    hostAddress = "10.100.1.1";
    localAddress = "10.100.1.2";

    bindMounts = {
      "/nix/store" = {
        hostPath = "/nix/store";
        isReadOnly = true;
      };
      "/workspace" = {
        hostPath = "/home/user/project";
        isReadOnly = false;
      };
      "/run/secrets" = {
        hostPath = "/run/secrets/test-sandbox";
        isReadOnly = true;
      };
    };

    config =
      { pkgs, ... }:
      {
        system.stateVersion = "24.05";
        nixpkgs.config.allowUnfree = true;
        networking.hostName = "test-sandbox";
        # No network access
        networking.nameservers = [ ];
        networking.defaultGateway = null;

        # Disable all network interfaces except loopback
        networking.useDHCP = false;

        # Use nftables with default-drop policy (consistent with restricted mode)
        networking.nftables = {
          enable = true;
          ruleset = ''
            table inet filter {
              chain input {
                type filter hook input priority 0; policy accept;
              }

              chain output {
                type filter hook output priority 0; policy drop;

                # Allow loopback only
                oif "lo" accept

                # Allow established/related (for SSH management)
                ct state established,related accept

                # Reject everything else
                reject with icmp type admin-prohibited
              }
            }
          '';
        };

        # Disable iptables (using nftables)
        networking.firewall.enable = false;
        users.users.agent = {
          isNormalUser = true;
          home = "/home/agent";
          shell = "${pkgs.bash}/bin/bash";
          uid = 1000;
          group = "users";
          extraGroups = [ ];
          openssh.authorizedKeys.keys = [
            "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExample user@host"
          ];
        };
        users.groups.users.gid = 100;

        security.sudo.enable = false;

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
          pkgs.claude-code
        ];

        environment.sessionVariables = {
          ANTHROPIC_API_KEY = "$(cat /run/secrets/anthropic 2>/dev/null || echo '')";
        };

        environment.etc."nix/registry.json".text = builtins.toJSON {
          version = 2;
          flakes = [
            {
              exact = true;
              from = {
                id = "nixpkgs";
                type = "indirect";
              };
              to = {
                type = "path";
                path = "${pkgs.path}";
              };
            }
          ];
        };

        environment.etc."forage.json".text = builtins.toJSON {
          sandboxName = "test-sandbox";
          containerName = "f1";
          runtime = "";
        };

        # Ensure ~/.config is owned by agent (bind mounts may create it as root)
        systemd.tmpfiles.rules = [
          "d /home/agent/.config 0755 agent users -"
          "d /home/agent/.config 0755 agent users -"
        ];

        systemd.services.forage-init = {
          description = "Forage Sandbox Initialization";
          wantedBy = [ "multi-user.target" ];
          after = [ "network.target" ];
          serviceConfig = {
            Type = "oneshot";
            User = "agent";
            WorkingDirectory = "/workspace";
            ExecStart = "${pkgs.writeShellScript "forage-init" ''
              tmux new-session -d -s forage -c /workspace -n claude
              tmux send-keys -t forage:claude 'claude' Enter
              true
            ''}";
          };
        };
      };
  };
}

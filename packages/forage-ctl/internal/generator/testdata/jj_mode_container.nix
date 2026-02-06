{ pkgs, ... }: {
  containers.forage-test-sandbox = {
    autoStart = true;
    ephemeral = true;
    privateNetwork = true;
    hostAddress = "10.100.1.1";
    localAddress = "10.100.1.2";

    bindMounts = {
      "/nix/store" = { hostPath = "/nix/store"; isReadOnly = true; };
      "/workspace" = { hostPath = "/var/lib/forage/workspaces/test-sandbox"; isReadOnly = false; };
      "/run/secrets" = { hostPath = "/run/secrets/test-sandbox"; isReadOnly = true; };
      "/home/user/myrepo/.jj" = { hostPath = "/home/user/myrepo/.jj"; isReadOnly = false; };
      "/home/user/myrepo/.git" = { hostPath = "/home/user/myrepo/.git"; isReadOnly = false; };
    };

    config = { pkgs, ... }: {
      system.stateVersion = "24.05";
      # Full network access
      networking.defaultGateway = "10.100.1.1";
      networking.nameservers = [ "1.1.1.1" "8.8.8.8" ];
      networking.firewall.allowedTCPPorts = [ 22 ];
      users.users.agent = {
        isNormalUser = true;
        home = "/home/agent";
        uid = 1000;
        group = "users";
        extraGroups = [ "wheel" ];
        openssh.authorizedKeys.keys = [
          "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExample user@host"
        ];
      };
      users.groups.users.gid = 100;

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
        pkgs.claude-code
      ];

      environment.sessionVariables = {
        ANTHROPIC_API_KEY = "$(cat /run/secrets/anthropic 2>/dev/null || echo '')";
      };

      environment.etc."nix/registry.json".text = builtins.toJSON {
        version = 2;
        flakes = [{
          exact = true;
          from = { id = "nixpkgs"; type = "indirect"; };
          to = { type = "github"; owner = "NixOS"; repo = "nixpkgs"; rev = "abc123def456"; };
        }];
      };

      systemd.services.forage-init = {
        description = "Forage Sandbox Initialization";
        wantedBy = [ "multi-user.target" ];
        after = [ "network.target" ];
        serviceConfig = {
          Type = "oneshot";
          User = "agent";
          WorkingDirectory = "/workspace";
          ExecStart = "${pkgs.bash}/bin/bash -c 'tmux new-session -d -s forage -c /workspace || true'";
        };
      };
    };
  };
}

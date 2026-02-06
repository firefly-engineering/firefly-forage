{ pkgs, ... }: {
  containers.forage-test-sandbox = {
    autoStart = true;
    ephemeral = true;
    privateNetwork = true;
    hostAddress = "10.100.1.1";
    localAddress = "10.100.1.2";

    bindMounts = {
      "/nix/store" = { hostPath = "/nix/store"; isReadOnly = true; };
      "/workspace" = { hostPath = "/home/user/project"; isReadOnly = false; };
      "/run/secrets" = { hostPath = "/run/secrets/test-sandbox"; isReadOnly = true; };
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

      # Ensure ~/.config is owned by agent (bind mounts may create it as root)
      systemd.tmpfiles.rules = [
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
          ExecStart = "${pkgs.bash}/bin/bash -c 'tmux new-session -d -s forage -c /workspace || true'";
        };
      };
    };
  };
}

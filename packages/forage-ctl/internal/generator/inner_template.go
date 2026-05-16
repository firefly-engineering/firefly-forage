package generator

// innerTemplateText is the NixOS module for the cached inner system.
// It contains everything template-level: packages, services, users, etc.
// Per-sandbox data (hostname, network slot, env vars, forage.json) is
// injected at runtime via bind mounts and the forage-network service.
const innerTemplateText = `{ pkgs, ... }:
{
  boot.isContainer = true;
  system.stateVersion = "{{.StateVersion}}";
  nixpkgs.config.allowUnfree = true;
  networking.hostName = "{{.TemplateName}}";
  {{.NetworkConfig}}
  users.users.{{.Username}} = {
    isNormalUser = true;
    home = "{{.HomeDir}}";
    shell = "${pkgs.bash}/bin/bash";
    uid = {{.UID}};
    group = "users";
    extraGroups = [ ];
    openssh.authorizedKeys.keys = [
{{- range .AuthorizedKeys}}
      {{. | printf "%q"}}
{{- end}}
    ];
  };
  users.groups.users.gid = {{.GID}};

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
{{- range .MuxPackages}}
    {{.}}
{{- end}}
    neovim
    ripgrep
    fd
    jq
    iproute2
{{- range .AgentPackages}}
    {{.}}
{{- end}}
{{- if .ClaudePackagePath}}
    (pkgs.writeShellScriptBin "claude" ''
      exec ${pkgs.{{.ClaudePackagePath}}}/bin/claude \
        --append-system-prompt "$(cat {{.SystemPromptFile}})" "$@"
    '')
{{- end}}
  ];

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
          path = "{{.NixpkgsPath}}";
        };
      }
    ];
  };

  # Ensure directories exist for bind mounts
  systemd.tmpfiles.rules = [
{{- range .ExtraTmpfilesRules}}
    "{{.}}"
{{- end}}
  ];

  # Runtime network configuration: reads gateway from bind-mounted config
  systemd.services.forage-network = {
    description = "Forage Network Configuration";
    wantedBy = [ "network.target" ];
    before = [ "network-online.target" ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      ExecStart = "${pkgs.writeShellScript "forage-network" ''
        set -euo pipefail
        CONFIG=/run/forage/config.json
        if [ -f "$CONFIG" ]; then
          GATEWAY=$(${pkgs.jq}/bin/jq -r '.gateway' "$CONFIG")
          if [ -n "$GATEWAY" ] && [ "$GATEWAY" != "null" ]; then
            ${pkgs.iproute2}/bin/ip route replace default via "$GATEWAY"
          fi
        fi
      ''}";
    };
  };

  # Runtime hostname configuration: reads sandbox name from bind-mounted config
  systemd.services.forage-hostname = {
    description = "Forage Hostname Configuration";
    wantedBy = [ "multi-user.target" ];
    before = [ "network.target" ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      ExecStart = "${pkgs.writeShellScript "forage-hostname" ''
        set -euo pipefail
        CONFIG=/run/forage/config.json
        if [ -f "$CONFIG" ]; then
          HOSTNAME=$(${pkgs.jq}/bin/jq -r '.sandboxName' "$CONFIG")
          if [ -n "$HOSTNAME" ] && [ "$HOSTNAME" != "null" ]; then
            ${pkgs.hostname}/bin/hostname "$HOSTNAME"
          fi
        fi
      ''}";
    };
  };

  systemd.services.forage-init = {
    description = "Forage Sandbox Initialization";
    wantedBy = [ "multi-user.target" ];
    after = [ "network.target" ];
    serviceConfig = {
      Type = "oneshot";
      User = "{{.Username}}";
      WorkingDirectory = "{{.WorkspaceDir}}";
      ExecStart = "${pkgs.writeShellScript "forage-init" ''
{{.MuxInitScript}}
      ''}";
    };
  };
{{- if .ResourceLimits}}
  systemd.services.forage-resources = {
    description = "Forage Resource Limits (no-op anchor for resource control)";
    wantedBy = [ "multi-user.target" ];
    serviceConfig = {
      Type = "oneshot";
      ExecStart = "${pkgs.coreutils}/bin/true";
      RemainAfterExit = true;
{{- if .ResourceLimits.CPUQuota}}
      CPUQuota = "{{.ResourceLimits.CPUQuota}}";
{{- end}}
{{- if .ResourceLimits.MemoryMax}}
      MemoryMax = "{{.ResourceLimits.MemoryMax}}";
{{- end}}
{{- if .ResourceLimits.TasksMax}}
      TasksMax = {{.ResourceLimits.TasksMax}};
{{- end}}
    };
  };
{{- end}}
{{- if or .GitUser .GitEmail .SSHKeyName}}
  systemd.services.forage-agent-identity = {
    description = "Forage Agent Identity Setup";
    wantedBy = [ "multi-user.target" ];
    after = [ "network.target" ];
    serviceConfig = {
      Type = "oneshot";
      User = "{{.Username}}";
      ExecStart = "${pkgs.writeShellScript "forage-agent-identity" ''
        set -euo pipefail
        ${pkgs.coreutils}/bin/mkdir -p {{.HomeDir}}/.ssh {{.HomeDir}}/.config/jj
{{- if .GitUser}}
        ${pkgs.git}/bin/git config --global user.name {{.GitUser | shellQuote | nixEscapeIndented}}
        ${pkgs.jujutsu}/bin/jj config set --user user.name {{.GitUser | shellQuote | nixEscapeIndented}} || true
{{- end}}
{{- if .GitEmail}}
        ${pkgs.git}/bin/git config --global user.email {{.GitEmail | shellQuote | nixEscapeIndented}}
        ${pkgs.jujutsu}/bin/jj config set --user user.email {{.GitEmail | shellQuote | nixEscapeIndented}} || true
{{- end}}
{{- if .SSHKeyName}}
        ${pkgs.coreutils}/bin/cat > {{.HomeDir}}/.ssh/config <<SSH_EOF
        Host *
          IdentityFile {{.HomeDir}}/.ssh/{{.SSHKeyName}}
          StrictHostKeyChecking accept-new
        SSH_EOF
{{- end}}
      ''}";
    };
  };
{{- end}}
}
`

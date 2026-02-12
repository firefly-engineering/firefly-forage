package generator

import (
	"strings"
	"text/template"
)

// TemplateData holds all data needed to render the container Nix configuration.
type TemplateData struct {
	ContainerName      string
	Hostname           string   // Hostname inside the container (the sandbox name)
	NetworkSlot        int
	StateVersion       string
	BindMounts         []BindMount
	AuthorizedKeys     []string
	NetworkConfig      string // Pre-rendered from network package
	AgentPackages      []string
	EnvVars            []EnvVar
	MuxPackages        []string // Multiplexer packages to install (e.g. ["tmux"] or ["wezterm"])
	MuxInitScript      string   // Pre-rendered init script from multiplexer backend
	UID                int      // Host user's UID for the container agent user
	GID                int      // Host user's GID for the container agent user
	ExtraTmpfilesRules []string // Additional systemd tmpfiles rules
	GitUser            string   // Git user.name for agent identity
	GitEmail           string   // Git user.email for agent identity
	SSHKeyName         string   // Basename of SSH key file (empty if no SSH key)
	SystemPromptFile   string   // Container path of system prompt file (empty if not set)
	ClaudePackagePath  string   // Nix store path of unwrapped claude package (empty if not wrapping)
	SandboxName        string   // Sandbox name (for in-container metadata)
	Runtime            string   // Runtime backend name (for in-container metadata)
}

// BindMount represents a bind mount entry in the Nix config.
type BindMount struct {
	Path     string
	HostPath string
	ReadOnly bool
}

// EnvVar represents an environment variable in the Nix config.
type EnvVar struct {
	Name  string
	Value string
}

// nixBool returns "true" or "false" for use in Nix configuration.
func nixBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// nixEscape escapes a string for safe inclusion inside a Nix "..." string literal.
// It handles backslashes, double quotes, and ${} interpolation sequences.
func nixEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "${", "\\${")
	return s
}

// shellQuote wraps a string in double quotes with proper escaping for bash.
func shellQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "`", "\\`")
	s = strings.ReplaceAll(s, "$", `\$`)
	return `"` + s + `"`
}

// containerTemplate is the main Go template for generating NixOS container configurations.
const containerTemplateText = `{ pkgs, ... }:
{
  containers.{{.ContainerName}} = {
    autoStart = true;
    ephemeral = true;
    privateNetwork = true;
    hostAddress = "10.100.{{.NetworkSlot}}.1";
    localAddress = "10.100.{{.NetworkSlot}}.2";

    bindMounts = {
{{- range .BindMounts}}
      "{{.Path}}" = {
        hostPath = "{{.HostPath}}";
        isReadOnly = {{.ReadOnly | nixBool}};
      };
{{- end}}
    };

    config =
      { pkgs, ... }:
      {
        system.stateVersion = "{{.StateVersion}}";
        nixpkgs.config.allowUnfree = true;
        networking.hostName = "{{.Hostname}}";
        {{.NetworkConfig}}
        users.users.agent = {
          isNormalUser = true;
          home = "/home/agent";
          shell = "${pkgs.bash}/bin/bash";
          uid = {{.UID}};
          group = "users";
          extraGroups = [ "wheel" ];
          openssh.authorizedKeys.keys = [
{{- range .AuthorizedKeys}}
            {{. | printf "%q"}}
{{- end}}
          ];
        };
        users.groups.users.gid = {{.GID}};

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
{{- range .MuxPackages}}
          {{.}}
{{- end}}
          neovim
          ripgrep
          fd
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
{{if .EnvVars}}
        environment.sessionVariables = {
{{- range .EnvVars}}
          {{.Name}} = {{.Value}};
{{- end}}
        };
{{end}}
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
          sandboxName = "{{.SandboxName}}";
          containerName = "{{.ContainerName}}";
          runtime = "{{.Runtime}}";
        };

        # Ensure ~/.config is owned by agent (bind mounts may create it as root)
        systemd.tmpfiles.rules = [
          "d /home/agent/.config 0755 agent users -"
{{- range .ExtraTmpfilesRules}}
          "{{.}}"
{{- end}}
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
{{.MuxInitScript}}
            ''}";
          };
        };
{{- if or .GitUser .GitEmail .SSHKeyName}}
        systemd.services.forage-agent-identity = {
          description = "Forage Agent Identity Setup";
          wantedBy = [ "multi-user.target" ];
          after = [ "network.target" ];
          serviceConfig = {
            Type = "oneshot";
            User = "agent";
            ExecStart = "${pkgs.bash}/bin/bash -c '${pkgs.coreutils}/bin/mkdir -p /home/agent/.ssh /home/agent/.config/jj && " +
{{- if .GitUser}}
              "${pkgs.git}/bin/git config --global user.name {{.GitUser | shellQuote | nixEscape}} && " +
              "${pkgs.jujutsu}/bin/jj config set --user user.name {{.GitUser | shellQuote | nixEscape}} && " +
{{- end}}
{{- if .GitEmail}}
              "${pkgs.git}/bin/git config --global user.email {{.GitEmail | shellQuote | nixEscape}} && " +
              "${pkgs.jujutsu}/bin/jj config set --user user.email {{.GitEmail | shellQuote | nixEscape}} && " +
{{- end}}
{{- if .SSHKeyName}}
              "${pkgs.coreutils}/bin/cat > /home/agent/.ssh/config <<SSH_EOF\nHost *\n  IdentityFile /home/agent/.ssh/{{.SSHKeyName}}\n  StrictHostKeyChecking accept-new\nSSH_EOF\n && " +
{{- end}}
              "true'";
          };
        };
{{- end}}
      };
  };
}
`

// containerTemplate is the parsed template, initialized at package load time.
var containerTemplate *template.Template

func init() {
	funcs := template.FuncMap{
		"nixBool":    nixBool,
		"nixEscape":  nixEscape,
		"shellQuote": shellQuote,
	}
	containerTemplate = template.Must(template.New("container").Funcs(funcs).Parse(containerTemplateText))
}

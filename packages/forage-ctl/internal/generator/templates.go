package generator

import (
	"strings"
	"text/template"

	shellquote "github.com/kballard/go-shellquote"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
)

// TemplateData holds all data needed to render the container Nix configuration.
type TemplateData struct {
	ContainerName      string
	Hostname           string // Hostname inside the container (the sandbox name)
	NetworkSlot        int
	StateVersion       string
	Username           string // Container username (e.g. "agent")
	HomeDir            string // Container home directory (e.g. "/home/agent")
	WorkspaceDir       string // Container workspace path (e.g. "/workspace")
	BindMounts         []BindMount
	AuthorizedKeys     []string
	NetworkConfig      string // Pre-rendered from network package
	AgentPackages      []string
	EnvVars            []EnvVar
	MuxPackages        []string               // Multiplexer packages to install (e.g. ["tmux"] or ["wezterm"])
	MuxInitScript      string                 // Pre-rendered init script from multiplexer backend
	UID                int                    // Host user's UID for the container agent user
	GID                int                    // Host user's GID for the container agent user
	ExtraTmpfilesRules []string               // Additional systemd tmpfiles rules
	GitUser            string                 // Git user.name for agent identity
	GitEmail           string                 // Git user.email for agent identity
	SSHKeyName         string                 // Basename of SSH key file (empty if no SSH key)
	SystemPromptFile   string                 // Container path of system prompt file (empty if not set)
	ClaudePackagePath  string                 // Nix store path of unwrapped claude package (empty if not wrapping)
	SandboxName        string                 // Sandbox name (for in-container metadata)
	Runtime            string                 // Runtime backend name (for in-container metadata)
	ResourceLimits     *config.ResourceLimits // Optional resource limits for systemd
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
            ExecStart = "${pkgs.bash}/bin/bash -c '${pkgs.coreutils}/bin/mkdir -p {{.HomeDir}}/.ssh {{.HomeDir}}/.config/jj && " +
{{- if .GitUser}}
              "${pkgs.git}/bin/git config --global user.name {{.GitUser | shellQuote | nixEscape}} && " +
              "${pkgs.jujutsu}/bin/jj config set --user user.name {{.GitUser | shellQuote | nixEscape}} && " +
{{- end}}
{{- if .GitEmail}}
              "${pkgs.git}/bin/git config --global user.email {{.GitEmail | shellQuote | nixEscape}} && " +
              "${pkgs.jujutsu}/bin/jj config set --user user.email {{.GitEmail | shellQuote | nixEscape}} && " +
{{- end}}
{{- if .SSHKeyName}}
              "${pkgs.coreutils}/bin/cat > {{.HomeDir}}/.ssh/config <<SSH_EOF\nHost *\n  IdentityFile {{.HomeDir}}/.ssh/{{.SSHKeyName}}\n  StrictHostKeyChecking accept-new\nSSH_EOF\n && " +
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
		"nixBool":   nixBool,
		"nixEscape": nixEscape,
		"shellQuote": func(s string) string {
			return shellquote.Join(s)
		},
	}
	containerTemplate = template.Must(template.New("container").Funcs(funcs).Parse(containerTemplateText))
}

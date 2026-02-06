package generator

import (
	"text/template"
)

// TemplateData holds all data needed to render the container Nix configuration.
type TemplateData struct {
	ContainerName  string
	NetworkSlot    int
	StateVersion   string
	BindMounts     []BindMount
	AuthorizedKeys []string
	NetworkConfig  string // Pre-rendered from network package
	AgentPackages  []string
	EnvVars        []EnvVar
	RegistryConfig RegistryConfig
	TmuxSession    string
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

// RegistryConfig holds nixpkgs registry configuration.
type RegistryConfig struct {
	Enabled    bool
	NixpkgsRev string
}

// nixBool returns "true" or "false" for use in Nix configuration.
func nixBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// containerTemplate is the main Go template for generating NixOS container configurations.
const containerTemplateText = `{ pkgs, ... }: {
  containers.{{.ContainerName}} = {
    autoStart = true;
    ephemeral = true;
    privateNetwork = true;
    hostAddress = "10.100.{{.NetworkSlot}}.1";
    localAddress = "10.100.{{.NetworkSlot}}.2";

    bindMounts = {
{{- range .BindMounts}}
      "{{.Path}}" = { hostPath = "{{.HostPath}}"; isReadOnly = {{.ReadOnly | nixBool}}; };
{{- end}}
    };

    config = { pkgs, ... }: {
      system.stateVersion = "{{.StateVersion}}";
      {{.NetworkConfig}}
      users.users.agent = {
        isNormalUser = true;
        home = "/home/agent";
        extraGroups = [ "wheel" ];
        openssh.authorizedKeys.keys = [
{{- range .AuthorizedKeys}}
          {{. | printf "%q"}}
{{- end}}
        ];
      };

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
{{- range .AgentPackages}}
        {{.}}
{{- end}}
      ];
{{if .EnvVars}}
      environment.sessionVariables = {
{{- range .EnvVars}}
        {{.Name}} = {{.Value}};
{{- end}}
      };
{{end}}
{{- if .RegistryConfig.Enabled}}
      environment.etc."nix/registry.json".text = builtins.toJSON {
        version = 2;
        flakes = [{
          exact = true;
          from = { id = "nixpkgs"; type = "indirect"; };
          to = { type = "github"; owner = "NixOS"; repo = "nixpkgs"; rev = "{{.RegistryConfig.NixpkgsRev}}"; };
        }];
      };
{{end}}
      systemd.services.forage-init = {
        description = "Forage Sandbox Initialization";
        wantedBy = [ "multi-user.target" ];
        after = [ "network.target" ];
        serviceConfig = {
          Type = "oneshot";
          User = "agent";
          WorkingDirectory = "/workspace";
          ExecStart = "${pkgs.bash}/bin/bash -c 'tmux new-session -d -s {{.TmuxSession}} -c /workspace || true'";
        };
      };
    };
  };
}
`

// containerTemplate is the parsed template, initialized at package load time.
var containerTemplate *template.Template

func init() {
	funcs := template.FuncMap{
		"nixBool": nixBool,
	}
	containerTemplate = template.Must(template.New("container").Funcs(funcs).Parse(containerTemplateText))
}

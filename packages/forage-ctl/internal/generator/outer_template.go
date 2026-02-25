package generator

// OuterTemplateData holds data for the outer container definition.
// This is the per-sandbox config that references the cached inner system.
type OuterTemplateData struct {
	ContainerName string      // Short container name (e.g., "f42")
	NetworkSlot   int         // Network slot for IP addressing
	SystemPath    string      // Nix store path of the cached inner system
	BindMounts    []BindMount // All bind mounts (workspace, secrets, runtime files, etc.)
}

// outerTemplateText is the minimal container definition that wraps a pre-built
// inner system. It only specifies the container shell (networking, mounts) and
// references the inner system via its store path. This evaluates in ~0.5s
// instead of ~12s because no NixOS module evaluation happens.
//
// The path uses lib.mkForce because extra-container's extraModule always injects
// a `config` module into every container, and nixos-containers.nix derives a
// `path` from that config. Without mkForce, both sources set `path` and Nix
// reports a conflicting definition error.
const outerTemplateText = `{ lib, ... }:
{
  containers.{{.ContainerName}} = {
    autoStart = true;
    ephemeral = true;
    privateNetwork = true;
    hostAddress = "10.100.{{.NetworkSlot}}.1";
    localAddress = "10.100.{{.NetworkSlot}}.2";
    path = lib.mkForce {{.SystemPath}};

    bindMounts = {
{{- range .BindMounts}}
      "{{.Path}}" = {
        hostPath = "{{.HostPath}}";
        isReadOnly = {{.ReadOnly | nixBool}};
      };
{{- end}}
    };
  };
}
`

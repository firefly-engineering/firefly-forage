{ lib }:
let
  # Generate an agent wrapper that injects auth from a secret file
  mkAgentWrapper =
    {
      pkgs,
      name,
      package,
      authEnvVar,
      secretPath,
    }:
    pkgs.writeShellApplication {
      inherit name;
      runtimeInputs = [ package ];
      text = ''
        # Load auth from secret file into environment variable.
        # Note: exported vars are visible via env/proc. This is
        # obfuscation for UX convenience, not a security boundary.
        if [ -f "${secretPath}" ]; then
          export ${authEnvVar}="$(cat "${secretPath}")"
        else
          echo "Warning: Secret file not found: ${secretPath}" >&2
        fi
        exec ${lib.getExe package} "$@"
      '';
    };
in
{
  # Generate NixOS container configuration for a sandbox
  mkSandboxConfig = import ./mkSandboxConfig.nix { inherit lib mkAgentWrapper; };

  # Generate skill injection content
  mkSkillsContent = import ./skills.nix { inherit lib; };

  inherit mkAgentWrapper;

  # Network mode type
  networkModes = [
    "full"
    "restricted"
    "none"
  ];
}

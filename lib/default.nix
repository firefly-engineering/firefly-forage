{ lib }:
{
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
        # Load auth from secret file (not visible in environment)
        if [ -f "${secretPath}" ]; then
          export ${authEnvVar}="$(cat "${secretPath}")"
        else
          echo "Warning: Secret file not found: ${secretPath}" >&2
        fi
        exec ${lib.getExe package} "$@"
      '';
    };

  # Network mode type
  networkModes = [
    "full"
    "restricted"
    "none"
  ];
}

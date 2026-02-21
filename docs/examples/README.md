# Configuration Examples

This directory contains example configuration files for Firefly Forage.

## Files

### config.json

The host configuration file. Place at `/etc/firefly-forage/config.json`.

| Field | Type | Description |
|-------|------|-------------|
| `user` | string | The system user that owns sandboxes |
| `portRange.from` | int | Start of SSH port range for sandboxes |
| `portRange.to` | int | End of SSH port range for sandboxes |
| `authorizedKeys` | []string | SSH public keys authorized for sandbox access |
| `secrets` | map[string]string | Named secrets (e.g., API keys) |
| `stateDir` | string | Directory for sandbox state (default: `/var/lib/firefly-forage`) |
| `extraContainerPath` | string | Path to extra-container binary |
| `nixpkgsRev` | string | Nixpkgs revision to use for containers |
| `proxyUrl` | string | Optional URL for API proxy service |

### template-claude.json

Example template for Claude Code. Place templates at `/etc/firefly-forage/templates/<name>.json`.

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Template name (auto-set from filename if empty) |
| `description` | string | Human-readable description |
| `network` | string | Network mode: `full`, `restricted`, or `none` |
| `allowedHosts` | []string | Hosts allowed in `restricted` network mode |
| `agents` | map[string]AgentConfig | Agent configurations |
| `extraPackages` | []string | Additional Nix packages to include |
| `useProxy` | bool | Whether to use the API proxy |

### AgentConfig

| Field | Type | Description |
|-------|------|-------------|
| `packagePath` | string | Nix package path (e.g., `pkgs.claude-code`) |
| `secretName` | string | Name of secret from host config to inject |
| `authEnvVar` | string | Environment variable name for the secret |

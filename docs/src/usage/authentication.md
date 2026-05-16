# Authentication

Sandboxes run in isolated containers that cannot access the host's credential stores (e.g., macOS Keychain, Linux secret-service). This page explains how forage-ctl bridges that gap for each authentication method.

## Overview

There are three ways to authenticate agents in sandboxes, in order of preference:

| Method | Scope | Token lifetime | Best for |
|--------|-------|---------------|----------|
| [Long-lived token](#long-lived-token-recommended) | Claude-specific | 1 year | Production use, long-running sandboxes |
| [Keychain passthrough](#keychain-passthrough) | Claude-specific | ~8 hours | Quick experiments, no setup needed |
| [Secret files](#secret-files) | Any agent | Indefinite | API key auth (Anthropic API, OpenAI, etc.) |

## Long-lived token (recommended)

For Claude Code with a Max/Pro subscription (OAuth authentication), generate a long-lived token that forage-ctl stores and injects into every sandbox.

### Setup

1. Generate a token (opens browser for OAuth):

   ```bash
   claude setup-token
   ```

2. Copy the displayed token and store it:

   ```bash
   forage-ctl claude token store <token>
   ```

3. Verify:

   ```bash
   forage-ctl claude token status
   ```

That's it. All future sandboxes with a Claude agent will automatically pick up this token.

### How it works

```
forage-ctl claude token store <token>
    │
    ▼
<stateDir>/tokens/claude-oauth.json     ← token + creation time + expiry
    │
    │  (on forage-ctl up)
    ▼
CLAUDE_CODE_OAUTH_TOKEN env var         ← injected into container
    │
    ▼
Claude Code reads env var               ← authenticates without keychain
```

The token file is stored at `<stateDir>/tokens/claude-oauth.json` (typically `/var/lib/firefly-forage/tokens/claude-oauth.json`) with mode `0600`. It contains:

```json
{
  "token": "sk-ant-...",
  "createdAt": "2026-03-19T21:00:00Z",
  "expiresAt": "2027-03-19T21:00:00Z"
}
```

### Token lifecycle

- **Valid**: token is used silently, no output.
- **Expiring** (< 30 days remaining): token is used but forage-ctl prints a warning during `forage-ctl up` suggesting renewal.
- **Expired**: forage-ctl falls back to keychain passthrough and warns. Renew with `claude setup-token` + `forage-ctl claude token store`.
- **Missing**: same behavior as expired — keychain fallback with instructions.

### Management commands

```bash
forage-ctl claude token store <token>   # Store a new token
forage-ctl claude token status          # Show token state and expiry
forage-ctl claude token remove          # Delete stored token
```

### Nix configuration

When using a long-lived token, the template only needs `hostConfigDir` — no secrets or API key env vars:

```nix
services.firefly-forage.templates.claude = {
  description = "Claude Code sandbox";
  network = "full";
  agents.claude = {
    package = pkgs.claude-code;
    hostConfigDir = "~/.claude";
  };
};
```

`hostConfigDir` mounts the host `~/.claude` directory into the container. This provides Claude Code with its configuration, project history, and settings. The OAuth token is injected separately via `CLAUDE_CODE_OAUTH_TOKEN`, not through the mounted directory.

## Keychain passthrough

When no long-lived token is stored, forage-ctl automatically reads the OAuth access token from the host's credential store and injects it. This requires no setup but has limitations.

### How it works

On macOS, Claude Code stores OAuth credentials in the login keychain under the service name `Claude Code-credentials`. At sandbox creation time, forage-ctl:

1. Reads the keychain entry via `security find-generic-password`
2. Parses the JSON to extract the access token
3. Checks the token hasn't expired
4. Injects it as `CLAUDE_CODE_OAUTH_TOKEN`

### Limitations

- **macOS only** — Linux keychain support is not yet implemented.
- **Short-lived** — access tokens expire in ~8 hours. A token extracted at sandbox creation may expire during a long session.
- **No refresh** — once injected, the token cannot be refreshed inside the container. When it expires, Claude Code will report authentication errors.
- **Requires active session** — the host must have a valid Claude Code login (i.e., you've used `claude` on the host recently).

The keychain passthrough is a convenience for quick experiments. For anything beyond that, use a long-lived token.

### Verbose output

With `-v`, forage-ctl logs which token source was used:

```
level=DEBUG msg="using stored long-lived Claude OAuth token"
```

or:

```
level=DEBUG msg="read OAuth token from keychain" expiresIn=7h25m0s
level=DEBUG msg="using short-lived OAuth token from host keychain ..."
```

## Secret files

For agents that authenticate via API keys (not OAuth), use the secrets mechanism. This works for any agent type — Claude with an API key, OpenAI, or custom agents.

### Nix configuration

Define secrets as a mapping from names to file paths:

```nix
services.firefly-forage = {
  secrets = {
    anthropic = config.sops.secrets.anthropic-api-key.path;
    openai = "/run/secrets/openai-api-key";
  };

  templates.claude = {
    agents.claude = {
      package = pkgs.claude-code;
      secretName = "anthropic";
      authEnvVar = "ANTHROPIC_API_KEY";
    };
  };
};
```

### How it works

1. The Nix module validates that each agent's `secretName` exists in the top-level `secrets` map.
2. At sandbox creation, forage-ctl copies the secret file into a per-sandbox directory under `<secretsDir>/<sandbox-name>/`.
3. The secret directory is bind-mounted read-only into the container at `/run/secrets/`.
4. The agent's wrapper script reads the file and exports it as the specified `authEnvVar`.

### Secret management integration

Secrets should come from a proper secret manager, not plain files:

```nix
# sops-nix (recommended)
secrets.anthropic = config.sops.secrets.anthropic-api-key.path;

# agenix
secrets.anthropic = config.age.secrets.anthropic-api-key.path;
```

## Token resolution priority

When a Claude agent is configured with `hostConfigDir` (OAuth flow) and no `secretName`, forage-ctl resolves the token in this order:

1. **Token store** — `<stateDir>/tokens/claude-oauth.json`. If valid, use it.
2. **Token store (expiring)** — if the stored token has < 30 days remaining, use it but warn.
3. **Token store (expired)** — skip, warn, fall through.
4. **Host keychain** — extract the short-lived access token from the macOS Keychain.
5. **No token** — warn with setup instructions. The sandbox is created but Claude Code will report "Not logged in".

When `secretName` is set, the secret file path is used directly and none of the above applies.

## Troubleshooting

### "Not logged in" inside sandbox

Check which token source forage-ctl is using:

```bash
forage-ctl up mysandbox --template=claude --repo=. -v 2>&1 | grep -i "oauth\|token\|keychain"
```

Common causes:

- **No long-lived token and no keychain entry**: log in on the host with `claude auth login`, then either use the keychain passthrough or generate a long-lived token.
- **Expired keychain token**: run `claude` on the host to trigger a refresh, then recreate the sandbox.
- **Expired long-lived token**: `forage-ctl claude token status` will confirm. Regenerate with `claude setup-token`.

### Verify authentication inside a running sandbox

```bash
forage-ctl exec mysandbox -- claude auth status
```

Expected output for a working setup:

```json
{
  "loggedIn": true,
  "authMethod": "oauth_token",
  "apiProvider": "firstParty"
}
```

### Token not reaching the container

Verify the env var is set:

```bash
forage-ctl exec mysandbox -- printenv CLAUDE_CODE_OAUTH_TOKEN
```

If empty, check `forage-ctl up -v` output for token resolution messages.

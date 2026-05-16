# Agent Wrappers

Agent wrappers are generated scripts that inject authentication and execute the actual agent binary. They provide a layer of auth obfuscation.

## How Wrappers Work

```
┌─────────────────────────────────────────────────────────┐
│ Container                                               │
│                                                         │
│  $ claude chat "hello"                                  │
│       │                                                 │
│       ▼                                                 │
│  /usr/bin/claude (wrapper)                              │
│       │                                                 │
│       ├─► read /run/secrets/anthropic-api-key           │
│       ├─► export ANTHROPIC_API_KEY="sk-..."             │
│       └─► exec /nix/store/.../bin/claude "$@"           │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

The wrapper:
1. Reads the API key from a file (not environment)
2. Sets the environment variable only for the child process
3. Executes the real agent binary with all arguments

## Generated Wrapper Code

For each agent defined in a template:

```nix
agents.claude = {
  package = pkgs.claude-code;
  secretName = "anthropic";
  authEnvVar = "ANTHROPIC_API_KEY";
};
```

Forage generates:

```bash
#!/usr/bin/env bash
if [ -f "/run/secrets/anthropic" ]; then
  export ANTHROPIC_API_KEY="$(cat /run/secrets/anthropic)"
fi
exec /nix/store/abc123-claude-code/bin/claude "$@"
```

This wrapper is added to the container's `environment.systemPackages`.

## Security Properties

### What Wrappers Protect Against

- **Environment snooping**: The API key isn't in the global environment
- **Process listing**: `ps aux` won't show the key
- **Casual discovery**: Agent can't just `echo $ANTHROPIC_API_KEY`

### What Wrappers Don't Protect Against

- **Determined agents**: An agent could read `/run/secrets/` directly
- **Memory inspection**: The key is in the process memory
- **Network interception**: Keys are sent to APIs

Wrappers provide *obfuscation*, not *security*. They make it harder for an agent to accidentally discover credentials, but a malicious agent could still find them.

## Secret Mounting

Secrets flow from host to container:

```
Host:
  /run/secrets/anthropic-api-key (from sops/agenix)
       │
       ▼
  /run/forage-secrets/myproject/anthropic (copied at sandbox creation)
       │
       ▼
Container:
  /run/secrets/anthropic (bind mounted, read-only)
```

The secrets directory is:
- Created fresh for each sandbox
- Bind-mounted read-only into the container
- Cleaned up when the sandbox is destroyed

## Multiple Agents

Templates can define multiple agents:

```nix
agents = {
  claude = {
    package = pkgs.claude-code;
    secretName = "anthropic";
    authEnvVar = "ANTHROPIC_API_KEY";
  };

  aider = {
    package = pkgs.aider-chat;
    secretName = "openai";
    authEnvVar = "OPENAI_API_KEY";
  };
};
```

Each gets its own wrapper, and both are available in the container:

```bash
# Inside container
claude --help
aider --help
```

## Wrapper vs Direct Execution

| Aspect | Wrapper | Direct |
|--------|---------|--------|
| Auth source | File read at runtime | Environment variable |
| Auth visibility | Hidden from environment | Visible in `env` |
| Setup required | Automatic | Manual export |
| Works outside sandbox | No | Yes (with manual setup) |

## Future: API Bridge

A more secure approach (planned for Phase 5) would remove secrets from containers entirely:

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│ Sandbox         │     │ API Bridge       │     │ External APIs   │
│                 │     │ (on host)        │     │                 │
│ claude-wrapper ─┼────►│ - Auth injection │────►│ api.anthropic.  │
│  (no secrets)   │     │ - Rate limiting  │     │                 │
│                 │     │ - Audit logs     │     │                 │
└─────────────────┘     └──────────────────┘     └─────────────────┘
```

With an API bridge:
- Secrets never enter the container
- All API calls are logged
- Rate limiting is enforced
- Requests can be filtered/modified

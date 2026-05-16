# ADR 002: SSH-Based Sandbox Access

## Status

Accepted

## Context

Users need to access sandboxes to interact with AI agents, run commands, and debug issues. Several access methods were considered.

Requirements:
1. Works from remote machines (not just localhost)
2. Supports terminal-based AI agents (Claude Code, etc.)
3. Compatible with existing developer workflows
4. Secure by default
5. Works consistently across container runtimes

## Decision

Use SSH as the primary access method for all sandboxes:

1. Each sandbox runs an OpenSSH server on port 22 (container-internal)
2. Container port 22 is forwarded to a unique host port (from configured range)
3. Authentication uses SSH public keys (no passwords)
4. `forage-ctl ssh <name>` connects to the sandbox's tmux session

The `ssh` package provides a builder pattern for constructing SSH commands:

```go
opts := ssh.DefaultOptions(port).WithTTY()
args := opts.BuildArgs("tmux", "attach", "-t", "forage")
```

## Consequences

### Positive

- **Universal**: SSH works from any machine, any network
- **Familiar**: Developers already know SSH
- **Secure**: Key-based auth, encrypted transport
- **Flexible**: Works with VS Code Remote, Mosh, etc.
- **Runtime-agnostic**: Same access method for nspawn, Docker, Apple Container
- **Scriptable**: Easy to automate with standard SSH tools

### Negative

- **Overhead**: SSH adds latency compared to `machinectl shell`
- **Port management**: Need to track and allocate ports
- **Key management**: Requires SSH key configuration
- **Extra dependency**: Requires sshd in container image

## Alternatives Considered

### 1. machinectl shell (nspawn only)

Use `machinectl shell` for direct container access.

**Rejected because**: Only works for nspawn, not Docker or Apple Container. Not accessible from remote machines.

### 2. docker exec / podman exec

Use container runtime's native exec for access.

**Rejected because**: Requires the runtime's CLI on the client machine. Not accessible from remote machines without additional tooling.

### 3. Web terminal

Provide a browser-based terminal.

**Rejected because**: Adds significant complexity (web server, auth). Poor integration with AI agent workflows that expect real terminals.

### 4. VS Code Server built-in

Rely on each AI agent providing its own remote access.

**Rejected because**: Not all agents have this. Inconsistent experience.

## Notes

The gateway service (Phase 5) provides single-port access by running an SSH server that routes to the selected sandbox. This builds on the SSH foundation rather than replacing it.

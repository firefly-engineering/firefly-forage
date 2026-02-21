# Firefly Forage

**Isolated, ephemeral sandboxes for AI coding agents on NixOS.**

Firefly Forage is a NixOS module that creates lightweight, isolated environments for running AI coding assistants like Claude Code. Each sandbox is a systemd-nspawn container with:

- **Shared nix store** - Read-only bind mount, no duplication
- **Ephemeral root** - Fresh state on every reset
- **Persistent workspace** - Your project files survive restarts
- **Auth obfuscation** - API keys injected at runtime, not visible in environment

## Why Forage?

AI coding agents are powerful but unpredictable. They can:

- Install packages you didn't ask for
- Modify system configuration
- Accumulate cruft over long sessions
- Potentially exfiltrate sensitive data

Forage addresses these concerns by running agents in disposable containers. When things go wrong, just reset the sandbox and start fresh.

## Key Features

### Multi-Agent Support

Run multiple sandboxes simultaneously, each with its own:
- SSH port for direct access
- Tmux session for persistence
- Workspace bind mount

### JJ Workspace Integration

Create multiple sandboxes working on the same repository using [Jujutsu](https://github.com/martinvonz/jj) workspaces. Each agent gets an isolated working copy while sharing the repository's history.

```bash
# Two agents working on the same repo in parallel
forage-ctl up agent-a --template claude --repo ~/projects/myrepo
forage-ctl up agent-b --template claude --repo ~/projects/myrepo
```

### Composable Workspace Mounts

Assemble a sandbox's filesystem from multiple sources — mount multiple repos, overlay branches, and mix VCS-backed and literal bind mounts:

```bash
# Template mounts: main workspace + beads overlay + named data repo
forage-ctl up dev -t claude-beads --repo ~/projects/myrepo --repo data=~/datasets
```

### Nix Store Efficiency

Sandboxes share the host's `/nix/store` read-only. When an agent runs `nix shell nixpkgs#ripgrep`, the build happens on the host via the nix daemon socket—no duplication, instant availability.

### Template System

Define sandbox configurations declaratively in your NixOS config:

```nix
templates.claude = {
  description = "Claude Code sandbox";
  agents.claude = {
    package = pkgs.claude-code;
    secretName = "anthropic";
    authEnvVar = "ANTHROPIC_API_KEY";
  };
  extraPackages = with pkgs; [ ripgrep fd jq ];
  network = "full";
};
```

## Quick Example

```bash
# Create a sandbox for your project
forage-ctl up myproject -t claude -w ~/projects/myproject

# Connect and start working
forage-ctl ssh myproject

# Inside the sandbox, claude is ready to use
claude

# When done, clean up
forage-ctl down myproject
```

## Requirements

- NixOS (tested on 24.11+)
- systemd-nspawn (included in NixOS)
- [extra-container](https://github.com/erikarvstedt/extra-container) (managed by the module)

## Status

Firefly Forage has completed all planned implementation phases:

- Phases 1-3: Basic sandboxing, JJ workspaces, UX improvements
- Phase 4: Go rewrite of forage-ctl
- Phase 5: Gateway & interactive picker
- Phase 6: Network isolation modes
- Phase 7: API proxy for auth injection
- Phase 8: Git worktree backend
- Phase 9: Multi-runtime support (nspawn, Docker, Podman, Apple Container)

See the [DESIGN.md](https://github.com/firefly-engineering/firefly-forage/blob/main/DESIGN.md) for architecture details.

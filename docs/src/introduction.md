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

Firefly Forage is under active development. Phase 1 (basic sandboxing) and Phase 2 (JJ workspace integration) are complete. See the [roadmap](https://github.com/firefly-engineering/firefly-forage/blob/main/DESIGN.md#implementation-phases) for what's coming next.

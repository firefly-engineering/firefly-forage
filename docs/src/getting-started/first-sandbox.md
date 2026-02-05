# First Sandbox

This guide walks you through creating and using your first Forage sandbox.

## Prerequisites

- Forage is [installed](./installation.md) and [configured](./configuration.md)
- You have at least one template defined
- Your API key secrets are in place

## List Available Templates

First, see what templates are available:

```bash
forage-ctl templates
```

Output:
```
TEMPLATE        AGENTS              NETWORK    DESCRIPTION
claude          claude              full       Claude Code for general development
claude-isolated claude              none       Claude Code without network
```

## Create a Sandbox

Create a sandbox bound to a project directory:

```bash
forage-ctl up myproject --template claude --workspace ~/projects/myproject
```

You'll see output like:
```
ℹ Creating sandbox 'myproject' from template 'claude'
ℹ Mode: direct workspace
ℹ Workspace: /home/user/projects/myproject → /workspace
ℹ SSH port: 2200
ℹ Network slot: 1 (IP: 192.168.100.11)
ℹ Creating container with extra-container...
ℹ Waiting for SSH to become available on port 2200...
✓ Sandbox 'myproject' created successfully
ℹ Connect with: forage-ctl ssh myproject
```

## Connect to the Sandbox

SSH into the sandbox:

```bash
forage-ctl ssh myproject
```

This attaches to a tmux session inside the container. You'll land in `/workspace`, which is your project directory.

## Use the Agent

Inside the sandbox, the configured agent is ready to use:

```bash
# Start Claude Code
claude

# Or run a one-off command
claude "explain this codebase"
```

The agent has access to:
- Your project files in `/workspace`
- Tools specified in `extraPackages`
- Any nix package via `nix run nixpkgs#<package>`

## Tmux Basics

The sandbox uses tmux for session persistence:

- **Detach**: `Ctrl-b d` (leaves agent running)
- **Reattach**: `forage-ctl ssh myproject`
- **New window**: `Ctrl-b c`
- **Switch windows**: `Ctrl-b n` / `Ctrl-b p`
- **Scrollback**: `Ctrl-b [` then arrow keys, `q` to exit

## Check Sandbox Status

List running sandboxes:

```bash
forage-ctl ps
```

Output:
```
NAME            TEMPLATE   PORT   MODE  WORKSPACE                      STATUS
myproject       claude     2200   dir   /home/user/projects/myproject  ✓ healthy
```

## Reset if Needed

If the sandbox gets into a bad state, reset it:

```bash
forage-ctl reset myproject
```

This destroys and recreates the container while preserving:
- Your workspace files
- The sandbox configuration

## Clean Up

When done, remove the sandbox:

```bash
forage-ctl down myproject
```

This:
- Stops the container
- Removes secrets
- Cleans up metadata
- Removes injected skill files from workspace

## Next Steps

- Learn about [JJ workspaces](../usage/jj-workspaces.md) for parallel agent work
- See the full [CLI reference](../usage/cli-reference.md)
- Understand [skill injection](../usage/skill-injection.md)

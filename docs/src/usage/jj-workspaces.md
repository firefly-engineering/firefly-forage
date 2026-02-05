# JJ Workspaces

Forage integrates with [Jujutsu (jj)](https://github.com/martinvonz/jj) to enable multiple agents working on the same repository simultaneously, each with an isolated working copy.

## Overview

When you use `--repo` instead of `--workspace`, Forage:

1. Creates a JJ workspace at `/var/lib/forage/workspaces/<name>`
2. Bind mounts this workspace to `/workspace` in the container
3. Bind mounts the source repo's `.jj` directory so the workspace symlink resolves

Each sandbox gets its own working copy of the files, but they all share the repository's operation log and history.

```
┌─────────────────────────────────────────────────────────────────────┐
│ Host                                                                │
│                                                                     │
│  ~/projects/myrepo/                                                 │
│  ├── .jj/              ◄─────────────────────────┐                  │
│  ├── src/                                        │ shared           │
│  └── ...                                         │                  │
│                                                  │                  │
│  /var/lib/forage/workspaces/                     │                  │
│  ├── agent-a/        ◄── jj workspace ───────────┤                  │
│  │   ├── src/        (separate working copy)     │                  │
│  │   └── ...                                     │                  │
│  └── agent-b/        ◄── jj workspace ───────────┘                  │
│      ├── src/        (separate working copy)                        │
│      └── ...                                                        │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

## Creating JJ Sandboxes

### Prerequisites

Your project must be a JJ repository:

```bash
cd ~/projects/myrepo
jj git init --colocate  # or jj init
```

### Create Multiple Sandboxes

```bash
# First agent
forage-ctl up agent-a --template claude --repo ~/projects/myrepo

# Second agent on the same repo
forage-ctl up agent-b --template claude --repo ~/projects/myrepo

# Third agent with a different template
forage-ctl up agent-c --template multi --repo ~/projects/myrepo
```

Each sandbox appears as a JJ workspace:

```bash
jj workspace list -R ~/projects/myrepo
```

Output:
```
default: abc123 (no description set)
agent-a: def456 (empty) (no description set)
agent-b: ghi789 (empty) (no description set)
agent-c: jkl012 (empty) (no description set)
```

## Working with JJ Inside Sandboxes

When you connect to a JJ sandbox, the skill injection includes JJ-specific instructions:

```bash
forage-ctl ssh agent-a
```

Inside the sandbox, use JJ commands:

```bash
# Show status
jj status

# Show changes
jj diff

# Create a new change
jj new

# Describe your change
jj describe -m "Add feature X"

# See all changes
jj log
```

## Isolation Benefits

### Parallel Work

Each agent works on a separate JJ change:

```
agent-a: Working on feature-auth
agent-b: Working on feature-api
agent-c: Reviewing and testing
```

Changes don't interfere—each workspace has its own working copy.

### Easy Coordination

From the host, you can see all work:

```bash
# See all changes from all workspaces
jj log -R ~/projects/myrepo

# Squash agent work into main
jj squash --from agent-a -R ~/projects/myrepo
```

### Safe Experimentation

If an agent makes a mess:

```bash
# Reset just that sandbox
forage-ctl reset agent-a

# Or abandon the change in JJ
jj abandon agent-a -R ~/projects/myrepo
```

## Cleanup

When you remove a JJ sandbox, Forage:

1. Runs `jj workspace forget <name>`
2. Removes the workspace directory
3. Cleans up container and metadata

```bash
forage-ctl down agent-a
```

The changes made in that workspace remain in the repository history—only the workspace is removed.

## Comparison: --workspace vs --repo

| Aspect | `--workspace` | `--repo` |
|--------|---------------|----------|
| Working directory | Direct bind mount | JJ workspace |
| Multiple sandboxes | Need separate directories | Share same repo |
| Isolation | File-level (same files) | Change-level (separate working copies) |
| VCS | Any (git, jj, etc.) | JJ only |
| Cleanup | Removes skill files | Forgets JJ workspace |

**Use `--workspace` when:**
- Simple single-agent workflow
- Project doesn't use JJ
- You want direct file access

**Use `--repo` when:**
- Multiple agents on same codebase
- You want change isolation
- Project uses JJ for version control

## Troubleshooting

### "Not a jj repository"

The path doesn't contain a `.jj/repo` directory:

```bash
# Initialize JJ
cd ~/projects/myrepo
jj git init --colocate
```

### "JJ workspace already exists"

A workspace with that name already exists in the repo:

```bash
# Check existing workspaces
jj workspace list -R ~/projects/myrepo

# Use a different sandbox name, or remove the existing workspace
jj workspace forget existingname -R ~/projects/myrepo
```

### JJ commands fail inside sandbox

Ensure the source repo's `.jj` directory is accessible. The sandbox needs the bind mount to resolve the workspace symlink. This should be automatic—if it's not working, check:

```bash
# Inside sandbox
ls -la /workspace/.jj/
# Should show a symlink to the repo's .jj directory
```

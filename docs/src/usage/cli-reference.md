# CLI Reference

Complete reference for the `forage-ctl` command-line tool.

## Synopsis

```bash
forage-ctl <command> [options]
```

## Commands

### `templates`

List available sandbox templates.

```bash
forage-ctl templates
```

**Output:**
```
TEMPLATE        AGENTS              NETWORK    DESCRIPTION
claude          claude              full       Claude Code sandbox
multi           claude,aider        full       Multi-agent sandbox
```

---

### `up`

Create and start a sandbox.

```bash
forage-ctl up <name> --template <template> --repo <path> [options]
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `<name>` | Unique name for the sandbox |

**Options:**

| Option | Description |
|--------|-------------|
| `--template, -t <name>` | Template to use (required) |
| `--repo, -r <path>` | Repository or directory path (required) |
| `--direct` | Mount directory directly, skipping VCS isolation |
| `--ssh-key <key>` | SSH public key for sandbox access (can be repeated) |
| `--ssh-key-path <path>` | Path to SSH private key for agent push access |
| `--git-user <name>` | Git user.name for agent commits |
| `--git-email <email>` | Git user.email for agent commits |
| `--no-mux-config` | Don't mount host multiplexer config into sandbox |

**Workspace Modes:**

The workspace mode is determined automatically based on the `--repo` path and flags:

| Mode | Condition | Behavior |
|------|-----------|----------|
| Direct | `--direct` flag used | Mounts directory directly at `/workspace` |
| JJ workspace | Path contains `.jj/` directory | Creates isolated JJ workspace |
| Git worktree | Path contains `.git/` directory | Creates git worktree with branch `forage-<name>` |

**Examples:**

```bash
# Direct mount (no VCS isolation)
forage-ctl up myproject -t claude --repo ~/projects/myproject --direct

# JJ workspace (auto-detected, creates isolated working copy)
forage-ctl up agent-a -t claude --repo ~/projects/jj-repo

# Git worktree (auto-detected, creates isolated worktree)
forage-ctl up agent-b -t claude --repo ~/projects/git-repo

# With SSH key for push access
forage-ctl up myproject -t claude --repo ~/projects/myrepo --ssh-key-path ~/.ssh/id_ed25519

# With git identity for commits
forage-ctl up myproject -t claude --repo ~/projects/myrepo --git-user "Agent" --git-email "agent@example.com"
```

---

### `down`

Stop and remove a sandbox.

```bash
forage-ctl down <name>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `<name>` | Name of the sandbox to remove |

**Example:**

```bash
forage-ctl down myproject
```

**Cleanup performed:**
- Stops and destroys the container
- Removes secrets from `/var/lib/forage/secrets/<name>/`
- For JJ mode: runs `jj workspace forget` and removes workspace directory
- For git-worktree mode: removes the worktree
- Removes skills file and container configuration
- Deletes sandbox metadata

---

### `ps`

List sandboxes with health status.

```bash
forage-ctl ps
```

**Output:**
```
NAME            TEMPLATE   PORT   MODE        WORKSPACE                         STATUS
myproject       claude     2200   direct      /home/user/projects/myproj        ✓ healthy
agent-a         claude     2201   jj          ...forage/workspaces/agent-a      ✓ healthy
agent-b         claude     2202   git-worktree ...forage/workspaces/agent-b     ● stopped
```

**Columns:**

| Column | Description |
|--------|-------------|
| NAME | Sandbox name |
| TEMPLATE | Template used |
| PORT | SSH port |
| MODE | `direct` (direct mount), `jj` (JJ workspace), or `git-worktree` (git worktree) |
| WORKSPACE | Path mounted at `/workspace` |
| STATUS | Health status (see below) |

**Status values:**

| Status | Description |
|--------|-------------|
| `✓ healthy` | Container running, SSH reachable, tmux session active |
| `⚠ unhealthy` | Container running but SSH not reachable |
| `○ no-tmux` | Container running, SSH works, but no tmux session |
| `● stopped` | Container not running |

---

### `status`

Show detailed sandbox status and health information.

```bash
forage-ctl status <name>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `<name>` | Name of the sandbox |

**Example output:**
```
Sandbox: myproject
========================================

Configuration:
  Template:      claude
  Workspace:     /home/user/projects/myproject
  Mode:          direct
  SSH Port:      2200
  Container IP:  192.168.100.11
  Created:       2024-01-15T10:30:00+00:00

Container Status:
  Running:       yes
  Uptime:        2h 30m

Health Checks:
  SSH:           reachable
  Tmux Session:  active
  Tmux Windows:
    - 0:bash
    - 1:claude

Connect:
  forage-ctl ssh myproject
  ssh -p 2200 agent@localhost
```

Use this command for debugging connectivity issues or checking sandbox health.

---

### `ssh`

Connect to a sandbox via SSH, attaching to the tmux session.

```bash
forage-ctl ssh <name>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `<name>` | Name of the sandbox |

This runs:
```bash
ssh -p <port> -t agent@localhost 'tmux attach -t forage || tmux new -s forage'
```

**Tmux controls:**
- Detach: `Ctrl-b d`
- New window: `Ctrl-b c`
- Next/prev window: `Ctrl-b n` / `Ctrl-b p`

---

### `exec`

Execute a command inside a sandbox.

```bash
forage-ctl exec <name> -- <command>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `<name>` | Name of the sandbox |
| `<command>` | Command to execute |

**Examples:**

```bash
# Check agent version
forage-ctl exec myproject -- claude --version

# Run a script
forage-ctl exec myproject -- bash -c 'cd /workspace && ./build.sh'

# List files
forage-ctl exec myproject -- ls -la /workspace
```

---

### `start`

Start an agent in the sandbox's tmux session.

```bash
forage-ctl start <name> [agent]
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `<name>` | Name of the sandbox |
| `[agent]` | Agent to start (optional, defaults to first agent in template) |

**Examples:**

```bash
# Start the default agent
forage-ctl start myproject

# Start a specific agent (in multi-agent templates)
forage-ctl start myproject claude
forage-ctl start myproject aider
```

This sends the agent command to the existing tmux session. Use `forage-ctl ssh` to attach and interact with the agent.

---

### `shell`

Open a shell in a new tmux window.

```bash
forage-ctl shell <name>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `<name>` | Name of the sandbox |

This creates a new tmux window in the sandbox's session and attaches to it. Useful for running commands alongside a running agent.

**Tmux window navigation:**
- Switch windows: `Ctrl-b n` (next) / `Ctrl-b p` (previous)
- List windows: `Ctrl-b w`
- Close window: `exit` or `Ctrl-d`

---

### `logs`

Show container logs.

```bash
forage-ctl logs <name> [-f] [-n <lines>]
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `<name>` | Name of the sandbox |

**Options:**

| Option | Description |
|--------|-------------|
| `-f, --follow` | Follow log output (like `tail -f`) |
| `-n, --lines <n>` | Number of lines to show (default: 100) |

**Examples:**

```bash
# Show last 100 lines
forage-ctl logs myproject

# Follow logs in real-time
forage-ctl logs myproject -f

# Show last 500 lines
forage-ctl logs myproject -n 500
```

This uses `journalctl` to show logs from the container's systemd services (sshd, tmux, etc.).

---

### `reset`

Reset a sandbox to fresh state.

```bash
forage-ctl reset <name>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `<name>` | Name of the sandbox |

This destroys and recreates the container while preserving:
- Workspace files
- Sandbox configuration (template, port, network slot)
- JJ workspace association (if applicable)

Use this when:
- The container is in a bad state
- You want a fresh environment
- The agent has polluted the container filesystem

---

### `network`

Change sandbox network isolation mode.

```bash
forage-ctl network <name> <mode> [--allow <host>...] [--no-restart]
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `<name>` | Name of the sandbox |
| `<mode>` | Network mode: `full`, `restricted`, or `none` |

**Options:**

| Option | Description |
|--------|-------------|
| `--allow <host>` | Additional hosts to allow (restricted mode only) |
| `--no-restart` | Don't restart sandbox (changes won't take effect until reset) |

**Modes:**

| Mode | Description |
|------|-------------|
| `full` | Unrestricted internet access (default) |
| `restricted` | Only allowed hosts can be accessed |
| `none` | No network access except SSH for management |

**Examples:**

```bash
# Switch to no network
forage-ctl network myproject none

# Switch to restricted with allowed hosts
forage-ctl network myproject restricted --allow api.anthropic.com
```

---

### `gateway`

Interactive sandbox selector (gateway mode).

```bash
forage-ctl gateway [sandbox-name]
```

If a sandbox name is provided, connects directly. Otherwise, presents an interactive picker.

This command is designed to be used as a login shell for SSH access, providing a single entry point to all sandboxes.

---

### `pick`

Interactive sandbox picker.

```bash
forage-ctl pick
```

Opens a TUI for selecting and connecting to sandboxes.

**Controls:**
- Arrow keys or `j/k` to navigate
- `/` to filter
- `Enter` to connect
- `n` to show new sandbox instructions
- `d` to show remove instructions
- `q` or `Esc` to quit

---

### `proxy`

Start the API proxy server.

```bash
forage-ctl proxy [--port <port>] [--host <host>]
```

Starts an HTTP proxy that injects API keys into requests. Used for sandboxes that need auth injection without storing secrets in the container.

---

### `runtime`

Show container runtime information.

```bash
forage-ctl runtime
```

Displays the active container runtime and lists available runtimes on the system.

**Supported runtimes:**
- `nspawn` - NixOS (systemd-nspawn via extra-container)
- `apple` - macOS 13+ (Apple Virtualization.framework)
- `podman` - Linux, macOS (rootless preferred)
- `docker` - Linux, macOS, Windows

---

### `gc`

Garbage collect orphaned sandbox resources.

```bash
forage-ctl gc [--force]
```

**Options:**

| Option | Description |
|--------|-------------|
| `--force` | Actually remove orphaned resources (default is dry run) |

This command reconciles disk state with runtime state and removes orphaned resources. Without `--force`, it performs a dry run showing what would be cleaned.

**Detects:**

| Type | Description |
|------|-------------|
| Orphaned files | Sandbox files on disk with no matching container |
| Orphaned containers | Containers in runtime with no matching metadata on disk |
| Stale metadata | Metadata files for sandboxes whose container no longer exists |

**Examples:**

```bash
# Dry run - show what would be cleaned
forage-ctl gc

# Actually clean up orphaned resources
forage-ctl gc --force
```

**Use cases:**

- After a system crash that left containers in an inconsistent state
- When manual cleanup left orphaned files
- Periodic maintenance to reclaim disk space

---

### `help`

Show help message.

```bash
forage-ctl help
forage-ctl --help
forage-ctl -h
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Sandbox not found |
| 3 | Template not found |
| 4 | Port/slot allocation failed |
| 5 | Container operation failed |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `FORAGE_CONFIG_DIR` | `/etc/firefly-forage` | Configuration directory |
| `FORAGE_STATE_DIR` | `/var/lib/firefly-forage` | State directory |

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
forage-ctl up <name> --template <template> [--workspace <path> | --repo <path>] [--port <port>]
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `<name>` | Unique name for the sandbox |

**Options:**

| Option | Description |
|--------|-------------|
| `--template, -t <name>` | Template to use (required) |
| `--workspace, -w <path>` | Directory to mount at `/workspace` |
| `--repo, -r <path>` | JJ repository to create workspace from |
| `--port, -p <port>` | Specific SSH port (auto-assigned if omitted) |

> **Note:** `--workspace` and `--repo` are mutually exclusive. One is required.

**Examples:**

```bash
# Direct workspace mount
forage-ctl up myproject -t claude -w ~/projects/myproject

# JJ workspace (creates isolated working copy)
forage-ctl up agent-a -t claude --repo ~/projects/myrepo

# Specific port
forage-ctl up myproject -t claude -w ~/projects/myproject -p 2205
```

---

### `down`

Stop and remove a sandbox.

```bash
forage-ctl down <name> [--keep-skills]
forage-ctl down --all
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `<name>` | Name of the sandbox to remove |

**Options:**

| Option | Description |
|--------|-------------|
| `--keep-skills` | Don't remove injected skill files |
| `--all` | Remove all sandboxes |

**Examples:**

```bash
# Remove single sandbox
forage-ctl down myproject

# Remove but keep .claude/forage-skills.md
forage-ctl down myproject --keep-skills

# Remove all sandboxes
forage-ctl down --all
```

**Cleanup performed:**
- Stops and destroys the container
- Removes secrets from `/run/forage-secrets/<name>/`
- For JJ mode: runs `jj workspace forget` and removes workspace directory
- For direct mode: removes `.claude/forage-skills.md` (unless `--keep-skills`)
- Deletes sandbox metadata

---

### `ps`

List sandboxes with health status.

```bash
forage-ctl ps
```

**Output:**
```
NAME            TEMPLATE   PORT   MODE WORKSPACE                    HEALTH
myproject       claude     2200   dir  /home/user/projects/myproj   healthy
agent-a         claude     2201   jj   ...forage/workspaces/agent-a healthy
agent-b         claude     2202   jj   ...forage/workspaces/agent-b stopped
```

**Columns:**

| Column | Description |
|--------|-------------|
| NAME | Sandbox name |
| TEMPLATE | Template used |
| PORT | SSH port |
| MODE | `dir` (direct workspace) or `jj` (JJ workspace) |
| WORKSPACE | Path mounted at `/workspace` |
| HEALTH | Health status (see below) |

**Health statuses:**

| Status | Description |
|--------|-------------|
| `healthy` | Container running, SSH reachable, tmux session active |
| `unhealthy` | Container running but SSH not reachable |
| `no-tmux` | Container running, SSH works, but no tmux session |
| `stopped` | Container not running |

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

### `ssh-cmd`

Print the SSH command for connecting to a sandbox.

```bash
forage-ctl ssh-cmd <name>
```

**Output:**
```
ssh -p 2200 -o StrictHostKeyChecking=no agent@myhost
```

Useful for connecting from remote machines or configuring SSH clients.

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

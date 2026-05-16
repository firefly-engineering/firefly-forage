# Skill Injection

Forage automatically injects "skills"—configuration files that teach AI agents about the sandbox environment and available tools.

## How It Works

When a sandbox is created, Forage generates `.claude/forage-skills.md` in the workspace directory. This file is automatically loaded by Claude Code alongside any existing project instructions.

```
workspace/
├── .claude/
│   ├── forage-skills.md    ◄── Injected by Forage
│   └── settings.json       ◄── Your project settings (untouched)
├── CLAUDE.md               ◄── Your project instructions (untouched)
└── src/
```

## Injected Content

The generated skill file includes:

### Environment Information

```markdown
# Forage Sandbox Skills

You are running inside a Firefly Forage sandbox named `myproject`.

## Environment

- **Workspace**: `/workspace` (your working directory)
- **Network**: Full internet access
- **Session**: tmux session `forage` (persistent across reconnections)
```

### Available Agents

Lists the agents configured in the template:

```markdown
## Available Agents

claude
```

### JJ Instructions (if applicable)

For sandboxes created with `--repo`:

```markdown
## Version Control: JJ (Jujutsu)

This workspace uses `jj` for version control:

- `jj status` - Show working copy status
- `jj diff` - Show changes
- `jj new` - Create new change
- `jj describe -m ""` - Set commit message
- `jj bookmark set` - Update bookmark

This is an isolated jj workspace - changes don't affect other workspaces.
```

### Sandbox Constraints

```markdown
## Sandbox Constraints

- The root filesystem is ephemeral (tmpfs) - changes outside /workspace are lost on restart
- `/nix/store` is read-only (shared from host)
- `/workspace` is your persistent working directory
- Secrets are mounted read-only at `/run/secrets/`
```

### Nix Usage

```markdown
## Installing Additional Tools

Any tool not pre-installed can be used via Nix:

- `nix run nixpkgs#ripgrep -- --help` - Run a tool once
- `nix shell nixpkgs#jq nixpkgs#yq` - Enter a shell with multiple tools
- `nix run github:owner/repo` - Build and run a flake

This works because `/nix/store` is shared (read-only) and the Nix daemon
handles all builds on the host.
```

### Tips and Sub-Agent Information

```markdown
## Tips

- Use `tmux` for long-running processes
- All project work should be done in `/workspace`
- The sandbox can be reset with `forage-ctl reset myproject` from the host

## Sub-Agent Spawning

When spawning sub-agents (e.g., with Claude Code's Task tool):
- Sub-agents share this same sandbox environment
- Use tmux windows/panes for parallel agent work
- Each sub-agent has access to the same workspace and tools
```

## Skill Priority

Claude Code loads instructions in this order:

1. **Project CLAUDE.md** - Your existing project instructions (highest priority)
2. **Forage skills** - Injected `.claude/forage-skills.md`
3. **User settings** - From `.claude/settings.json`

The Forage skills supplement rather than override your project documentation.

## Cleanup

When a sandbox is removed with `forage-ctl down`:

- **Direct mode (`--workspace`)**: The skill file is removed from the workspace
- **JJ mode (`--repo`)**: The entire workspace directory is removed, including skills
- **Git worktree mode (`--git-worktree`)**: The worktree is removed, including skills

## Composite Workspace Layout

For sandboxes with [composable workspace mounts](./workspace-mounts.md), the skill file includes a description of the full mount layout:

```markdown
## Workspace Layout

Your workspace contains multiple mount sources:
- /workspace: jj workspace from ~/projects/myrepo
- /workspace/.beads: jj workspace (branch beads-sync) from ~/projects/myrepo
- /workspace/data: direct mount from ~/datasets (read-only)
```

This gives the agent context about what's mounted where and how each path is managed.

## Dynamic Skill Generation

Skills are dynamically generated based on project analysis. The skills analyzer (`internal/skills/analyzer.go`) detects:

- **Project type**: Go, Rust, Python, Node/TypeScript, Nix, and more
- **Build system**: detected build commands (e.g., `go build`, `cargo build`, `npm run build`)
- **Test commands**: detected test runners (e.g., `go test ./...`, `cargo test`, `pytest`)
- **Frameworks**: detected web frameworks and libraries
- **VCS**: Git or JJ repository detection

Based on detection results, the injected skill content includes project-specific guidance for the agent (build/test commands, VCS workflow tips, etc.).

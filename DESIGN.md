# Firefly Forage - Design Document

Firefly Forage is a NixOS module for creating isolated, ephemeral sandboxes to run AI coding agents safely.

## Goals

1. **Isolation** - Run AI agents in contained environments
2. **Efficiency** - Share the nix store read-only, no duplication
3. **Disposability** - Ephemeral container roots, easy reset
4. **Multi-agent** - Support multiple concurrent sandboxes
5. **Security** - Auth obfuscation, optional network isolation
6. **Usability** - Simple CLI, automatic SSH access

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│ Host Machine                                                    │
│                                                                 │
│  nix-daemon ◄──────────────────────────────┐                    │
│       │                                    │                    │
│       ▼                                    │                    │
│  /nix/store ◄──────────────────────────────┼───────────┐        │
│  (writable by daemon)                      │           │        │
│                                            │           │        │
│  ┌─────────────────────────────┐  ┌────────┴───────────┴──┐     │
│  │ sandbox-project-a           │  │ sandbox-project-b     │     │
│  │                             │  │                       │     │
│  │ /nix/store (ro bind)        │  │ /nix/store (ro bind)  │     │
│  │ /nix/var/nix/daemon-socket  │  │ /nix/var/nix/daemon.. │     │
│  │ /workspace ──► ~/proj-a     │  │ /workspace ──► ~/pr.. │     │
│  │ /run/secrets (ro bind)      │  │ /run/secrets (ro ..)  │     │
│  │                             │  │                       │     │
│  │ agent: claude               │  │ agents: claude, open  │     │
│  │ sshd :22 ──► host:2200      │  │ sshd :22 ──► host:22. │     │
│  └─────────────────────────────┘  └───────────────────────┘     │
│                                                                 │
│  forage-ctl (CLI)                                               │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## Core Concepts

### Sandbox Template

A declarative specification for a type of sandbox:
- Which agents are available
- Extra packages to include
- Network access policy
- Resource limits (future)

### Sandbox Instance

A running container created from a template:
- Bound to a specific workspace directory
- Allocated a unique SSH port
- Ephemeral root filesystem (tmpfs)
- Persistent workspace via bind mount

### Agent Wrapper

A generated binary that:
- Reads auth from a bind-mounted secret file
- Sets environment variables for the agent
- Executes the real agent binary
- Keeps global environment clean (auth obfuscation)

## Design Decisions

### Container Backend: systemd-nspawn

We use NixOS containers (systemd-nspawn) because:
- Native NixOS integration
- Lightweight (shares kernel)
- Excellent bind mount support
- Built-in networking options
- No portability requirement outside NixOS

### Nix Store: Read-Only with Daemon Socket

The nix store is bind-mounted read-only. All nix operations go through the host's nix daemon via a bind-mounted socket.

**Bind mounts required:**
```nix
"/nix/store" = { hostPath = "/nix/store"; isReadOnly = true; };
"/nix/var/nix/daemon-socket" = { hostPath = "/nix/var/nix/daemon-socket"; };
```

**Why this works:**
1. When `/nix/store` is read-only, nix client detects it can't write
2. Client automatically uses daemon mode
3. Daemon on host performs actual store writes
4. Container sees new paths via the same bind mount
5. Content-addressed store means no conflicts

**Verified:** Tested with `unshare --mount` and confirmed nix builds work.

### Nix Registry Pinning

Sandboxes automatically have a pinned nix registry that matches the host's nixpkgs version:

```nix
# Generated in container config
environment.etc."nix/registry.json".text = builtins.toJSON {
  version = 2;
  flakes = [
    {
      from = { type = "indirect"; id = "nixpkgs"; };
      to = {
        type = "github";
        owner = "NixOS";
        repo = "nixpkgs";
        rev = "...";  # Automatically set to host's nixpkgs revision
      };
    }
  ];
};
```

**Benefits:**
- All agents use the same nixpkgs version
- Reproducible tool installations across sandboxes
- No accumulation of different nixpkgs versions in store
- Pinned to the same nixpkgs used to build the sandbox

**Implementation:** The host module exposes its nixpkgs input revision via `config.json`, and the container config generator injects this into each sandbox's registry.

### Instance Tracking: Stateless

Instead of maintaining state files, we derive instance information from:
- Running systemd-nspawn containers (machinectl list)
- Container naming convention: `forage-{name}`
- Introspection of bind mounts for workspace info

Benefits:
- No state to corrupt or get out of sync
- System is the source of truth
- Simpler implementation

### User Identity: Same UID as Host

The container runs with a user that has the same UID/GID as the host user who created the sandbox. This ensures:
- No permission issues with bind-mounted workspace
- Files created in workspace have correct ownership
- No need for complex UID mapping

### Auth Obfuscation

Agent authentication is handled via wrapper binaries:

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
- Reads auth from a file (not environment variable)
- Sets env var only for the child process
- Agent cannot easily discover where auth came from
- Provides minimal protection against credential exfiltration

### JJ Workspace Integration

Each sandbox uses a separate jj workspace, enabling parallel agent work on the same repository without conflicts.

```
┌─────────────────────────────────────────────────────────────────────┐
│ Host                                                                │
│                                                                     │
│  ~/projects/myrepo/                                                 │
│  ├── .jj/              ◄─────────────────────────┐                  │
│  ├── src/                                        │ shared           │
│  └── ...                                         │ (read-only)      │
│                                                  │                  │
│  /var/lib/forage/workspaces/                     │                  │
│  ├── sandbox-a/        ◄── jj workspace ─────────┤                  │
│  │   ├── src/          (separate working copy)   │                  │
│  │   └── ...                                     │                  │
│  └── sandbox-b/        ◄── jj workspace ─────────┘                  │
│      ├── src/          (separate working copy)                      │
│      └── ...                                                        │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

**How it works:**
1. `forage-ctl up` creates a jj workspace at a persistent location
2. The workspace shares the repo's `.jj` directory (operation log, etc.)
3. Each sandbox gets its own working copy of the files
4. Changes in one sandbox don't affect others until committed
5. Agents can work in parallel on different changes

**CLI integration:**
```bash
# Create sandbox with jj workspace
forage-ctl up agent-a --template claude --repo ~/projects/myrepo

# This internally runs:
# jj workspace add /var/lib/forage/workspaces/agent-a --name agent-a

# Multiple agents on same repo
forage-ctl up agent-b --template claude --repo ~/projects/myrepo
forage-ctl up agent-c --template opencode --repo ~/projects/myrepo
```

**Cleanup:**
```bash
# Remove sandbox and its workspace
forage-ctl down agent-a
# Internally: jj workspace forget agent-a && rm -rf workspace
```

### Skill Injection

Sandboxes automatically include "skills" - configuration that teaches agents about available tools and project conventions.

**Injection location:** `.claude/forage-skills.md` (or similar)

This avoids modifying the project's `CLAUDE.md` which may contain valuable upstream information. Claude Code loads instructions from multiple files in `.claude/`.

```
workspace/
├── .claude/
│   ├── forage-skills.md    ◄── Injected by forage (sandbox-specific)
│   └── settings.json       ◄── May also inject settings here
├── CLAUDE.md               ◄── Untouched (from upstream repo)
└── src/
```

**Injected content (.claude/forage-skills.md):**
```markdown
# Firefly Forage Sandbox Environment

This workspace is running inside a Firefly Forage sandbox.

## Version Control: JJ (Jujutsu)

Use `jj` instead of `git` for all version control operations:

- `jj status` - Show working copy status
- `jj diff` - Show changes
- `jj new` - Create new change
- `jj describe -m "message"` - Set change description
- `jj bookmark set main` - Update bookmark

This is an isolated jj workspace. Your changes won't affect other
workspaces until you explicitly share them.

## Available Tools

- `rg` (ripgrep) - Fast recursive search
- `fd` - Fast file finder
- `jq` - JSON processing
- `nix build` - Build nix expressions (uses host daemon)

## Sandbox Constraints

- The nix store is read-only (builds go through host daemon)
- Network access: [full|restricted|none]
- This container is ephemeral - only /workspace persists
```

**Skill sources (in priority order):**
1. **Project skills**: From repo's existing `CLAUDE.md` (untouched, highest priority)
2. **Forage skills**: Injected `.claude/forage-skills.md` (sandbox-aware instructions)
3. **Template skills**: From sandbox template configuration
4. **User skills**: Custom per-sandbox overrides

**Configuration:**
```nix
templates.claude = {
  skills = {
    jj = true;           # Include jj skill (default: true)
    nix = true;          # Include nix skill (default: true)

    # Additional custom instructions
    custom = ''
      ## Testing Requirements
      Always write tests before implementation.
    '';
  };

  # Optionally inject into .claude/settings.json
  claudeSettings = {
    # Any claude-code settings to inject
  };
};
```

**Cleanup:** The injected `.claude/forage-skills.md` is created at sandbox start and can be removed on sandbox down if desired (though it's harmless to leave).

### Tmux Session Management

Each sandbox runs the agent inside a tmux session for better terminal handling and attach/detach capability.

```
┌─────────────────────────────────────────────────────────────┐
│ Sandbox Container                                           │
│                                                             │
│  tmux session: "forage"                                     │
│  ┌─────────────────────────────────────────────────────┐    │
│  │ Window 0: agent                                     │    │
│  │ ┌─────────────────────────────────────────────────┐ │    │
│  │ │ $ claude                                        │ │    │
│  │ │ Claude Code ready...                            │ │    │
│  │ │ >                                               │ │    │
│  │ └─────────────────────────────────────────────────┘ │    │
│  └─────────────────────────────────────────────────────┘    │
│                                                             │
│  sshd                                                       │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

**Benefits:**
- **Attach/detach**: Connect to running agent, disconnect without stopping it
- **Session persistence**: Agent keeps running if SSH disconnects
- **Multiple windows**: Agent in one window, shell in another
- **Scrollback**: Review agent's previous output
- **Resilience**: Survives network interruptions
- **Sub-agent support**: Compatible with tools like opencode extensions that spawn sub-agents in tmux panes

**CLI integration:**
```bash
# Connect to sandbox (attaches to tmux session)
forage-ctl ssh myproject
# → ssh ... -t 'tmux attach -t forage'

# Start agent in sandbox (creates tmux session)
forage-ctl start myproject
# → Creates tmux session, starts claude in it

# Detach: Ctrl-b d (standard tmux)
# Reattach: forage-ctl ssh myproject

# Run shell alongside agent
forage-ctl shell myproject
# → Attaches to tmux, creates new window with shell
```

**Tmux configuration:**
```bash
# /etc/tmux.conf in sandbox
set -g prefix C-b
set -g mouse on
set -g history-limit 50000
set -g status-style 'bg=colour235 fg=colour136'
set -g status-left '[forage] '
```

### Gateway Access (Future)

Instead of exposing one SSH port per sandbox, a single gateway service provides access to all sandboxes through a selection interface.

```
┌─────────────────────────────────────────────────────────────────┐
│ Host Machine                                                    │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │ forage-gateway (port 2200)                              │    │
│  │                                                         │    │
│  │  ┌─────────────────────────────────────────────────┐    │    │
│  │  │  Firefly Forage - Select Sandbox                │    │    │
│  │  │                                                 │    │    │
│  │  │  > myproject     claude    running  2h ago      │    │    │
│  │  │    agent-a       claude    running  30m ago     │    │    │
│  │  │    agent-b       multi     running  5m ago      │    │    │
│  │  │                                                 │    │    │
│  │  │  [Enter] Attach  [n] New  [d] Down  [q] Quit    │    │    │
│  │  └─────────────────────────────────────────────────┘    │    │
│  └─────────────────────────────────────────────────────────┘    │
│                           │                                     │
│                           ▼                                     │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │ sandbox-myproj  │  │ sandbox-agent-a │  │ sandbox-agent-b │  │
│  │ tmux: forage    │  │ tmux: forage    │  │ tmux: forage    │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**Benefits:**
- **Single port**: Only one port to expose/forward for remote access
- **Discoverability**: See all sandboxes at a glance
- **Simpler firewall**: No dynamic port range needed
- **Better UX**: Interactive selection instead of remembering names

**Implementation options:**
1. **TUI selector**: fzf/gum-based picker that runs `machinectl shell` or SSH to selected sandbox
2. **Custom shell**: Login shell that presents the picker, then `exec`s into chosen sandbox
3. **SSH ForceCommand**: SSH config that runs the selector before allowing access

**Access patterns:**
```bash
# Interactive: land in selector
ssh -p 2200 forage@hostname

# Direct: skip selector, go straight to sandbox
ssh -p 2200 forage@hostname myproject

# From selector, attach to sandbox's tmux session
# → machinectl shell forage-myproject /bin/bash -c 'tmux attach -t forage'
```

### Network Isolation Modes

| Mode | Description | Use Case |
|------|-------------|----------|
| `full` | Unrestricted internet | Default, needed for API calls |
| `restricted` | Allowlist of hosts | Limit to specific APIs |
| `none` | No network (except daemon) | Maximum isolation |

Implementation:
- `full`: Use host network or NAT
- `restricted`: nftables rules in container
- `none`: Private network with no routing

## Module Structure

```
firefly-forage/
├── flake.nix                     # Flake definition
├── DESIGN.md                     # This document
├── README.md                     # User documentation
│
├── modules/
│   ├── host.nix                  # NixOS module for host machine
│   └── sandbox.nix               # Container configuration generator
│
├── lib/
│   ├── mkSandbox.nix             # Sandbox template builder
│   ├── mkAgentWrapper.nix        # Auth wrapper generator
│   └── types.nix                 # Custom types for options
│
└── packages/
    └── forage-ctl/               # CLI management tool
        ├── default.nix
        └── forage-ctl.sh
```

## Configuration Interface

### Host Configuration

```nix
{ inputs, config, pkgs, ... }:
{
  imports = [ inputs.firefly-forage.nixosModules.default ];

  services.firefly-forage = {
    enable = true;

    # SSH access to sandboxes
    authorizedKeys = config.users.users.myuser.openssh.authorizedKeys.keys;

    # Port range for sandbox SSH (one port per instance)
    portRange = { from = 2200; to = 2299; };

    # User identity for sandbox (UID/GID matching)
    user = "myuser";

    # Secrets (paths to files containing API keys)
    secrets = {
      anthropic = config.sops.secrets.anthropic-api-key.path;
      openai = config.sops.secrets.openai-api-key.path;
    };

    # Sandbox templates
    templates = {
      claude = {
        description = "Claude Code agent sandbox";

        agents.claude = {
          package = pkgs.claude-code;
          secretName = "anthropic";
          authEnvVar = "ANTHROPIC_API_KEY";
        };

        extraPackages = with pkgs; [
          ripgrep
          fd
          jq
          tree
        ];

        network = "full";
      };

      multi = {
        description = "Multi-agent sandbox";

        agents = {
          claude = {
            package = pkgs.claude-code;
            secretName = "anthropic";
            authEnvVar = "ANTHROPIC_API_KEY";
          };
          opencode = {
            package = pkgs.opencode;
            secretName = "openai";
            authEnvVar = "OPENAI_API_KEY";
          };
        };

        extraPackages = with pkgs; [ ripgrep fd ];
        network = "full";
      };

      isolated = {
        description = "Network-isolated sandbox";

        agents.claude = {
          package = pkgs.claude-code;
          secretName = "anthropic";
          authEnvVar = "ANTHROPIC_API_KEY";
        };

        network = "none";
      };
    };
  };
}
```

## CLI Interface

### Commands

```bash
# List available templates
forage-ctl templates
TEMPLATE    AGENTS          NETWORK    DESCRIPTION
claude      claude          full       Claude Code agent sandbox
multi       claude,opencode full       Multi-agent sandbox
isolated    claude          none       Network-isolated sandbox

# Create and start a sandbox (with workspace directory)
forage-ctl up <name> --template <template> --workspace <path>
forage-ctl up myproject --template claude --workspace ~/projects/myproject

# Create and start a sandbox (with jj repo - creates workspace automatically)
forage-ctl up <name> --template <template> --repo <path>
forage-ctl up agent-a --template claude --repo ~/projects/myrepo
forage-ctl up agent-b --template claude --repo ~/projects/myrepo  # parallel work!

# List running sandboxes
forage-ctl ps
NAME        TEMPLATE    PORT    WORKSPACE                      STATUS    TMUX
myproject   claude      2200    /home/user/projects/myproject  running   attached
agent-a     claude      2201    /var/lib/forage/ws/agent-a     running   detached
agent-b     claude      2202    /var/lib/forage/ws/agent-b     running   detached

# Connect to sandbox (attaches to tmux session)
forage-ctl ssh <name>
forage-ctl ssh myproject

# Get SSH command (for use from remote machines)
forage-ctl ssh-cmd <name>
# Output: ssh -p 2200 -t agent@hostname 'tmux attach -t forage'

# Start the agent in sandbox (if not already running)
forage-ctl start <name>

# Open a shell window alongside the agent
forage-ctl shell <name>

# Execute command in sandbox
forage-ctl exec <name> -- <command>
forage-ctl exec myproject -- claude --version

# Reset sandbox (restart with fresh ephemeral state, keeps workspace)
forage-ctl reset <name>

# Stop and remove sandbox (and jj workspace if created)
forage-ctl down <name>

# Stop and remove all sandboxes
forage-ctl down --all
```

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Sandbox not found |
| 3 | Template not found |
| 4 | Port allocation failed |
| 5 | Container start failed |

## Implementation Phases

### Phase 1: Basic Sandbox

- [x] Flake structure and module skeleton
- [x] Basic host module with template definitions
- [x] Container configuration generator
- [x] Agent wrapper generator
- [x] forage-ctl: up, down, ps, ssh
- [x] Port allocation (find free ports)
- [x] Tmux session management
- [x] Basic skill injection (.claude/forage-skills.md)
- [x] Documentation (mdbook)

### Phase 2: JJ Workspace Integration

- [x] Workspace creation on sandbox up
- [x] Workspace cleanup on sandbox down
- [x] Mount configuration for shared .jj
- [x] Handle workspace conflicts
- [x] forage-ctl: --repo flag for jj integration

### Phase 3: Robustness & UX

- [x] Better port allocation (find free ports)
- [ ] Health checks
- [ ] Logging integration
- [x] forage-ctl: exec, reset
- [x] forage-ctl: logs, start, shell
- [ ] Error handling improvements
- [ ] Advanced skill injection (project analysis)
- [ ] Gateway service with sandbox selector (single port access)
- [ ] TUI picker for sandbox selection
- [x] Nix registry pinning (pin nixpkgs to host version)

### Phase 4: Network Isolation

- [ ] nftables rules for restricted mode
- [ ] DNS filtering
- [ ] Network mode switching

### Phase 5: API Bridge (Future)

- [ ] Proxy service running on host
- [ ] Auth injection at proxy level
- [ ] Rate limiting
- [ ] Audit logging
- [ ] Secrets never enter sandbox

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│ Sandbox         │     │ API Bridge       │     │ External APIs   │
│                 │     │ (on host)        │     │                 │
│ claude-wrapper ─┼────►│ - Auth injection │────►│ api.anthropic.  │
│  (no secrets)   │     │ - Rate limiting  │     │                 │
│                 │     │ - Audit logs     │     │                 │
└─────────────────┘     └──────────────────┘     └─────────────────┘
```

### Phase 6: Git Worktree Backend

Alternative to JJ workspaces for projects using plain git.

- [ ] `--git-worktree` flag as alternative to `--repo`
- [ ] `git worktree add` on sandbox creation
- [ ] `git worktree remove` on sandbox cleanup
- [ ] Skill injection with git-specific instructions
- [ ] Handle worktree conflicts and naming

```bash
# Usage
forage-ctl up agent-a --template claude --git-worktree ~/projects/myrepo
forage-ctl up agent-b --template claude --git-worktree ~/projects/myrepo

# Internally:
# git worktree add /var/lib/forage/workspaces/agent-a -b agent-a
```

### Phase 7: Container Runtime Abstraction

Abstract the container backend to support multiple platforms.

- [ ] Define container runtime interface (create, destroy, exec, status)
- [ ] systemd-nspawn backend (NixOS, current implementation)
- [ ] Apple Container backend (macOS via github.com/apple/container)
- [ ] Docker/Podman backend (universal fallback)
- [ ] Runtime auto-detection based on platform
- [ ] Consistent bind mount semantics across runtimes
- [ ] Platform-specific nix store sharing strategies

```
┌─────────────────────────────────────────────────────────────────┐
│ forage-ctl                                                      │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │ Container Runtime Interface                              │    │
│  │  - create(name, config) -> Container                     │    │
│  │  - destroy(name)                                         │    │
│  │  - exec(name, command) -> Output                         │    │
│  │  - status(name) -> Status                                │    │
│  └─────────────────────────────────────────────────────────┘    │
│         │                    │                    │              │
│         ▼                    ▼                    ▼              │
│  ┌─────────────┐     ┌─────────────┐     ┌─────────────┐        │
│  │ nspawn      │     │ apple/      │     │ docker/     │        │
│  │ (NixOS)     │     │ container   │     │ podman      │        │
│  │             │     │ (macOS)     │     │ (fallback)  │        │
│  └─────────────┘     └─────────────┘     └─────────────┘        │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**Platform considerations:**

| Platform | Runtime | Nix Store Strategy |
|----------|---------|-------------------|
| NixOS | systemd-nspawn | Direct bind mount (current) |
| macOS | apple/container | nix-darwin store or Determinate Nix |
| Linux (other) | Docker/Podman | Volume mount or bind mount |

## Security Considerations

### Threat Model

**Trusted:**
- Host system administrator
- Nix store contents (from nixpkgs/trusted sources)

**Untrusted:**
- AI agent behavior
- Code being worked on in workspace

### Mitigations

| Threat | Mitigation |
|--------|------------|
| Agent exfiltrates API keys | Auth obfuscation via wrappers |
| Agent accesses host filesystem | Container isolation, bind mounts only |
| Agent makes unwanted network calls | Network isolation modes |
| Agent corrupts sandbox | Ephemeral root, easy reset |
| Agent escapes container | systemd-nspawn security features |

### Limitations

- Auth obfuscation is not foolproof (determined agent could find it)
- Network isolation doesn't prevent data exfil via API calls
- Container escape vulnerabilities may exist

### Future Improvements

- API bridge removes secrets from sandbox entirely
- Syscall filtering (seccomp)
- Capability dropping
- Read-only workspace mode for review tasks

## Testing Strategy

### Unit Tests

- Template validation
- Port allocation logic
- Wrapper generation

### Integration Tests

- Container lifecycle (up/down/reset)
- SSH connectivity
- Nix builds inside container
- Agent execution

### Manual Testing

- Real AI agent workflows
- Multi-agent scenarios
- Network isolation verification

## Related Projects

- **NixOS Containers**: Built on top of this
- **devenv**: Development environments (different scope)
- **nix-shells**: Per-project environments (not isolated)
- **Docker/Podman**: Alternative container runtimes (not used)

## Glossary

| Term | Definition |
|------|------------|
| **Template** | Declarative specification for a sandbox type |
| **Instance** | Running sandbox created from a template |
| **Agent** | AI coding tool (claude-code, opencode, etc.) |
| **Wrapper** | Generated binary that injects auth and calls agent |
| **Workspace** | Bind-mounted project directory |
| **Ephemeral** | Container root that doesn't persist across restarts |

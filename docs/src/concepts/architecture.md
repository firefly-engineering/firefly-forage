# Architecture

Forage uses NixOS containers (systemd-nspawn) to create isolated environments for AI agents.

## System Overview

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
│  │ agent: claude               │  │ agents: claude, aider │     │
│  │ sshd :22 ──► host:2200      │  │ sshd :22 ──► host:22. │     │
│  └─────────────────────────────┘  └───────────────────────┘     │
│                                                                 │
│  forage-ctl (CLI)                                               │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## Components

### Host Module

The NixOS module (`services.firefly-forage`) configures:

- Template definitions
- Secret paths
- Port ranges
- User identity mapping
- System directories via tmpfiles

### forage-ctl

The CLI tool that:

- Creates/destroys containers using extra-container
- Manages SSH port allocation
- Handles JJ workspace lifecycle
- Injects skill files

### extra-container

[extra-container](https://github.com/erikarvstedt/extra-container) manages the systemd-nspawn containers. It allows creating NixOS containers without modifying the host's `/etc/nixos` configuration.

### Containers

Each sandbox is a systemd-nspawn container with:

- **Ephemeral root**: tmpfs filesystem, lost on restart
- **Private network**: Virtual ethernet with NAT to host
- **Bind mounts**: Nix store, workspace, secrets
- **SSH server**: For external access
- **Tmux session**: For session persistence

## Data Flow

### Container Creation

```
forage-ctl up myproject -t claude -w ~/project
       │
       ├─► Find available port (2200-2299)
       ├─► Find available network slot (192.168.100.x)
       ├─► Copy secrets to /run/forage-secrets/myproject/
       ├─► Inject skills to ~/project/.claude/forage-skills.md
       ├─► Generate container Nix configuration
       ├─► Call extra-container create --start
       └─► Wait for SSH to become available
```

### Container Configuration

The generated Nix configuration includes:

```nix
containers."forage-myproject" = {
  ephemeral = true;
  privateNetwork = true;
  hostAddress = "192.168.100.1";
  localAddress = "192.168.100.11";

  forwardPorts = [{
    containerPort = 22;
    hostPort = 2200;
    protocol = "tcp";
  }];

  bindMounts = {
    "/nix/store" = { hostPath = "/nix/store"; isReadOnly = true; };
    "/workspace" = { hostPath = "/home/user/project"; isReadOnly = false; };
    "/run/secrets" = { hostPath = "/run/forage-secrets/myproject"; isReadOnly = true; };
  };

  config = { ... }: {
    # Container NixOS configuration
    services.openssh.enable = true;
    users.users.agent = { ... };
    environment.systemPackages = [ ... ];
  };
};
```

### Network Architecture

```
┌─────────────────────────────────────────────────┐
│ Host                                            │
│                                                 │
│  ┌─────────────┐                                │
│  │ NAT Gateway │ 192.168.100.1                  │
│  └──────┬──────┘                                │
│         │                                       │
│    ┌────┴────┬────────────┐                     │
│    │         │            │                     │
│    ▼         ▼            ▼                     │
│  .11       .12          .13                     │
│ sandbox-a  sandbox-b   sandbox-c                │
│ :2200      :2201       :2202                    │
│                                                 │
└─────────────────────────────────────────────────┘
```

Each sandbox:
- Gets a unique IP in the 192.168.100.0/24 range
- Has SSH port forwarded from host
- Uses host's DNS resolution

## State Management

### Metadata Files

Sandbox metadata is stored in JSON files:

```
/var/lib/firefly-forage/sandboxes/myproject.json
```

```json
{
  "name": "myproject",
  "template": "claude",
  "port": 2200,
  "workspace": "/home/user/project",
  "networkSlot": 1,
  "createdAt": "2024-01-15T10:30:00+00:00",
  "workspaceMode": "direct"
}
```

For JJ workspaces, additional fields:

```json
{
  "workspaceMode": "jj",
  "sourceRepo": "/home/user/repos/myrepo",
  "jjWorkspaceName": "myproject"
}
```

### Directories

| Path | Purpose |
|------|---------|
| `/etc/firefly-forage/` | Configuration and templates |
| `/var/lib/firefly-forage/sandboxes/` | Sandbox metadata |
| `/var/lib/firefly-forage/workspaces/` | JJ workspace directories |
| `/run/forage-secrets/` | Runtime secrets (tmpfs) |

## Security Boundaries

```
┌─────────────────────────────────────────────────────────────────┐
│ Trusted Zone (Host)                                             │
│                                                                 │
│  - NixOS configuration                                          │
│  - Nix daemon                                                   │
│  - Secret files                                                 │
│  - forage-ctl                                                   │
│                                                                 │
├─────────────────────────────────────────────────────────────────┤
│ Isolation Boundary (systemd-nspawn)                             │
├─────────────────────────────────────────────────────────────────┤
│ Untrusted Zone (Container)                                      │
│                                                                 │
│  - AI agent code                                                │
│  - User workspace (read-write)                                  │
│  - Agent-installed packages                                     │
│                                                                 │
│  Limited access to:                                             │
│  - /nix/store (read-only)                                       │
│  - /run/secrets (read-only)                                     │
│  - Network (configurable)                                       │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

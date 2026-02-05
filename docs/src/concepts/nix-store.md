# Nix Store Sharing

Forage sandboxes share the host's nix store, avoiding duplication while maintaining isolation.

## How It Works

The nix store is bind-mounted read-only into each container:

```nix
bindMounts = {
  "/nix/store" = {
    hostPath = "/nix/store";
    isReadOnly = true;
  };
};
```

When an agent needs to install packages, they go through the host's nix daemon:

```
┌─────────────────────────────────────────────────────────────────┐
│ Host                                                            │
│                                                                 │
│  nix-daemon ◄──────────────────────────────┐                    │
│       │                                    │                    │
│       ▼                                    │                    │
│  /nix/store ◄──────────────────────────────┼───────────┐        │
│  (writable by daemon)                      │           │        │
│                                            │           │        │
│  ┌─────────────────────────────┐  ┌────────┴───────────┴──┐     │
│  │ sandbox-a                   │  │ sandbox-b             │     │
│  │                             │  │                       │     │
│  │ /nix/store (read-only)      │  │ /nix/store (read-only)│     │
│  │                             │  │                       │     │
│  │ $ nix run nixpkgs#ripgrep   │  │ $ nix shell nixpkgs#jq│     │
│  │       │                     │  │       │               │     │
│  │       └─────────────────────┼──┼───────┘               │     │
│  │                             │  │                       │     │
│  └─────────────────────────────┘  └───────────────────────┘     │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## Why This Works

1. **Read-only detection**: When `/nix/store` is read-only, the nix client detects it can't write directly

2. **Daemon mode**: The client automatically switches to daemon mode and communicates via socket

3. **Host builds**: The nix daemon on the host performs the actual builds and writes to the store

4. **Instant visibility**: Since the container bind-mounts the same store, new paths are immediately visible

5. **Content-addressed**: Nix's content-addressed store means there are no conflicts—the same input always produces the same output path

## Benefits

### No Duplication

Without store sharing, each container would need its own copy of:
- Base system packages
- Development tools
- Agent binaries

With sharing, the store is used efficiently:

```
Without sharing:
  Container A: /nix/store/...-ripgrep-14.0.0  (15MB)
  Container B: /nix/store/...-ripgrep-14.0.0  (15MB)
  Container C: /nix/store/...-ripgrep-14.0.0  (15MB)
  Total: 45MB

With sharing:
  Host: /nix/store/...-ripgrep-14.0.0  (15MB)
  Container A, B, C: bind mount (0MB additional)
  Total: 15MB
```

### Instant Availability

Packages already in the host store are immediately available:

```bash
# Inside container - if ripgrep is already on host
$ nix run nixpkgs#ripgrep -- --version
ripgrep 14.0.0
# (instant, no download/build)
```

### Shared Build Cache

If one container builds a package, others can use it:

```bash
# Container A builds a package
$ nix build nixpkgs#somePackage

# Container B can use it immediately (same store path)
$ nix run nixpkgs#somePackage
# (no rebuild needed)
```

## Using Nix in Sandboxes

### One-Off Commands

```bash
# Run a tool without installing
nix run nixpkgs#ripgrep -- --help
nix run nixpkgs#jq -- '.foo' data.json
```

### Interactive Shell

```bash
# Enter a shell with multiple tools
nix shell nixpkgs#nodejs nixpkgs#yarn nixpkgs#typescript

# Now node, yarn, tsc are available
node --version
```

### Building Projects

```bash
# Build a flake-based project
cd /workspace
nix build

# Run the result
./result/bin/myapp
```

### Development Shells

```bash
# Enter a project's dev shell
cd /workspace
nix develop

# Or with direnv (if project has .envrc)
direnv allow
```

## Limitations

### No Direct Store Writes

Containers cannot write directly to `/nix/store`:

```bash
# This won't work
$ nix-store --add myfile
error: cannot open `/nix/store/.../myfile' for writing: Read-only file system
```

All writes must go through the daemon.

### Daemon Socket Required

The nix daemon socket must be accessible. This is handled by systemd-nspawn's socket activation.

### Store Garbage Collection

Garbage collection happens on the host. If the host runs `nix-collect-garbage`, it may remove paths that containers are using.

Best practice: Don't run aggressive garbage collection while sandboxes are active.

## Future: Registry Pinning

To ensure consistency across all `nix run nixpkgs#foo` commands, a future enhancement will inject a pinned nix registry:

```nix
# In container config
environment.etc."nix/registry.json".text = builtins.toJSON {
  version = 2;
  flakes = [{
    from = { type = "indirect"; id = "nixpkgs"; };
    to = {
      type = "github";
      owner = "NixOS";
      repo = "nixpkgs";
      rev = "abc123...";  # Pinned to host's nixpkgs
    };
  }];
};
```

This will ensure:
- All sandboxes use the same nixpkgs version
- No accumulation of different nixpkgs versions in the store
- Reproducible tool installations across sandboxes

# Workspace Mounts

Forage supports composable workspace mounts, allowing you to assemble a sandbox's filesystem from multiple sources. Instead of a single `--repo` mapped to `/workspace`, you can mount multiple repositories, overlay branches, and mix VCS-backed and literal bind mounts.

## Overview

The traditional single-workspace model mounts one directory at `/workspace`:

```bash
forage-ctl up myproject -t claude --repo ~/projects/myrepo
```

With composable mounts, a template can declare multiple mount points:

```
/workspace          ← jj workspace from ~/projects/myrepo
/workspace/.beads   ← jj workspace (branch beads-sync) from same repo
/workspace/data     ← direct bind mount from ~/datasets
```

No mount is special-cased as "root" — you could have `/workspace/proj1` and `/workspace/proj2` with nothing at `/workspace` itself.

## Configuring Mounts in Templates

Mounts are declared in your NixOS configuration under `workspace.mounts`. Each mount is keyed by a stable name:

```nix
services.firefly-forage.templates.my-template = {
  agents.claude = { ... };

  workspace.mounts = {
    main = {
      containerPath = "/workspace";
      mode = "jj";
      # repo = null → uses default --repo from CLI
    };

    data = {
      containerPath = "/workspace/data";
      repo = "data";  # references --repo data=<path>
      mode = "git-worktree";
    };

    config = {
      containerPath = "/workspace/.config";
      hostPath = "~/shared-config";  # literal bind mount
      readOnly = true;
    };
  };
};
```

### Mount Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `containerPath` | string | (required) | Mount point inside the container |
| `hostPath` | string or null | `null` | Literal host path for bind mount. Mutually exclusive with `repo`. |
| `repo` | string or null | `null` | Repo reference (see [Repo Resolution](#repo-resolution)) |
| `mode` | `"jj"`, `"git-worktree"`, `"direct"`, or null | `null` (auto-detect) | VCS mode for repo-backed mounts |
| `branch` | string or null | `null` | Branch/ref to check out (VCS mounts only) |
| `readOnly` | bool | `false` | Mount as read-only |

### Repo Resolution

The `repo` field controls where a mount's source comes from:

| Value | Behavior |
|-------|----------|
| `null` or `""` | Uses the default (unnamed) `--repo` value from CLI |
| `"<name>"` | Looks up the named repo from `--repo <name>=<path>` |
| `"/absolute/path"` | Literal path, no CLI lookup needed |

When a mount specifies `hostPath` instead of `repo`, it becomes a direct bind mount — no VCS workspace is created.

## Named Repo Parameters

The `--repo` flag supports both unnamed (default) and named parameters:

```bash
# Default repo (used by mounts with repo = null)
forage-ctl up mysandbox -t my-template --repo ~/projects/myrepo

# Default repo + named repo
forage-ctl up mysandbox -t my-template \
  --repo ~/projects/myrepo \
  --repo data=~/datasets/my-data

# Multiple named repos (no default)
forage-ctl up mysandbox -t my-template \
  --repo main=~/projects/myrepo \
  --repo data=~/datasets/my-data
```

The `--repo` flag is repeatable. Values containing `=` are parsed as `name=path`; values without `=` set the default repo.

### When `--repo` Is Optional

If every mount in the template specifies either `hostPath` or an absolute `repo` path, the `--repo` flag is not required:

```nix
workspace.mounts = {
  project = {
    containerPath = "/workspace";
    repo = "/home/user/projects/myrepo";  # absolute path
  };
  config = {
    containerPath = "/workspace/.config";
    hostPath = "/etc/shared-config";  # literal bind mount
  };
};
```

```bash
# No --repo needed
forage-ctl up mysandbox -t self-contained
```

## Backward Compatibility

Templates without `workspace.mounts` behave exactly as before — `--repo` creates a single auto-detected mount at the configured workspace path. All existing workflows continue to work unchanged.

```bash
# This still works identically to before
forage-ctl up myproject -t claude --repo ~/projects/myrepo
forage-ctl up myproject -t claude --repo ~/projects/myrepo --direct
```

## VCS Mode Behavior

Each repo-backed mount gets its own VCS workspace:

| Mode | What Happens |
|------|-------------|
| `jj` | Creates a JJ workspace at the managed path. If `branch` is set, checks out that branch. |
| `git-worktree` | Creates a git worktree with branch `forage-<sandbox>-<mount>`. |
| `direct` | Bind mounts the repo path directly (no workspace isolation). |
| `null` (auto-detect) | Detects `.jj/` → jj, `.git/` → git-worktree, otherwise → direct. |

Managed workspace directories are created under `/var/lib/firefly-forage/workspaces/<sandbox>/<mount-name>/`, one subdirectory per VCS-backed mount.

## `useBeads` Convenience Option

The `workspace.useBeads` option provides a shorthand for a common pattern — overlaying a beads workspace:

```nix
services.firefly-forage.templates.with-beads = {
  agents.claude = { ... };

  workspace.mounts.main = {
    containerPath = "/workspace";
    mode = "jj";
  };

  workspace.useBeads = {
    enable = true;
    branch = "beads-sync";        # default
    containerPath = "/workspace/.beads";  # default
    package = pkgs.beads;         # added to extraPackages
    # repo = null;                # null → inherits default --repo
  };
};
```

When `useBeads.enable = true`, the Nix module automatically:

1. Injects a mount named `beads` into `workspace.mounts` (jj mode, specified branch, at `containerPath`)
2. Adds the `package` to `extraPackages` (if set)

### `useBeads` Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enable` | bool | `false` | Enable the beads workspace overlay |
| `branch` | string | `"beads-sync"` | Branch to check out in the beads workspace |
| `package` | package or null | `null` | Beads package to install in the sandbox |
| `containerPath` | string | `"/workspace/.beads"` | Mount point inside the container |
| `repo` | string or null | `null` | Repo reference (`null` → inherit default `--repo`) |

## Examples

### Single Repo with Beads Overlay

The most common multi-mount pattern — a primary workspace with a beads branch overlaid:

```nix
templates.claude-beads = {
  description = "Claude with beads";

  agents.claude = {
    package = pkgs.claude-code;
    secretName = "anthropic";
    authEnvVar = "ANTHROPIC_API_KEY";
  };

  workspace.mounts.main = {
    containerPath = "/workspace";
    mode = "jj";
  };

  workspace.useBeads = {
    enable = true;
    package = pkgs.beads;
  };

  extraPackages = with pkgs; [ ripgrep fd jq ];
};
```

```bash
forage-ctl up agent-a -t claude-beads --repo ~/projects/myrepo
```

Inside the sandbox:
```
/workspace/           ← jj workspace (main working copy)
/workspace/.beads/    ← jj workspace (beads-sync branch)
```

### Monorepo with Multiple Services

Mount different parts of a monorepo at different paths:

```nix
templates.monorepo = {
  description = "Multi-service development";

  agents.claude = { ... };

  workspace.mounts = {
    frontend = {
      containerPath = "/workspace/frontend";
      repo = "frontend";
      mode = "jj";
    };
    backend = {
      containerPath = "/workspace/backend";
      repo = "backend";
      mode = "jj";
    };
    shared = {
      containerPath = "/workspace/shared";
      hostPath = "~/projects/shared-libs";
      readOnly = true;
    };
  };
};
```

```bash
forage-ctl up dev -t monorepo \
  --repo frontend=~/projects/frontend \
  --repo backend=~/projects/backend
```

### Read-Only Reference Mount

Mount documentation or reference data alongside the workspace:

```nix
templates.with-docs = {
  agents.claude = { ... };

  workspace.mounts = {
    main = {
      containerPath = "/workspace";
      mode = "jj";
    };
    docs = {
      containerPath = "/workspace/reference";
      hostPath = "~/docs/api-reference";
      readOnly = true;
    };
  };
};
```

## Mount Validation

Before creating any VCS workspaces, Forage validates the mount configuration:

- **Duplicate container paths**: Two mounts claiming the same path is an error
- **Repo resolution**: A mount referencing a named repo not provided via `--repo` is an error
- **Source existence**: `hostPath` that doesn't exist or `repo` path that isn't a valid directory is an error
- **Rollback on failure**: If creating a VCS workspace fails partway through, all previously-created workspaces for that sandbox are rolled back

## Cleanup

When you remove a sandbox with `forage-ctl down`, each mount is cleaned up individually:

- **VCS-backed mounts** (jj, git-worktree): The workspace/worktree is removed via the appropriate VCS command
- **Literal bind mounts** (`hostPath`): No cleanup needed — the host directory is left untouched
- **Managed directories**: The subdirectory under `/var/lib/firefly-forage/workspaces/<sandbox>/` is removed

## Skill Injection with Multiple Mounts

When a sandbox has multiple mounts, the injected skill file describes the composite layout:

```markdown
## Workspace Layout

Your workspace contains multiple mount sources:
- /workspace: jj workspace from ~/projects/myrepo
- /workspace/.beads: jj workspace (branch beads-sync) from ~/projects/myrepo
- /workspace/data: direct mount from ~/datasets (read-only)
```

This gives the agent full context about what's mounted where and how each mount is managed.

## Metadata

Multi-mount sandboxes store mount information in their metadata:

```json
{
  "name": "myproject",
  "template": "claude-beads",
  "workspaceMounts": [
    {
      "name": "main",
      "containerPath": "/workspace",
      "hostPath": "/var/lib/firefly-forage/workspaces/myproject/main",
      "sourceRepo": "/home/user/projects/myrepo",
      "mode": "jj"
    },
    {
      "name": "beads",
      "containerPath": "/workspace/.beads",
      "hostPath": "/var/lib/firefly-forage/workspaces/myproject/beads",
      "sourceRepo": "/home/user/projects/myrepo",
      "mode": "jj",
      "branch": "beads-sync"
    }
  ]
}
```

Legacy single-workspace fields (`workspace`, `workspaceMode`, `sourceRepo`) are still populated for backward compatibility with older tooling.

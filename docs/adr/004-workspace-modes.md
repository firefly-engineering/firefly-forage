# ADR 004: Workspace Modes (Direct, JJ, Git Worktree)

## Status

Accepted

## Context

Sandboxes need access to project files. The workspace configuration determines:
1. Where files come from (bind mount, workspace copy)
2. How changes are isolated between sandboxes
3. How version control works inside the sandbox

A key use case is running multiple AI agents on the same repository simultaneously without conflicts.

## Decision

Support three workspace modes:

### 1. Direct Mode (`--workspace`)

Bind-mount an existing directory directly into the sandbox.

```bash
forage-ctl up mybox --template claude --workspace ~/projects/myrepo
```

- Simple and straightforward
- No isolation between sandboxes using the same directory
- Best for single-agent workflows

### 2. JJ Mode (`--repo` with jj repository)

Create an isolated jj workspace from a repository.

```bash
forage-ctl up agent-a --template claude --repo ~/projects/myrepo
forage-ctl up agent-b --template claude --repo ~/projects/myrepo
```

- Each sandbox gets its own working copy
- Changes don't affect other sandboxes until committed
- Shared operation log enables cross-sandbox visibility
- Workspace created at `/var/lib/forage/workspaces/<name>`

### 3. Git Worktree Mode (`--git-worktree`)

Create an isolated git worktree with a dedicated branch.

```bash
forage-ctl up agent-a --template claude --git-worktree ~/projects/myrepo
```

- Each sandbox gets a separate branch (`forage-<name>`)
- Worktree created at `/var/lib/forage/workspaces/<name>`
- Works with plain git repositories

### Implementation

The `workspace` package defines a `Backend` interface:

```go
type Backend interface {
    Name() string
    IsRepo(path string) bool
    Exists(repoPath, name string) bool
    Create(repoPath, name, workspacePath string) error
    Remove(repoPath, name, workspacePath string) error
}
```

With implementations:
- `JJBackend`: Manages jj workspaces
- `GitBackend`: Manages git worktrees

## Consequences

### Positive

- **Parallel work**: Multiple agents can work on the same repo simultaneously
- **Isolation**: Changes in one sandbox don't affect others
- **Flexibility**: Users choose the mode that fits their workflow
- **Cleanup**: Workspaces are properly removed on sandbox destruction

### Negative

- **Complexity**: Three modes to understand and maintain
- **Disk usage**: JJ/git modes create file copies
- **VCS dependency**: JJ/git must be installed and functional
- **Bind mount quirks**: JJ mode requires special handling of `.jj` directory

## Alternatives Considered

### 1. Copy-on-write filesystem

Use a COW filesystem (btrfs subvolumes, overlayfs) for isolation.

**Rejected because**: Adds filesystem requirements, complex to set up, doesn't integrate with version control.

### 2. JJ only

Only support jj workspaces, require jj for all isolated workflows.

**Rejected because**: Many users still use git. Forcing jj adoption is a barrier.

### 3. No isolation

Only support direct bind mounts, let users manage isolation.

**Rejected because**: Defeats a key use case (parallel agents on same repo).

### 4. rsync-based copies

Copy workspace files to a new directory.

**Rejected because**: Loses version control integration, hard to sync changes back, inefficient for large repos.

## Technical Notes

### JJ Mode Bind Mounts

JJ workspaces use a symlink in `.jj/repo` that points to the source repo's `.jj` directory. For this to work inside the container, we bind-mount the source repo's `.jj` at its original host path:

```nix
bindMounts = {
  "/workspace" = { hostPath = "/var/lib/forage/workspaces/agent-a"; };
  "/home/user/projects/myrepo/.jj" = { hostPath = "/home/user/projects/myrepo/.jj"; };
};
```

### Git Worktree Branches

Git worktrees require a unique branch per worktree. We use the pattern `forage-<sandbox-name>`:

```bash
git worktree add /var/lib/forage/workspaces/agent-a -b forage-agent-a HEAD
```

On cleanup:
```bash
git worktree remove /var/lib/forage/workspaces/agent-a
git branch -d forage-agent-a
```

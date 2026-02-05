# ADR 001: Container Runtime Abstraction

## Status

Accepted

## Context

Firefly Forage creates isolated sandbox environments for AI coding agents. Initially, the implementation was tightly coupled to NixOS containers (systemd-nspawn). However, several requirements emerged:

1. **Cross-platform support**: Users on macOS need sandboxes too
2. **Testing**: Integration tests need mock runtimes
3. **Flexibility**: Different deployment environments may prefer different backends
4. **Future-proofing**: New container technologies may emerge

## Decision

Introduce a `Runtime` interface that abstracts container operations:

```go
type Runtime interface {
    Name() string
    Create(ctx context.Context, opts CreateOptions) error
    Start(ctx context.Context, name string) error
    Stop(ctx context.Context, name string) error
    Destroy(ctx context.Context, name string) error
    IsRunning(ctx context.Context, name string) (bool, error)
    Status(ctx context.Context, name string) (*ContainerInfo, error)
    Exec(ctx context.Context, name string, command []string, opts ExecOptions) (*ExecResult, error)
    ExecInteractive(ctx context.Context, name string, command []string, opts ExecOptions) error
    List(ctx context.Context) ([]*ContainerInfo, error)
}
```

Implementations:
- `NspawnRuntime`: NixOS containers via extra-container (Linux)
- `DockerRuntime`: Docker/Podman containers (universal fallback)
- `AppleRuntime`: Apple Container framework (macOS 13+)
- `MockRuntime`: For testing

Runtime selection is automatic via `Global()` which detects the platform and available tools.

## Consequences

### Positive

- **Portability**: Same CLI works across platforms with different backends
- **Testability**: MockRuntime enables comprehensive unit testing
- **Extensibility**: New backends can be added without changing command code
- **Separation of concerns**: Commands focus on business logic, not container details

### Negative

- **Abstraction overhead**: Some runtime-specific features may not fit the interface
- **Complexity**: More code to maintain across multiple backends
- **Lowest common denominator**: Interface limited to features all backends support

## Alternatives Considered

### 1. Platform-specific CLIs

Build separate `forage-ctl-nix`, `forage-ctl-macos`, etc.

**Rejected because**: Significant code duplication, harder to maintain, confusing for users.

### 2. Docker-only

Use Docker everywhere, including on NixOS.

**Rejected because**: Loses NixOS integration benefits (nix-daemon socket sharing, ephemeral roots), adds Docker dependency on NixOS systems that don't need it.

### 3. No abstraction (nspawn only)

Keep tight coupling to systemd-nspawn, add other platforms later.

**Rejected because**: Would require significant refactoring later, harder to test now.

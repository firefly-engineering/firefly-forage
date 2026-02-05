# Technical Debt Remediation Plan

This document outlines technical debt, architecture issues, and maintainability concerns identified in the Firefly Forage codebase, along with a phased remediation plan.

## Executive Summary

**Total Issues Identified:** 46
- Go codebase: 34 issues (13 High, 17 Medium, 4 Low)
- NixOS modules: 12 issues (4 High, 5 Medium, 3 Low)

**Primary Concerns:**
1. **Testability** - Heavy reliance on global state and direct OS calls makes unit testing difficult
2. **Code duplication** - SSH argument construction repeated in 7+ locations
3. **Error handling** - Inconsistent patterns, silent failures in cleanup operations
4. **Configuration validation** - Missing validation for both Go configs and NixOS options
5. **Hardcoded values** - Network interfaces, IP ranges, paths scattered throughout

---

## Phase 1: Foundation (Critical Path)

These changes unblock other improvements and prevent production issues.

### 1.1 Create Abstractions for Testability

**Priority:** High | **Effort:** Large | **Risk:** Low

Create interfaces that enable mocking for tests:

```go
// internal/system/interfaces.go
type FileSystem interface {
    ReadFile(path string) ([]byte, error)
    WriteFile(path string, data []byte, perm os.FileMode) error
    Remove(path string) error
    RemoveAll(path string) error
    Stat(path string) (os.FileInfo, error)
    MkdirAll(path string, perm os.FileMode) error
}

type CommandExecutor interface {
    Execute(ctx context.Context, name string, args ...string) ([]byte, error)
    ExecuteInteractive(ctx context.Context, name string, args ...string) error
}
```

**Files to create:**
- `internal/system/interfaces.go` - Interface definitions
- `internal/system/os.go` - Real OS implementation
- `internal/system/mock.go` - Mock implementation for tests

**Files to modify:**
- `internal/config/config.go` - Accept FileSystem interface
- `internal/ssh/ssh.go` - Accept CommandExecutor interface
- `internal/health/health.go` - Accept CommandExecutor interface

### 1.2 Consolidate SSH Command Building

**Priority:** High | **Effort:** Medium | **Risk:** Low

SSH arguments are duplicated in 7+ locations. Create a single builder:

```go
// internal/ssh/builder.go
type SSHOptions struct {
    Port              int
    User              string
    Host              string
    StrictHostKeyCheck bool
    KnownHostsFile    string
    ConnectTimeout    int
    RequestTTY        bool
}

func DefaultOptions(port int) SSHOptions {
    return SSHOptions{
        Port:              port,
        User:              "agent",
        Host:              "localhost",
        StrictHostKeyCheck: false,
        KnownHostsFile:    "/dev/null",
        ConnectTimeout:    5,
    }
}

func (o SSHOptions) BuildArgs() []string { ... }
func (o SSHOptions) BuildExecArgs(command ...string) []string { ... }
func (o SSHOptions) BuildInteractiveArgs() []string { ... }
```

**Files to modify:**
- `internal/ssh/ssh.go` - Use builder internally
- `cmd/ssh.go` - Use `ssh.DefaultOptions(port).BuildInteractiveArgs()`
- `cmd/exec.go` - Use `ssh.DefaultOptions(port).BuildExecArgs(cmd...)`
- `internal/runtime/nspawn.go` - Use builder

### 1.3 Fix Cleanup Error Handling

**Priority:** High | **Effort:** Small | **Risk:** Medium

Cleanup operations silently ignore errors, potentially leaving orphaned state.

**File:** `cmd/up.go` (lines 329-351)

```go
// Before (errors ignored):
os.RemoveAll(secretsPath)
os.Remove(configPath)

// After (errors logged):
func cleanup(ctx cleanupContext) error {
    var errs []error

    if err := os.RemoveAll(ctx.secretsPath); err != nil {
        logging.Warn("failed to remove secrets", "path", ctx.secretsPath, "error", err)
        errs = append(errs, err)
    }

    if err := os.Remove(ctx.configPath); err != nil && !os.IsNotExist(err) {
        logging.Warn("failed to remove config", "path", ctx.configPath, "error", err)
        errs = append(errs, err)
    }

    // ... other cleanup

    if len(errs) > 0 {
        return fmt.Errorf("cleanup completed with %d errors", len(errs))
    }
    return nil
}
```

**Files to modify:**
- `cmd/up.go` - Fix cleanup function
- `cmd/down.go` - Fix removal operations (lines 68, 72, 80)

### 1.4 Add Configuration Validation

**Priority:** High | **Effort:** Medium | **Risk:** Low

**Go side - `internal/config/config.go`:**

```go
func (c *HostConfig) Validate() error {
    if c.User == "" {
        return errors.New("HostConfig.User is required")
    }
    if c.PortRange.From == 0 || c.PortRange.To == 0 {
        return errors.New("HostConfig.PortRange is required")
    }
    if c.PortRange.From > c.PortRange.To {
        return fmt.Errorf("invalid port range: from (%d) > to (%d)",
            c.PortRange.From, c.PortRange.To)
    }
    return nil
}

func LoadHostConfig(path string) (*HostConfig, error) {
    // ... existing loading code ...
    if err := config.Validate(); err != nil {
        return nil, fmt.Errorf("invalid host config: %w", err)
    }
    return &config, nil
}
```

**NixOS side - `modules/host.nix`:**

```nix
config = mkIf cfg.enable {
    assertions = [
      {
        assertion = cfg.portRange.from <= cfg.portRange.to;
        message = "firefly-forage: portRange.from must be <= portRange.to";
      }
      {
        assertion = cfg.user != "";
        message = "firefly-forage: user must be specified";
      }
      {
        assertion = config.users.users ? ${cfg.user};
        message = "firefly-forage: user '${cfg.user}' does not exist";
      }
    ];
    # ... rest of config
};
```

---

## Phase 2: Architecture Improvements

### 2.1 Extract Business Logic from Commands

**Priority:** High | **Effort:** Large | **Risk:** Medium

The `runUp` function is 240+ lines mixing validation, configuration, container creation, and health checks.

**Proposed structure:**

```
internal/
├── sandbox/
│   ├── create.go      # Sandbox creation orchestration
│   ├── workspace.go   # Workspace setup (dir/jj/git-worktree)
│   ├── secrets.go     # Secret file management
│   └── health.go      # Health check coordination
```

**Example refactor for `cmd/up.go`:**

```go
func runUp(cmd *cobra.Command, args []string) error {
    ctx := cmd.Context()

    // Parse and validate input
    opts, err := parseUpOptions(cmd, args)
    if err != nil {
        return err
    }

    // Create sandbox using business logic package
    creator := sandbox.NewCreator(
        sandbox.WithRuntime(runtime.Global()),
        sandbox.WithFS(system.DefaultFS()),
    )

    result, err := creator.Create(ctx, opts)
    if err != nil {
        return err
    }

    // Output results
    logSuccess("Sandbox '%s' created successfully", result.Name)
    logInfo("Connect with: forage-ctl ssh %s", result.Name)
    return nil
}
```

### 2.2 Make NixOS Network Configuration Flexible ✅

**Priority:** High | **Effort:** Small | **Risk:** Low | **Status:** Complete

**File:** `modules/host.nix`

```nix
options.services.firefly-forage = {
    # Add new options
    externalInterface = mkOption {
      type = types.nullOr types.str;
      default = null;
      description = ''
        External network interface for NAT. If null, NAT configuration
        is skipped (useful when using an existing NAT setup).
      '';
      example = "eth0";
    };

    containerSubnet = mkOption {
      type = types.str;
      default = "192.168.100.0/24";
      description = "Subnet for container networking";
    };
};

config = mkIf cfg.enable {
    networking.nat = mkIf (cfg.externalInterface != null) {
      enable = true;
      internalInterfaces = [ "ve-+" ];
      externalInterface = cfg.externalInterface;
    };
};
```

### 2.3 Unify Runtime Interface for SSH

**Priority:** Medium | **Effort:** Medium | **Risk:** Medium

The `SSHRuntime` interface is implemented inconsistently. Nspawn returns errors for `SSHPort()`.

**Option A: Remove SSHRuntime from nspawn**
```go
// runtime/nspawn.go
// Remove SSHPort, SSHExec, SSHExecWithOutput methods
// Callers use metadata.Port directly
```

**Option B: Store port in runtime (preferred)**
```go
// runtime/interface.go
type CreateOptions struct {
    // ... existing fields
    SSHPort int  // Add this
}

// runtime/nspawn.go
type NspawnRuntime struct {
    // ... existing fields
    sandboxPorts map[string]int  // Track ports per sandbox
}

func (r *NspawnRuntime) Create(ctx context.Context, opts CreateOptions) error {
    // ... existing logic
    r.sandboxPorts[opts.Name] = opts.SSHPort
    return nil
}

func (r *NspawnRuntime) SSHPort(ctx context.Context, name string) (int, error) {
    port, ok := r.sandboxPorts[name]
    if !ok {
        return 0, fmt.Errorf("unknown sandbox: %s", name)
    }
    return port, nil
}
```

---

## Phase 3: Code Quality

### 3.1 Define Constants for Magic Values

**Priority:** Medium | **Effort:** Small | **Risk:** Low

**Create `internal/constants/constants.go`:**

```go
package constants

const (
    // SSH configuration
    DefaultSSHUser           = "agent"
    SSHReadyTimeoutSeconds   = 30
    SSHConnectTimeoutSeconds = 5

    // Network configuration
    NetworkSlotMin = 1
    NetworkSlotMax = 254  // 255 reserved, 0 invalid

    // Container configuration
    ContainerPrefix    = "forage-"
    NixOSStateVersion  = "24.05"

    // Tmux
    TmuxSessionName = "forage"
)
```

### 3.2 Standardize Error Handling

**Priority:** Medium | **Effort:** Medium | **Risk:** Low

**Adopt consistent error wrapping:**

```go
// internal/errors/errors.go
package errors

import "fmt"

type ConfigError struct {
    Op      string
    Path    string
    Wrapped error
}

func (e *ConfigError) Error() string {
    return fmt.Sprintf("config %s %s: %v", e.Op, e.Path, e.Wrapped)
}

func (e *ConfigError) Unwrap() error { return e.Wrapped }

func LoadConfig(path string, err error) error {
    return &ConfigError{Op: "load", Path: path, Wrapped: err}
}

func ParseConfig(path string, err error) error {
    return &ConfigError{Op: "parse", Path: path, Wrapped: err}
}
```

### 3.3 Consolidate Logging

**Priority:** Low | **Effort:** Small | **Risk:** Low

**Unify user-facing and debug logging:**

```go
// internal/logging/user.go
package logging

import (
    "fmt"
    "os"
)

// User-facing output (stdout, with emoji)
func UserInfo(format string, args ...interface{}) {
    fmt.Fprintf(os.Stdout, "ℹ "+format+"\n", args...)
}

func UserSuccess(format string, args ...interface{}) {
    fmt.Fprintf(os.Stdout, "✓ "+format+"\n", args...)
}

func UserWarning(format string, args ...interface{}) {
    fmt.Fprintf(os.Stderr, "⚠ "+format+"\n", args...)
}

func UserError(format string, args ...interface{}) {
    fmt.Fprintf(os.Stderr, "✗ "+format+"\n", args...)
}
```

Then update `cmd/root.go` to use `logging.User*` instead of local functions.

---

## Phase 4: Testing Infrastructure

### 4.1 Add Test Fixtures

**Priority:** Medium | **Effort:** Medium | **Risk:** Low

```
internal/
├── testutil/
│   ├── fixtures/
│   │   ├── valid_host_config.json
│   │   ├── invalid_host_config.json
│   │   ├── valid_sandbox_metadata.json
│   │   └── valid_template.json
│   ├── mock_fs.go
│   ├── mock_executor.go
│   └── assertions.go
```

### 4.2 Add Unit Tests for Critical Paths

**Priority:** High | **Effort:** Large | **Risk:** Low

**Target coverage:**
- `internal/config/` - Config loading and validation
- `internal/ssh/` - SSH command building
- `internal/port/` - Port allocation
- `internal/health/` - Health check logic
- `internal/skills/` - Skill file generation

**Example test for SSH builder:**

```go
// internal/ssh/builder_test.go
func TestSSHOptionsDefaultArgs(t *testing.T) {
    opts := DefaultOptions(2200)
    args := opts.BuildArgs()

    assert.Contains(t, args, "-p", "2200")
    assert.Contains(t, args, "-o", "StrictHostKeyChecking=no")
    assert.Contains(t, args, "agent@localhost")
}

func TestSSHOptionsExecArgs(t *testing.T) {
    opts := DefaultOptions(2200)
    args := opts.BuildExecArgs("ls", "-la")

    // Should end with command
    assert.Equal(t, "ls", args[len(args)-2])
    assert.Equal(t, "-la", args[len(args)-1])
}
```

### 4.3 Add Integration Test Framework

**Priority:** Medium | **Effort:** Large | **Risk:** Low

Create a test harness that can spin up actual containers (in CI with nspawn support):

```go
// internal/integration/harness.go
type TestHarness struct {
    tempDir string
    runtime runtime.Runtime
}

func NewHarness(t *testing.T) *TestHarness {
    // Skip if not running integration tests
    if os.Getenv("FORAGE_INTEGRATION_TESTS") == "" {
        t.Skip("integration tests disabled")
    }
    // ...
}

func (h *TestHarness) CreateSandbox(name string, opts ...Option) error { ... }
func (h *TestHarness) Cleanup() { ... }
```

---

## Phase 5: Documentation & Polish

### 5.1 Add Package Documentation

**Priority:** Low | **Effort:** Small | **Risk:** None

Add `doc.go` to each package:

```go
// internal/runtime/doc.go

// Package runtime provides a unified interface for container runtimes.
//
// Supported runtimes:
//   - nspawn: NixOS containers via extra-container (Linux)
//   - docker: Docker containers (Linux, macOS, Windows)
//   - podman: Podman containers (Linux, macOS)
//   - apple: Apple Container (macOS 13+)
//
// Runtime selection is automatic based on platform and available tools.
// Use Global() to get the detected runtime, or construct specific
// implementations directly for testing.
package runtime
```

### 5.2 Add Architecture Decision Records

**Priority:** Low | **Effort:** Small | **Risk:** None

Create `docs/adr/` directory with key decisions:
- `001-container-runtime-abstraction.md`
- `002-ssh-based-sandbox-access.md`
- `003-skill-injection-strategy.md`
- `004-workspace-modes.md`

---

## Implementation Schedule

| Phase | Duration | Dependencies |
|-------|----------|--------------|
| Phase 1.1 (Abstractions) | 2-3 days | None |
| Phase 1.2 (SSH Builder) | 1 day | None |
| Phase 1.3 (Cleanup Errors) | 0.5 day | None |
| Phase 1.4 (Validation) | 1 day | None |
| Phase 2.1 (Extract Logic) | 3-4 days | Phase 1.1 |
| Phase 2.2 (NixOS Network) | 0.5 day | None |
| Phase 2.3 (Runtime SSH) | 1 day | Phase 1.2 |
| Phase 3 (Code Quality) | 2 days | None |
| Phase 4 (Testing) | 3-4 days | Phases 1-2 |
| Phase 5 (Documentation) | 1-2 days | All |

**Total estimated effort:** 15-20 days

---

## Risk Assessment

| Change | Risk | Mitigation |
|--------|------|------------|
| Abstractions for testability | Low | Backwards compatible, existing behavior unchanged |
| SSH builder consolidation | Low | Pure refactor, behavior unchanged |
| Cleanup error handling | Medium | Test thoroughly, may surface hidden issues |
| Extract business logic | Medium | Incremental extraction, maintain test coverage |
| NixOS network config | Medium | Default to current behavior, opt-in changes |

---

## Success Metrics

1. **Test coverage**: Target 70%+ for `internal/` packages
2. **Duplication**: SSH argument code appears in exactly 1 location
3. **Error visibility**: All cleanup failures are logged
4. **Validation**: Invalid configs fail at load time, not runtime
5. **Documentation**: All public APIs have godoc comments

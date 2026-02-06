# Code Remediation Tracking

This document tracks the implementation of security fixes, test coverage improvements, and architectural enhancements identified during the comprehensive code review.

## Status Legend
- [ ] Not started
- [~] In progress
- [x] Completed

---

## Phase 1: Critical Security Fixes (Immediate Priority)

### P0 - Critical Vulnerabilities

| ID | Task | Status | Notes |
|----|------|--------|-------|
| SEC-001 | Fix shell injection in `copySkillsToContainer` | [x] | Use stdin pipe instead of heredoc to prevent content breakout |
| SEC-002 | Add sandbox name validation with allowlist | [x] | Validate names match `^[a-z0-9][a-z0-9_-]*$` pattern |

### P1 - High Priority Security

| ID | Task | Status | Notes |
|----|------|--------|-------|
| SEC-003 | Validate gateway SSH_ORIGINAL_COMMAND input | [x] | Sanitize before passing to ConnectToSandbox |
| SEC-004 | Add path traversal protection in config loading | [x] | Validate name doesn't contain path separators |
| SEC-005 | Document SSH host key verification decision | [x] | Add ADR explaining the localhost-only trust model |

---

## Phase 2: Test Coverage Improvements

### P1 - Critical Package Tests

| ID | Task | Status | Notes |
|----|------|--------|-------|
| TEST-001 | Create `sandbox/creator_test.go` | [x] | Test full creation workflow with mocks |
| TEST-002 | Create `gateway/server_test.go` | [x] | Test SSH handling and picker logic |
| TEST-003 | Add runtime implementation tests | [x] | Docker and nspawn basic tests |

### P2 - Supporting Package Tests

| ID | Task | Status | Notes |
|----|------|--------|-------|
| TEST-004 | Create `app/app_test.go` | [x] | Test dependency injection container |
| TEST-005 | Create `logging/logging_test.go` | [x] | Test logger configuration |
| TEST-006 | Create `tui/picker_test.go` | [x] | Test with mocked terminal |

### P3 - Integration & CI

| ID | Task | Status | Notes |
|----|------|--------|-------|
| TEST-007 | Add end-to-end integration tests | [x] | Docker-based lifecycle tests + workflow tests |
| TEST-008 | Configure GitHub Actions CI | [x] | Run tests on PR |

---

## Phase 3: Architecture Improvements

### P1 - Critical Architecture

| ID | Task | Status | Notes |
|----|------|--------|-------|
| ARCH-001 | Make health checks runtime-agnostic | [x] | Use runtime.Status() instead of machinectl |
| ARCH-002 | Decompose `sandbox.Creator.Create()` | [x] | Extract sub-orchestrators |

### P2 - Moderate Architecture

| ID | Task | Status | Notes |
|----|------|--------|-------|
| ARCH-003 | Persist port registry in metadata | [x] | Remove in-memory sandboxPorts map |
| ARCH-004 | Add Nix config validation | [x] | Validate before writing |
| ARCH-005 | Replace global runtime in commands | [x] | Use injected dependencies |

---

## Phase 4: Maintainability Improvements

### P2 - Code Quality

| ID | Task | Status | Notes |
|----|------|--------|-------|
| MAINT-001 | Extract command helpers | [x] | `loadSandbox()` and `loadRunningSandbox()` helpers |
| MAINT-002 | Unify cleanup logic | [x] | Single `sandbox.Cleanup()` function |
| MAINT-003 | Add example config files | [x] | docs/examples/ with config.json and template |
| MAINT-004 | Standardize logging across commands | [x] | Already consistent: logInfo/logSuccess for status, fmt for data |

### P3 - Polish

| ID | Task | Status | Notes |
|----|------|--------|-------|
| MAINT-005 | Add golangci-lint config | [x] | .golangci.yml with standard linters |
| MAINT-006 | Document config file formats | [x] | docs/examples/README.md has full schema |

---

## Change Log

| Date | Change | Commit |
|------|--------|--------|
| 2026-02-05 | Created remediation tracking document | - |
| 2026-02-05 | SEC-001: Fixed shell injection in copySkillsToContainer | - |
| 2026-02-05 | SEC-002: Added sandbox name validation | - |
| 2026-02-05 | SEC-003: Validated gateway SSH input | - |
| 2026-02-05 | SEC-004: Added path traversal protection | - |
| 2026-02-05 | SEC-005: Added ADR for SSH host key verification | - |
| 2026-02-05 | TEST-001: Added sandbox creator tests | - |
| 2026-02-05 | TEST-002: Added gateway server tests | - |
| 2026-02-05 | TEST-004: Added app package tests | - |
| 2026-02-05 | TEST-005: Added logging package tests | - |
| 2026-02-05 | MAINT-001: Extracted command helpers | - |
| 2026-02-05 | TEST-003: Added runtime implementation tests | - |
| 2026-02-05 | ARCH-001: Made health checks runtime-agnostic | - |
| 2026-02-05 | MAINT-002: Unified cleanup logic | - |
| 2026-02-05 | MAINT-003: Added example config files | - |
| 2026-02-05 | MAINT-005: Added golangci-lint config | - |
| 2026-02-05 | Fixed all golangci-lint issues (0 remaining) | - |
| 2026-02-05 | TEST-006: Added TUI picker tests | - |
| 2026-02-05 | ARCH-003: Persisted port registry in metadata | - |
| 2026-02-05 | MAINT-004: Verified logging already standardized | - |
| 2026-02-05 | MAINT-006: Verified config formats already documented | - |
| 2026-02-05 | TEST-008: Added GitHub Actions CI workflow | - |
| 2026-02-05 | ARCH-004: Added Nix config validation | - |
| 2026-02-05 | ARCH-002: Decomposed sandbox.Creator.Create() | - |
| 2026-02-05 | ARCH-005: Replaced global runtime with injected dependencies | - |
| 2026-02-06 | TEST-007: Added docker-based integration tests and flake.nix | - |


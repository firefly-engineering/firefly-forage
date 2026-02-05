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
| SEC-002 | Add sandbox name validation with allowlist | [ ] | Validate names match `^[a-z0-9][a-z0-9_-]*$` pattern |

### P1 - High Priority Security

| ID | Task | Status | Notes |
|----|------|--------|-------|
| SEC-003 | Validate gateway SSH_ORIGINAL_COMMAND input | [ ] | Sanitize before passing to ConnectToSandbox |
| SEC-004 | Add path traversal protection in config loading | [ ] | Validate name doesn't contain path separators |
| SEC-005 | Document SSH host key verification decision | [ ] | Add ADR explaining the localhost-only trust model |

---

## Phase 2: Test Coverage Improvements

### P1 - Critical Package Tests

| ID | Task | Status | Notes |
|----|------|--------|-------|
| TEST-001 | Create `sandbox/creator_test.go` | [ ] | Test full creation workflow with mocks |
| TEST-002 | Create `gateway/server_test.go` | [ ] | Test SSH handling and picker logic |
| TEST-003 | Add runtime implementation tests | [ ] | Docker and nspawn basic tests |

### P2 - Supporting Package Tests

| ID | Task | Status | Notes |
|----|------|--------|-------|
| TEST-004 | Create `app/app_test.go` | [ ] | Test dependency injection container |
| TEST-005 | Create `logging/logging_test.go` | [ ] | Test logger configuration |
| TEST-006 | Create `tui/picker_test.go` | [ ] | Test with mocked terminal |

### P3 - Integration & CI

| ID | Task | Status | Notes |
|----|------|--------|-------|
| TEST-007 | Add end-to-end integration tests | [ ] | Full sandbox lifecycle |
| TEST-008 | Configure GitHub Actions CI | [ ] | Run tests on PR |

---

## Phase 3: Architecture Improvements

### P1 - Critical Architecture

| ID | Task | Status | Notes |
|----|------|--------|-------|
| ARCH-001 | Make health checks runtime-agnostic | [ ] | Move machinectl out of health package |
| ARCH-002 | Decompose `sandbox.Creator.Create()` | [ ] | Extract sub-orchestrators |

### P2 - Moderate Architecture

| ID | Task | Status | Notes |
|----|------|--------|-------|
| ARCH-003 | Persist port registry in metadata | [ ] | Remove in-memory sandboxPorts map |
| ARCH-004 | Add Nix config validation | [ ] | Validate before writing |
| ARCH-005 | Replace global runtime in commands | [ ] | Use injected dependencies |

---

## Phase 4: Maintainability Improvements

### P2 - Code Quality

| ID | Task | Status | Notes |
|----|------|--------|-------|
| MAINT-001 | Extract command helpers | [ ] | `LoadSandboxOrFail()` utility |
| MAINT-002 | Unify cleanup logic | [ ] | Single `sandbox.Cleanup()` function |
| MAINT-003 | Add example config files | [ ] | Document config.json format |
| MAINT-004 | Standardize logging across commands | [ ] | Consistent log levels |

### P3 - Polish

| ID | Task | Status | Notes |
|----|------|--------|-------|
| MAINT-005 | Add golangci-lint config | [ ] | Enforce code standards |
| MAINT-006 | Document config file formats | [ ] | Full schema documentation |

---

## Change Log

| Date | Change | Commit |
|------|--------|--------|
| 2026-02-05 | Created remediation tracking document | - |
| 2026-02-05 | SEC-001: Fixed shell injection in copySkillsToContainer | - |


# Forage Integration Tests

This directory contains integration tests for verifying forage functionality across different container backends and version control systems.

## Test Matrix

| Backend   | Git Worktree | JJ   |
|-----------|--------------|------|
| nspawn    | ✓            | ✓    |
| docker    | ✓            | ✓    |
| podman    | ✓            | ✓    |
| apple     | ✓            | ✓    |

## Running Tests

### Run all tests

```bash
./run-all.sh
```

### Run tests in parallel

```bash
./run-all.sh --parallel
```

### Run specific backend tests

```bash
./run-all.sh docker    # Only Docker tests
./run-all.sh nspawn    # Only nspawn tests
```

### Run specific VCS tests

```bash
./run-all.sh jj        # Only JJ tests
./run-all.sh git       # Only Git tests
```

### Run a single test

```bash
./test-docker-git.sh
```

## Prerequisites

Tests automatically skip if prerequisites are not met:

- **forage-ctl**: Must be installed and in PATH
- **Backend-specific**:
  - `nspawn`: NixOS with `extra-container` and `machinectl`
  - `docker`: Docker daemon running
  - `podman`: Podman installed
  - `apple`: macOS with Apple's `container` CLI
- **VCS-specific**:
  - `git`: Git installed
  - `jj`: JJ and Git installed

## Test Framework

### Directory Structure

```
tests/integration/
├── lib/
│   ├── common.sh       # Core utilities (logging, cleanup, temp dirs)
│   ├── prereqs.sh      # Prerequisite checking
│   ├── vcs.sh          # VCS helpers (create repos, commit, etc.)
│   ├── sandbox.sh      # Sandbox management (create, start, exec)
│   ├── assertions.sh   # Test assertions
│   └── scenarios.sh    # Test orchestration and common scenarios
├── test-{backend}-{vcs}.sh  # Individual test scripts
├── run-all.sh          # Test runner
└── README.md
```

### Writing Tests

Tests use the `run_scenario` function which orchestrates:

1. Prerequisite checking (auto-skip if not available)
2. Test repository creation
3. Sandbox creation and startup
4. Waiting for sandbox readiness
5. Running your test scenario
6. Cleanup

Example test:

```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/scenarios.sh"

# Define your test scenario
my_test_scenario() {
    # Create a file in the sandbox
    scenario_create_file "test.txt" "Hello World"

    # Verify it exists
    assert_true "File exists in sandbox" \
        "scenario_file_exists test.txt"

    # Verify it synced to host repo
    assert_true "File synced to host" \
        "scenario_repo_file_exists test.txt"
}

# Run with specific backend and VCS
run_scenario "docker" "git" my_test_scenario "my-custom-test"
```

### Available Scenario Functions

Within a scenario function, these helpers are available:

**Sandbox Operations:**
- `scenario_exec "command"` - Execute command in sandbox
- `scenario_exec_capture "command"` - Execute and capture output
- `scenario_create_file "path" "content"` - Create file in workspace
- `scenario_get_file "path"` - Get file content
- `scenario_file_exists "path"` - Check if file exists
- `scenario_vcs_status` - Get VCS status
- `scenario_vcs_commit "message"` - Commit changes

**Host Repository Operations:**
- `scenario_repo_file_exists "path"` - Check file in host repo
- `scenario_repo_get_file "path"` - Get file from host repo
- `scenario_repo_add_file "path" "content"` - Add file to host repo
- `scenario_repo_commit "message"` - Commit in host repo
- `scenario_repo_status` - Get host repo status

**Assertions:**
- `assert_true "description" "condition"`
- `assert_false "description" "condition"`
- `assert_equals "description" "expected" "actual"`
- `assert_contains "description" "haystack" "needle"`
- `assert_file_exists "description" "path"`
- `assert_file_contains "description" "path" "content"`
- `assert_command_succeeds "description" "command"`
- `assert_command_fails "description" "command"`

### Common Scenarios

Pre-built scenarios in `lib/scenarios.sh`:

- `scenario_basic_workspace_access` - Verify sandbox can access workspace
- `scenario_file_creation_sync` - Test file creation and sync to host
- `scenario_vcs_operations` - Test VCS commands inside sandbox
- `scenario_bidirectional_sync` - Test host→sandbox sync
- `scenario_full_integration` - Combines all above scenarios

## Configuration

### Environment Variables

- `FORAGE_TEST_TEMPLATE`: Template name to use (default: "test")
- `SANDBOX_TIMEOUT`: Timeout for sandbox operations in seconds (default: 120)
- `SSH_WAIT_TIMEOUT`: Timeout waiting for SSH in seconds (default: 60)

### Template Requirements

Tests expect a template named "test" to be configured in forage. The template should have:
- Network mode: "none" (for faster startup, no network needed)
- At least one agent configured

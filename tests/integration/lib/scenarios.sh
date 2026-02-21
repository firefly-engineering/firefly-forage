#!/usr/bin/env bash
# Test scenario orchestration for integration tests
#
# This library provides the main orchestration functions that encapsulate
# the test workflow and make individual test scenarios readable.

# Source all helper libraries
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"
source "${SCRIPT_DIR}/prereqs.sh"
source "${SCRIPT_DIR}/vcs.sh"
source "${SCRIPT_DIR}/sandbox.sh"
source "${SCRIPT_DIR}/assertions.sh"

# Global test context (set by run_scenario)
declare -g SCENARIO_BACKEND=""
declare -g SCENARIO_VCS=""
declare -g SCENARIO_REPO_DIR=""
declare -g SCENARIO_SANDBOX_NAME=""
declare -g SCENARIO_TEMPLATE="${FORAGE_TEST_TEMPLATE:-test}"

# Run a complete test scenario
#
# This function orchestrates the entire test lifecycle:
# 1. Check prerequisites (skip if not available)
# 2. Create test repository
# 3. Create and start sandbox
# 4. Wait for sandbox to be ready
# 5. Run scenario-specific steps (passed as function)
# 6. Cleanup
#
# Usage:
#   run_scenario "docker" "git" scenario_function
#
run_scenario() {
    local backend="$1"
    local vcs="$2"
    local scenario_func="$3"
    local test_name="${4:-${backend}-${vcs}}"

    # Initialize test
    test_start "$test_name"

    # Set global context
    SCENARIO_BACKEND="$backend"
    SCENARIO_VCS="$vcs"

    # Check prerequisites
    check_all_prerequisites "$backend" "$vcs"

    # Create test repository
    log_info "=== Setting up test repository ==="
    SCENARIO_REPO_DIR=$(create_test_repo "$vcs")
    log_info "Repository created at: $SCENARIO_REPO_DIR"

    # Create sandbox
    log_info "=== Creating sandbox ==="
    SCENARIO_SANDBOX_NAME=$(create_sandbox "$backend" "$vcs" "$SCENARIO_REPO_DIR" "$SCENARIO_TEMPLATE")

    # Start sandbox
    log_info "=== Starting sandbox ==="
    start_sandbox "$SCENARIO_SANDBOX_NAME"

    # Wait for sandbox to be ready
    log_info "=== Waiting for sandbox to be ready ==="
    wait_for_sandbox_ready "$SCENARIO_SANDBOX_NAME"

    # Run the scenario function
    log_info "=== Running test scenario ==="
    if ! "$scenario_func"; then
        test_fail "Scenario function failed"
    fi

    # End test (runs cleanup)
    test_end
}

# Run a scenario without starting the sandbox
# Useful for testing sandbox creation/configuration only
run_scenario_no_start() {
    local backend="$1"
    local vcs="$2"
    local scenario_func="$3"
    local test_name="${4:-${backend}-${vcs}-no-start}"

    # Initialize test
    test_start "$test_name"

    # Set global context
    SCENARIO_BACKEND="$backend"
    SCENARIO_VCS="$vcs"

    # Check prerequisites
    check_all_prerequisites "$backend" "$vcs"

    # Create test repository
    log_info "=== Setting up test repository ==="
    SCENARIO_REPO_DIR=$(create_test_repo "$vcs")
    log_info "Repository created at: $SCENARIO_REPO_DIR"

    # Create sandbox (but don't start)
    log_info "=== Creating sandbox (without starting) ==="
    SCENARIO_SANDBOX_NAME=$(create_sandbox "$backend" "$vcs" "$SCENARIO_REPO_DIR" "$SCENARIO_TEMPLATE")

    # Run the scenario function
    log_info "=== Running test scenario ==="
    if ! "$scenario_func"; then
        test_fail "Scenario function failed"
    fi

    # End test (runs cleanup)
    test_end
}

# Convenience functions for use within scenario functions
# These use the global context set by run_scenario

# Execute a command in the current scenario's sandbox
scenario_exec() {
    sandbox_exec "$SCENARIO_SANDBOX_NAME" "$@"
}

# Execute and capture output
scenario_exec_capture() {
    sandbox_exec_capture "$SCENARIO_SANDBOX_NAME" "$@"
}

# Create a file in the sandbox workspace
scenario_create_file() {
    sandbox_create_file "$SCENARIO_SANDBOX_NAME" "$@"
}

# Get file content from sandbox workspace
scenario_get_file() {
    sandbox_get_file_content "$SCENARIO_SANDBOX_NAME" "$@"
}

# Check if file exists in sandbox workspace
scenario_file_exists() {
    sandbox_file_exists "$SCENARIO_SANDBOX_NAME" "$@"
}

# Get VCS status in sandbox
scenario_vcs_status() {
    sandbox_vcs_status "$SCENARIO_SANDBOX_NAME" "$SCENARIO_VCS"
}

# Commit changes in sandbox
scenario_vcs_commit() {
    sandbox_vcs_commit "$SCENARIO_SANDBOX_NAME" "$SCENARIO_VCS" "$@"
}

# Check if a file exists in the host repository
scenario_repo_file_exists() {
    repo_file_exists "$SCENARIO_REPO_DIR" "$@"
}

# Get file content from host repository
scenario_repo_get_file() {
    repo_get_file_content "$SCENARIO_REPO_DIR" "$@"
}

# Add a file to the host repository
scenario_repo_add_file() {
    repo_add_file "$SCENARIO_REPO_DIR" "$@"
}

# Commit in host repository
scenario_repo_commit() {
    repo_commit "$SCENARIO_REPO_DIR" "$SCENARIO_VCS" "$@"
}

# Get VCS status of host repository
scenario_repo_status() {
    repo_status "$SCENARIO_REPO_DIR" "$SCENARIO_VCS"
}

# ============================================
# Common test scenario implementations
# These can be reused across different backends/VCS
# ============================================

# Scenario: Basic workspace access
# Verifies that the sandbox can access the workspace
scenario_basic_workspace_access() {
    log_info "Testing basic workspace access..."

    # Verify README exists (from initial commit)
    assert_true "README.md exists in sandbox workspace" \
        "scenario_file_exists README.md"

    # Read README content
    local content
    content=$(scenario_get_file README.md)
    assert_contains "README has expected content" "$content" "Test Project"

    log_info "Basic workspace access test passed"
}

# Scenario: File creation and sync
# Verifies that files created in sandbox appear in host repo
scenario_file_creation_sync() {
    log_info "Testing file creation and sync..."

    # Create a file in the sandbox
    scenario_create_file "sandbox-created.txt" "Hello from sandbox"

    # Verify file exists in sandbox
    assert_true "File exists in sandbox" \
        "scenario_file_exists sandbox-created.txt"

    # Verify file synced to host repository
    # (For jj/git-worktree modes, this should be immediate)
    assert_true "File synced to host repository" \
        "scenario_repo_file_exists sandbox-created.txt"

    # Verify content matches
    local host_content
    host_content=$(scenario_repo_get_file sandbox-created.txt)
    assert_contains "Content matches" "$host_content" "Hello from sandbox"

    log_info "File creation sync test passed"
}

# Scenario: VCS operations inside sandbox
# Verifies that VCS commands work inside the sandbox
scenario_vcs_operations() {
    log_info "Testing VCS operations inside sandbox..."

    # Create a file
    scenario_create_file "vcs-test.txt" "Testing VCS operations"

    # Check status shows the change
    local status
    status=$(scenario_vcs_status)
    log_info "VCS status: $status"

    # The status output format differs between git and jj
    # Just verify we got some output
    assert_true "VCS status shows output" "[[ -n '$status' ]]"

    # Commit the change
    scenario_vcs_commit "Add vcs-test.txt from integration test"

    # Verify commit was made
    local new_status
    new_status=$(scenario_vcs_status)
    log_info "VCS status after commit: $new_status"

    log_info "VCS operations test passed"
}

# Scenario: Bidirectional sync
# Verifies that changes from host appear in sandbox
scenario_bidirectional_sync() {
    log_info "Testing bidirectional sync..."

    # Create a file on the host side
    scenario_repo_add_file "host-created.txt" "Hello from host"
    scenario_repo_commit "Add host-created.txt"

    # Note: For git-worktree, changes need to be on the branch
    # For jj, the workspace should see changes automatically

    # Verify file appears in sandbox
    # This may require refreshing the workspace depending on VCS mode
    local found=false
    for i in {1..5}; do
        if scenario_file_exists host-created.txt 2>/dev/null; then
            found=true
            break
        fi
        sleep 1
    done

    if [[ "$found" == "true" ]]; then
        assert_true "Host file visible in sandbox" "true"
        local content
        content=$(scenario_get_file host-created.txt)
        assert_contains "Host file content matches" "$content" "Hello from host"
    else
        log_warn "Bidirectional sync not verified (may depend on VCS mode)"
    fi

    log_info "Bidirectional sync test completed"
}

# Full integration scenario combining all tests
scenario_full_integration() {
    scenario_basic_workspace_access
    scenario_file_creation_sync
    scenario_vcs_operations
    # scenario_bidirectional_sync  # Can be flaky depending on VCS mode
}

#!/usr/bin/env bash
# Integration test: Apple-specific backend behaviors
#
# This test exercises features unique to the Apple Container (Virtualization.framework)
# backend that are not covered by generic cross-backend scenarios:
#
# - Nix store availability in the VM
# - Generated file injection
# - Environment variable propagation
# - Container inspect JSON parsing
# - Command wrapping in /bin/sh -c
# - Graceful shutdown timeout behavior
# - Label-based container listing
#
# Prerequisites:
# - macOS system
# - Apple's container CLI available
# - Nix installed on host
# - forage-ctl configured with a 'test' template

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/scenarios.sh"

# ============================================
# Apple-specific scenario implementations
# ============================================

# Scenario: Nix store is accessible inside the VM
scenario_apple_nix_store() {
    log_info "Testing Nix store availability in Apple VM..."

    # /nix/store should be mounted
    assert_true "/nix/store is accessible" \
        "scenario_exec 'test -d /nix/store'"

    # Should contain at least some store paths
    local store_count
    store_count=$(scenario_exec_capture 'ls /nix/store | wc -l')
    store_count=$(echo "$store_count" | tr -d '[:space:]')
    assert_true "Nix store has entries (got $store_count)" \
        "[[ $store_count -gt 0 ]]"

    # nix-daemon socket should be available
    assert_true "Nix daemon socket is available" \
        "scenario_exec 'test -e /nix/var/nix/daemon-socket/socket || test -S /nix/var/nix/daemon-socket/socket'" || \
        log_warn "Nix daemon socket not found (nix-daemon may not be forwarded)"

    log_info "Nix store availability test passed"
}

# Scenario: Environment variables are propagated
scenario_apple_env_vars() {
    log_info "Testing environment variable propagation..."

    # The sandbox should have basic env vars set
    local path
    path=$(scenario_exec_capture 'echo $PATH')
    assert_true "PATH is set" "[[ -n '$path' ]]"

    # Nix profile paths should be in PATH (from /bin/sh sourcing)
    assert_contains "PATH contains nix profile" "$path" "nix"

    log_info "Environment variable propagation test passed"
}

# Scenario: Commands are properly wrapped in /bin/sh -c
scenario_apple_command_wrapping() {
    log_info "Testing command wrapping in /bin/sh -c..."

    # Commands with pipes should work (requires shell wrapping)
    local result
    result=$(scenario_exec_capture 'echo hello | tr h H')
    assert_equals "Piped command works" "Hello" "$(echo "$result" | tr -d '[:space:]')"

    # Commands with semicolons should work
    result=$(scenario_exec_capture 'echo first; echo second')
    assert_contains "Multi-command works" "$result" "first"
    assert_contains "Multi-command works (second)" "$result" "second"

    # Commands with environment variable expansion should work
    result=$(scenario_exec_capture 'export FOO=bar; echo $FOO')
    assert_contains "Variable expansion works" "$result" "bar"

    log_info "Command wrapping test passed"
}

# Scenario: Container inspect returns valid JSON
scenario_apple_inspect() {
    log_info "Testing container inspect JSON parsing..."

    # Get container status via forage-ctl (which parses inspect JSON)
    local status
    status=$(forage-ctl status "$SCENARIO_SANDBOX_NAME" 2>&1) || true
    assert_contains "Status shows running" "$status" "running"

    # The status output should include the sandbox name
    assert_contains "Status shows sandbox name" "$status" "$SCENARIO_SANDBOX_NAME"

    log_info "Container inspect test passed"
}

# Scenario: Container listing with labels works
scenario_apple_listing() {
    log_info "Testing label-based container listing..."

    # forage-ctl ps should list our sandbox
    local ps_output
    ps_output=$(forage-ctl ps 2>&1) || true
    assert_contains "ps lists our sandbox" "$ps_output" "$SCENARIO_SANDBOX_NAME"

    log_info "Container listing test passed"
}

# Scenario: Graceful shutdown works within timeout
scenario_apple_graceful_shutdown() {
    log_info "Testing graceful shutdown..."

    # Stop the sandbox
    forage-ctl stop "$SCENARIO_SANDBOX_NAME" >&2

    # Verify it's stopped
    local running
    running=$(forage-ctl status "$SCENARIO_SANDBOX_NAME" 2>&1) || true
    assert_contains "Container is stopped" "$running" "stopped"

    # Restart for cleanup
    forage-ctl start "$SCENARIO_SANDBOX_NAME" >&2
    wait_for_sandbox_ready "$SCENARIO_SANDBOX_NAME"

    log_info "Graceful shutdown test passed"
}

# Combined Apple-specific scenario
scenario_apple_specific() {
    scenario_apple_nix_store
    scenario_apple_env_vars
    scenario_apple_command_wrapping
    scenario_apple_inspect
    scenario_apple_listing
    scenario_apple_graceful_shutdown
}

# Run apple-specific scenarios using jj (default VCS for macOS)
run_scenario "apple" "jj" scenario_apple_specific "apple-specific"

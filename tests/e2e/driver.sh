#!/usr/bin/env bash
# E2E test driver for Firefly Forage
#
# This script boots a NixOS QEMU VM with the forage module configured,
# then runs the full sandbox lifecycle via SSH:
#   1. forage-ctl up (create + start sandbox with extra-container)
#   2. SSH exec (run commands inside nspawn container)
#   3. forage-ctl down (stop + destroy sandbox)
#
# Prerequisites:
#   - E2E_VM: path to the VM run script (set by nix wrapper)
#   - E2E_SSH_KEY: path to the SSH private key (set by nix wrapper)
#
# Usage:
#   ./driver.sh              # Run all scenarios
#   ./driver.sh --scenario X # Run specific scenario

set -euo pipefail

# Global timeout (15 minutes). Kills the entire test run if exceeded.
E2E_TIMEOUT="${E2E_TIMEOUT:-900}"

# Enforce global timeout by re-execing under `timeout(1)`.
# The guard variable prevents infinite recursion.
if [[ -z "${_E2E_TIMEOUT_SET:-}" ]]; then
    export _E2E_TIMEOUT_SET=1
    exec timeout --signal=TERM --kill-after=30 "$E2E_TIMEOUT" "$0" "$@"
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors for output (disabled if not a terminal)
if [[ -t 1 ]]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[0;33m'
    BLUE='\033[0;34m'
    NC='\033[0m'
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    NC=''
fi

# Logging
log_info()    { echo -e "${BLUE}[INFO]${NC} $*"; }
log_success() { echo -e "${GREEN}[PASS]${NC} $*"; }
log_warn()    { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_error()   { echo -e "${RED}[FAIL]${NC} $*"; }

# Source VM helpers
source "${SCRIPT_DIR}/lib/vm-helpers.sh"

# Test state
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_TOTAL=0
CURRENT_TEST=""

# Cleanup trap
cleanup() {
    log_info "Running cleanup..."
    if vm_is_running; then
        vm_shutdown
    fi
}
trap cleanup EXIT

# ============================================
# Container SSH helpers
# ============================================
# We use raw SSH to connect to containers instead of `forage-ctl exec`
# because cobra v1.10.2 strips the `--` separator before the exec handler
# sees it, making `forage-ctl exec <name> -- <cmd>` fail over SSH.
# This is a known issue; forage-ctl exec should use cmd.Flags().ArgsAfterDash().

# SSH options for connecting to containers inside the VM
CONTAINER_SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=5 -o BatchMode=yes"

# Execute a command in a sandbox container via SSH (through the VM)
# The command is wrapped in single quotes so shell metacharacters (>, |, &&)
# survive the double-SSH hop (host -> VM -> container) without being
# interpreted by the VM's shell.
# Usage: sandbox_exec <container_ip> <command...>
sandbox_exec() {
    local ip="$1"
    shift
    local cmd="$*"
    # Escape embedded single quotes: ' -> '\''
    cmd="${cmd//\'/\'\\\'\'}"
    vm_exec "ssh $CONTAINER_SSH_OPTS agent@${ip} '${cmd}'"
}

# Capture output from a sandbox container
sandbox_exec_capture() {
    local ip="$1"
    shift
    local cmd="$*"
    cmd="${cmd//\'/\'\\\'\'}"
    vm_exec_capture "ssh $CONTAINER_SSH_OPTS agent@${ip} '${cmd}'"
}

# Wait for a sandbox container to become SSH-reachable
# Usage: wait_sandbox_ready <container_ip> [timeout_seconds]
wait_sandbox_ready() {
    local ip="$1"
    local timeout="${2:-30}"
    local ready=false

    for i in $(seq 1 "$timeout"); do
        if sandbox_exec "$ip" true 2>/dev/null; then
            ready=true
            log_info "  Sandbox ready after ${i}s"
            break
        fi
        sleep 1
    done

    if [[ "$ready" != "true" ]]; then
        log_error "  Sandbox at $ip did not become ready within ${timeout}s"
        # Diagnostics
        log_info "  Diagnostic: machinectl list"
        vm_exec "machinectl list" 2>&1 || true
        log_info "  Diagnostic: forage-ctl ps"
        vm_exec "forage-ctl ps" 2>&1 || true
        log_info "  Diagnostic: ping $ip"
        vm_exec "ping -c 1 -W 2 $ip" 2>&1 || true
        log_info "  Diagnostic: SSH verbose"
        vm_exec "ssh -v $CONTAINER_SSH_OPTS agent@${ip} true" 2>&1 || true
        return 1
    fi
}

# ============================================
# Test assertion helpers
# ============================================

# Assert a command (passed as a single string) succeeds in the VM.
# Always returns 0 (safe with set -e); tracks failures via TESTS_FAILED.
assert_vm_success() {
    local description="$1"
    local cmd="$2"
    TESTS_TOTAL=$((TESTS_TOTAL + 1))

    if vm_exec "$cmd"; then
        log_success "  $description"
    else
        log_error "  $description"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
}

# Assert command output contains expected string
assert_vm_output_contains() {
    local description="$1"
    local expected="$2"
    local cmd="$3"
    TESTS_TOTAL=$((TESTS_TOTAL + 1))

    local output
    output=$(vm_exec_capture "$cmd") || true

    if [[ "$output" == *"$expected"* ]]; then
        log_success "  $description"
    else
        log_error "  $description (expected to contain: '$expected')"
        log_error "  actual output: $output"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
}

# Assert a sandbox command succeeds
assert_sandbox_success() {
    local description="$1"
    local ip="$2"
    local cmd="$3"
    TESTS_TOTAL=$((TESTS_TOTAL + 1))

    if sandbox_exec "$ip" "$cmd"; then
        log_success "  $description"
    else
        log_error "  $description"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
}

# Assert sandbox command output contains expected string
assert_sandbox_output_contains() {
    local description="$1"
    local expected="$2"
    local ip="$3"
    local cmd="$4"
    TESTS_TOTAL=$((TESTS_TOTAL + 1))

    local output
    output=$(sandbox_exec_capture "$ip" "$cmd") || true

    if [[ "$output" == *"$expected"* ]]; then
        log_success "  $description"
    else
        log_error "  $description (expected to contain: '$expected')"
        log_error "  actual output: $output"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
}

# Assert a command fails in the VM
assert_vm_failure() {
    local description="$1"
    local cmd="$2"
    TESTS_TOTAL=$((TESTS_TOTAL + 1))

    if ! vm_exec "$cmd" 2>/dev/null; then
        log_success "  $description"
    else
        log_error "  $description (expected failure but succeeded)"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
}

begin_test() {
    local name="$1"
    CURRENT_TEST="$name"
    echo ""
    echo -e "${BLUE}--- $name ---${NC}"
}

# ============================================
# Test scenarios
# ============================================

scenario_module_setup() {
    begin_test "Module Setup Verification"
    local failed_before=$TESTS_FAILED

    # Verify forage-ctl is installed
    assert_vm_success "forage-ctl is available" \
        "which forage-ctl"

    # Verify module created expected directories
    assert_vm_success "state directory exists" \
        "test -d /var/lib/firefly-forage"
    assert_vm_success "sandboxes directory exists" \
        "test -d /var/lib/firefly-forage/sandboxes"
    assert_vm_success "workspaces directory exists" \
        "test -d /var/lib/firefly-forage/workspaces"
    assert_vm_success "templates directory exists" \
        "test -d /etc/firefly-forage/templates"

    # Verify host config
    assert_vm_success "host config exists" \
        "test -f /etc/firefly-forage/config.json"
    assert_vm_output_contains "host config has correct user" '"user"' \
        "cat /etc/firefly-forage/config.json"

    # Verify template
    assert_vm_success "template JSON exists" \
        "test -f /etc/firefly-forage/templates/test.json"
    assert_vm_output_contains "template has network=none" '"network"' \
        "cat /etc/firefly-forage/templates/test.json"

    # Verify extra-container is available
    assert_vm_success "extra-container is available" \
        "which extra-container"

    # Verify secrets directory
    assert_vm_success "secrets directory exists" \
        "test -d /run/forage-secrets"

    [[ $TESTS_FAILED -eq $failed_before ]]
}

scenario_sandbox_lifecycle() {
    begin_test "Sandbox Full Lifecycle (up/ssh/down)"
    local failed_before=$TESTS_FAILED

    # Clean up any stale sandboxes from previous runs
    vm_exec "forage-ctl down e2e-test" 2>/dev/null || true

    # Create a test repository for the workspace
    log_info "  Creating test repository..."
    vm_exec 'rm -rf /tmp/e2e-project && mkdir -p /tmp/e2e-project && cd /tmp/e2e-project && git init -q && echo "# E2E Test Project" > README.md && git add README.md && git commit -q -m "Initial commit" && chown -R 1000:100 /tmp/e2e-project'

    # === forage-ctl up ===
    log_info "  Running forage-ctl up..."
    # Run forage-ctl up, redirecting build output to a log file inside the VM
    # to avoid flooding the SSH session with nix-build output (which can cause
    # SSH channel overflows and connection drops).
    assert_vm_success "forage-ctl up succeeds" \
        "forage-ctl up e2e-test -t test --repo /tmp/e2e-project --direct > /tmp/forage-up.log 2>&1"

    # Bail out early if creation failed — remaining assertions need a running sandbox
    if [[ $TESTS_FAILED -gt $failed_before ]]; then
        log_error "  Cannot continue: sandbox creation failed"
        # Diagnostics: why did the build fail?
        log_info "  Diagnostic: dmesg (last 20 lines)"
        vm_exec "dmesg | tail -20" 2>&1 || true
        log_info "  Diagnostic: disk space"
        vm_exec "df -h" 2>&1 || true
        log_info "  Diagnostic: memory"
        vm_exec "free -h" 2>&1 || true
        log_info "  Diagnostic: forage-ctl up log (last 30 lines)"
        vm_exec "tail -30 /tmp/forage-up.log" 2>&1 || true
        log_info "  Diagnostic: extra-container logs"
        vm_exec "journalctl -u 'container@*' --no-pager -n 30" 2>&1 || true
        vm_exec "forage-ctl down e2e-test" 2>/dev/null || true
        return 1
    fi

    # Container IP: forage uses 10.100.<slot>.2, slot 1 for the first sandbox
    local container_ip="10.100.1.2"

    # Wait for container SSH to be reachable
    log_info "  Waiting for sandbox to be ready..."
    if ! wait_sandbox_ready "$container_ip" 60; then
        TESTS_FAILED=$((TESTS_FAILED + 1))
        vm_exec "forage-ctl down e2e-test" 2>/dev/null || true
        return 1
    fi

    # === exec via SSH - basic connectivity ===
    assert_sandbox_success "exec: can run commands in sandbox" \
        "$container_ip" "true"

    # === exec via SSH - verify workspace ===
    assert_sandbox_output_contains "exec: workspace has README" "E2E Test Project" \
        "$container_ip" "cat /workspace/README.md"

    # === exec via SSH - verify forage metadata ===
    assert_sandbox_output_contains "exec: forage.json has sandbox name" "e2e-test" \
        "$container_ip" "cat /etc/forage.json"

    # === exec via SSH - verify packages ===
    assert_sandbox_success "exec: git is available in sandbox" \
        "$container_ip" "which git"
    assert_sandbox_success "exec: jj is available in sandbox" \
        "$container_ip" "which jj"
    assert_sandbox_success "exec: tmux is available in sandbox" \
        "$container_ip" "which tmux"

    # === exec via SSH - VCS operations ===
    # Verify git works in the workspace (already a git repo from creation)
    assert_sandbox_output_contains "exec: git log works in workspace" "Initial commit" \
        "$container_ip" "cd /workspace && git log --oneline -1"

    # Verify jj can operate on the git-backed workspace
    assert_sandbox_success "exec: jj init works in workspace" \
        "$container_ip" "cd /workspace && jj git init --colocate 2>&1"
    assert_sandbox_success "exec: jj log works after init" \
        "$container_ip" "cd /workspace && jj log --no-graph -r @ -T description 2>&1"

    # === exec via SSH - create file in workspace ===
    assert_sandbox_success "exec: can create files in workspace" \
        "$container_ip" "echo hello-from-sandbox > /workspace/sandbox-created.txt"

    # Verify file synced to host
    assert_vm_success "workspace sync: file visible on host" \
        "test -f /tmp/e2e-project/sandbox-created.txt"
    assert_vm_output_contains "workspace sync: content matches" "hello-from-sandbox" \
        "cat /tmp/e2e-project/sandbox-created.txt"

    # === forage-ctl status ===
    assert_vm_output_contains "status shows container healthy" "Container:" \
        "forage-ctl status e2e-test"

    # === forage-ctl ps ===
    assert_vm_output_contains "ps shows sandbox" "e2e-test" \
        "forage-ctl ps"

    # === forage-ctl down ===
    log_info "  Running forage-ctl down..."
    assert_vm_success "forage-ctl down succeeds" \
        "forage-ctl down e2e-test"

    # Verify sandbox is gone
    assert_vm_failure "sandbox no longer exists after down" \
        "forage-ctl status e2e-test"

    [[ $TESTS_FAILED -eq $failed_before ]]
}

scenario_multiple_sandboxes() {
    begin_test "Multiple Concurrent Sandboxes"
    local failed_before=$TESTS_FAILED

    # Clean up any stale sandboxes
    vm_exec "forage-ctl down e2e-multi-a" 2>/dev/null || true
    vm_exec "forage-ctl down e2e-multi-b" 2>/dev/null || true

    # Create two separate project directories
    vm_exec 'rm -rf /tmp/e2e-project-a /tmp/e2e-project-b && mkdir -p /tmp/e2e-project-a /tmp/e2e-project-b && cd /tmp/e2e-project-a && git init -q && echo "# Project A" > README.md && git add . && git commit -q -m "Init A" && cd /tmp/e2e-project-b && git init -q && echo "# Project B" > README.md && git add . && git commit -q -m "Init B" && chown -R 1000:100 /tmp/e2e-project-a /tmp/e2e-project-b'

    # Start sandbox A
    log_info "  Starting sandbox A..."
    assert_vm_success "sandbox A created" \
        "forage-ctl up e2e-multi-a -t test --repo /tmp/e2e-project-a --direct > /tmp/forage-multi-a.log 2>&1"

    if [[ $TESTS_FAILED -gt $failed_before ]]; then
        log_error "  Cannot continue: sandbox A creation failed"
        log_info "  Diagnostic: forage-ctl up log (last 20 lines)"
        vm_exec "tail -20 /tmp/forage-multi-a.log" 2>&1 || true
        vm_exec "forage-ctl down e2e-multi-a" 2>/dev/null || true
        return 1
    fi

    # Start sandbox B
    log_info "  Starting sandbox B..."
    assert_vm_success "sandbox B created" \
        "forage-ctl up e2e-multi-b -t test --repo /tmp/e2e-project-b --direct > /tmp/forage-multi-b.log 2>&1"

    if [[ $TESTS_FAILED -gt $failed_before ]]; then
        log_error "  Cannot continue: sandbox B creation failed"
        log_info "  Diagnostic: forage-ctl up log (last 20 lines)"
        vm_exec "tail -20 /tmp/forage-multi-b.log" 2>&1 || true
        vm_exec "forage-ctl down e2e-multi-a" 2>/dev/null || true
        vm_exec "forage-ctl down e2e-multi-b" 2>/dev/null || true
        return 1
    fi

    # Verify both sandboxes are independently accessible
    local ip_a="10.100.1.2"
    local ip_b="10.100.2.2"
    log_info "  Sandbox A IP: $ip_a, Sandbox B IP: $ip_b"

    assert_sandbox_output_contains "sandbox A has project-a README" "Project A" \
        "$ip_a" "cat /workspace/README.md"
    assert_sandbox_output_contains "sandbox B has project-b README" "Project B" \
        "$ip_b" "cat /workspace/README.md"

    # Verify ps shows both
    assert_vm_output_contains "ps shows sandbox A" "e2e-multi-a" \
        "forage-ctl ps"
    assert_vm_output_contains "ps shows sandbox B" "e2e-multi-b" \
        "forage-ctl ps"

    # Clean up
    vm_exec "forage-ctl down e2e-multi-a" 2>/dev/null || true
    vm_exec "forage-ctl down e2e-multi-b" 2>/dev/null || true

    [[ $TESTS_FAILED -eq $failed_before ]]
}

# ============================================
# Main
# ============================================

main() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}Firefly Forage E2E Test Suite${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo ""

    # Validate environment
    if [[ -z "${E2E_VM:-}" ]]; then
        log_error "E2E_VM not set. Run via 'just test-e2e' or the nix wrapper."
        exit 1
    fi

    if [[ -z "${E2E_SSH_KEY:-}" ]]; then
        log_error "E2E_SSH_KEY not set."
        exit 1
    fi

    local vm_script="$E2E_VM"
    if [[ ! -x "$vm_script" ]]; then
        log_error "VM script not found or not executable: $vm_script"
        exit 1
    fi

    # Kill any leftover VMs from previous runs (port 2222 conflict)
    if (echo >/dev/tcp/localhost/"$VM_SSH_PORT") 2>/dev/null; then
        log_warn "Port $VM_SSH_PORT already in use — killing previous VM"
        if command -v fuser &>/dev/null; then
            fuser -k "$VM_SSH_PORT"/tcp 2>/dev/null || true
        else
            log_warn "fuser not available; cannot auto-kill. Please stop the other VM."
        fi
        sleep 2
    fi

    # Clean up stale disk images from previous runs.
    # NixOS VM names the qcow2 after the hostname (forage-e2e).
    rm -f forage-e2e.qcow2

    log_info "VM script: $vm_script"
    log_info "SSH key: $E2E_SSH_KEY"
    log_info "Timeout: ${E2E_TIMEOUT}s"

    # Boot the VM
    vm_boot "$vm_script"

    # Wait for SSH
    vm_wait_ssh

    # Wait for system to be fully ready
    log_info "Waiting for multi-user.target..."
    vm_exec "systemctl is-system-running --wait" 2>/dev/null || true

    # Run scenarios
    local overall_failed=0

    # Always run module setup first
    if scenario_module_setup; then
        log_success "Module setup: PASSED"
    else
        log_error "Module setup: FAILED"
        overall_failed=1
    fi

    # Sandbox lifecycle (core test)
    if scenario_sandbox_lifecycle; then
        log_success "Sandbox lifecycle: PASSED"
    else
        log_error "Sandbox lifecycle: FAILED"
        overall_failed=1
    fi

    # Check VM is still alive before next scenario
    if ! vm_check_alive; then
        log_error "VM is no longer reachable — skipping remaining scenarios"
        overall_failed=1
    else
        # Multiple concurrent sandboxes
        if scenario_multiple_sandboxes; then
            log_success "Multiple sandboxes: PASSED"
        else
            log_error "Multiple sandboxes: FAILED"
            overall_failed=1
        fi
    fi

    # Summary
    echo ""
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}Summary${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo -e "Total assertions: $TESTS_TOTAL"
    echo -e "${GREEN}Passed: $((TESTS_TOTAL - TESTS_FAILED))${NC}"
    echo -e "${RED}Failed: $TESTS_FAILED${NC}"
    echo ""

    if [[ $overall_failed -ne 0 ]]; then
        log_error "E2E tests FAILED"
        exit 1
    else
        log_success "All E2E tests PASSED"
        exit 0
    fi
}

main "$@"

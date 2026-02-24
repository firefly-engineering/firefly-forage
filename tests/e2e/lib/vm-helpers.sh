#!/usr/bin/env bash
# VM lifecycle helpers for E2E tests
#
# This library provides functions to boot, manage, and shut down the
# QEMU VM used for E2E testing.

set -euo pipefail

# Configuration
VM_SSH_PORT="${VM_SSH_PORT:-2222}"
VM_SSH_TIMEOUT="${VM_SSH_TIMEOUT:-120}"
VM_PID=""
VM_PIDFILE=""
VM_SSH_KEY_COPY=""
VM_CONSOLE_LOG=""

# Create a dedicated temp directory for all test artifacts.
# Using a single directory prevents files from being cleaned up independently
# by /tmp cleaners and makes cleanup reliable.
VM_TMPDIR=$(mktemp -d /tmp/forage-e2e.XXXXXX)

# Prepare SSH key - copy from nix store to temp file with correct permissions.
# SSH requires private keys to have 0600, but nix store files are read-only.
vm_prepare_ssh_key() {
    VM_SSH_KEY_COPY="$VM_TMPDIR/ssh-key"
    cp "$E2E_SSH_KEY" "$VM_SSH_KEY_COPY"
    chmod 600 "$VM_SSH_KEY_COPY"
}
vm_prepare_ssh_key

# SSH options for connecting to the VM
VM_SSH_OPTS=(
    -o StrictHostKeyChecking=no
    -o UserKnownHostsFile=/dev/null
    -o ConnectTimeout=5
    -o ServerAliveInterval=15
    -o ServerAliveCountMax=20
    -o LogLevel=ERROR
    -p "$VM_SSH_PORT"
    -i "$VM_SSH_KEY_COPY"
)

# Boot the VM in the background
# Uses QEMU with optional KVM acceleration
vm_boot() {
    local vm_script="$1"

    log_info "Booting E2E VM..."

    # Clean up any leftover disk images from previous runs to ensure fresh state
    rm -f forage-e2e.qcow2

    VM_PIDFILE="$VM_TMPDIR/vm.pid"
    VM_CONSOLE_LOG="$VM_TMPDIR/console.log"

    # Check for KVM availability
    local qemu_extra_args=""
    if [[ -c /dev/kvm ]] && [[ -w /dev/kvm ]]; then
        log_info "KVM acceleration available"
    else
        log_warn "KVM not available, using software emulation (slower)"
    fi

    # Capture serial console output to diagnose VM crashes.
    # The VM kernel is configured with console=ttyS0, so kernel messages
    # and panic output will appear in this log file.
    qemu_extra_args="-serial file:$VM_CONSOLE_LOG"

    # shellcheck disable=SC2086
    $vm_script \
        -daemonize \
        -pidfile "$VM_PIDFILE" \
        -display none \
        $qemu_extra_args \
        &>/dev/null || true

    # Wait for PID file to be written.
    # The VM run script creates a qcow2 disk and a ~2GB erofs nix store
    # image before starting QEMU, which can take 60-120 seconds.
    local waited=0
    while [[ ! -s "$VM_PIDFILE" ]] && [[ $waited -lt 300 ]]; do
        sleep 1
        waited=$((waited + 1))
    done

    if [[ -s "$VM_PIDFILE" ]]; then
        VM_PID=$(cat "$VM_PIDFILE")
        log_info "VM started with PID $VM_PID"
    else
        log_error "Failed to start VM (no PID file after ${waited}s)"
        return 1
    fi
}

# Wait for the VM to become reachable via SSH
vm_wait_ssh() {
    local timeout="${1:-$VM_SSH_TIMEOUT}"

    log_info "Waiting for VM SSH to become ready (timeout: ${timeout}s)..."

    local start_time
    start_time=$(date +%s)

    while true; do
        local elapsed
        elapsed=$(( $(date +%s) - start_time ))

        if [[ $elapsed -ge $timeout ]]; then
            log_error "Timeout waiting for VM SSH after ${timeout}s"
            return 1
        fi

        if vm_exec_quiet true 2>/dev/null; then
            log_info "VM SSH ready (took ${elapsed}s)"
            return 0
        fi

        sleep 2
    done
}

# Execute a command in the VM via SSH
vm_exec() {
    ssh "${VM_SSH_OPTS[@]}" root@localhost "$@"
}

# Execute a command in the VM, suppressing stderr
vm_exec_quiet() {
    ssh "${VM_SSH_OPTS[@]}" root@localhost "$@" 2>/dev/null
}

# Execute a command and capture stdout (stderr suppressed to avoid SSH warnings
# like "Identity file not accessible" from polluting assertion matching)
vm_exec_capture() {
    ssh "${VM_SSH_OPTS[@]}" root@localhost "$@" 2>/dev/null
}

# Dump the VM console log (useful for diagnosing crashes)
vm_dump_console() {
    if [[ -n "$VM_CONSOLE_LOG" ]] && [[ -f "$VM_CONSOLE_LOG" ]]; then
        log_info "VM console log (last 50 lines):"
        tail -50 "$VM_CONSOLE_LOG" 2>/dev/null || true
    fi
}

# Shut down the VM gracefully
vm_shutdown() {
    log_info "Shutting down VM..."

    # Try graceful shutdown first
    if vm_exec_quiet poweroff 2>/dev/null; then
        # Wait for process to exit
        local waited=0
        while [[ $waited -lt 30 ]] && kill -0 "$VM_PID" 2>/dev/null; do
            sleep 1
            waited=$((waited + 1))
        done
    fi

    # Force kill if still running
    if [[ -n "$VM_PID" ]] && kill -0 "$VM_PID" 2>/dev/null; then
        log_warn "VM did not shut down gracefully, killing PID $VM_PID"
        kill "$VM_PID" 2>/dev/null || true
        sleep 1
        kill -9 "$VM_PID" 2>/dev/null || true
    fi

    # Clean up QEMU disk images (NixOS names the file after the hostname)
    rm -f forage-e2e.qcow2

    # Clean up temp directory (contains pidfile, console log, SSH key)
    if [[ -n "$VM_TMPDIR" ]] && [[ -d "$VM_TMPDIR" ]]; then
        rm -rf "$VM_TMPDIR"
    fi

    log_info "VM shut down"
}

# Check if VM process is alive
vm_is_running() {
    [[ -n "$VM_PID" ]] && kill -0 "$VM_PID" 2>/dev/null
}

# Check if VM is alive and SSH-reachable. Prints diagnostics on failure.
vm_check_alive() {
    if ! vm_is_running; then
        log_error "VM process (PID $VM_PID) is no longer running"
        vm_dump_console
        return 1
    fi
    if ! vm_exec_quiet true 2>/dev/null; then
        log_error "VM is running but SSH is not responding"
        return 1
    fi
}

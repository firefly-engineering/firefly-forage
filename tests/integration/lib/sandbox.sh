#!/usr/bin/env bash
# Sandbox helpers for integration tests
#
# This library provides functions to create, manage, and interact with
# forage sandboxes.

# Source common utilities and prereqs
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"
source "${SCRIPT_DIR}/prereqs.sh"

# Default timeout for sandbox operations (in seconds)
SANDBOX_TIMEOUT="${SANDBOX_TIMEOUT:-120}"
SSH_WAIT_TIMEOUT="${SSH_WAIT_TIMEOUT:-60}"

# Generate a unique sandbox name for testing
generate_sandbox_name() {
    local prefix="${1:-test}"
    echo "${prefix}-$(date +%s)-$$"
}

# Create a sandbox
# Returns the sandbox name
create_sandbox() {
    local backend="$1"
    local vcs="$2"
    local repo_dir="$3"
    local template="${4:-test}"
    local sandbox_name="${5:-$(generate_sandbox_name)}"

    local runtime_flag
    runtime_flag=$(get_runtime_flag "$backend")

    local workspace_flag
    workspace_flag=$(get_workspace_mode_flag "$vcs")

    log_info "Creating sandbox: $sandbox_name"
    log_info "  Backend: $backend"
    log_info "  VCS: $vcs"
    log_info "  Repository: $repo_dir"
    log_info "  Template: $template"

    if ! forage-ctl create "$sandbox_name" \
        --template="$template" \
        "$runtime_flag" \
        "$workspace_flag" \
        --source="$repo_dir" \
        --yes; then
        test_fail "Failed to create sandbox: $sandbox_name"
        return 1
    fi

    # Register cleanup
    register_cleanup "destroy_sandbox '$sandbox_name' || true"

    log_info "Sandbox created: $sandbox_name"
    echo "$sandbox_name"
}

# Start a sandbox
start_sandbox() {
    local sandbox_name="$1"

    log_info "Starting sandbox: $sandbox_name"

    if ! forage-ctl start "$sandbox_name"; then
        test_fail "Failed to start sandbox: $sandbox_name"
        return 1
    fi

    log_info "Sandbox started: $sandbox_name"
}

# Stop a sandbox
stop_sandbox() {
    local sandbox_name="$1"

    log_info "Stopping sandbox: $sandbox_name"

    if ! forage-ctl stop "$sandbox_name"; then
        log_warn "Failed to stop sandbox: $sandbox_name"
        return 1
    fi

    log_info "Sandbox stopped: $sandbox_name"
}

# Destroy a sandbox
destroy_sandbox() {
    local sandbox_name="$1"

    log_info "Destroying sandbox: $sandbox_name"

    if ! forage-ctl destroy "$sandbox_name" --yes 2>/dev/null; then
        log_warn "Failed to destroy sandbox: $sandbox_name (may already be destroyed)"
        return 1
    fi

    log_info "Sandbox destroyed: $sandbox_name"
}

# Wait for sandbox to be ready (SSH available)
wait_for_sandbox_ready() {
    local sandbox_name="$1"
    local timeout="${2:-$SSH_WAIT_TIMEOUT}"

    log_info "Waiting for sandbox to be ready (timeout: ${timeout}s)..."

    local start_time
    start_time=$(date +%s)

    while true; do
        local elapsed
        elapsed=$(( $(date +%s) - start_time ))

        if [[ $elapsed -ge $timeout ]]; then
            test_fail "Timeout waiting for sandbox to be ready"
            return 1
        fi

        # Try to execute a simple command
        if forage-ctl exec "$sandbox_name" -- true 2>/dev/null; then
            log_info "Sandbox is ready (took ${elapsed}s)"
            return 0
        fi

        sleep 1
    done
}

# Execute a command in the sandbox
sandbox_exec() {
    local sandbox_name="$1"
    shift
    local cmd="$*"

    log_info "Executing in sandbox: $cmd"
    forage-ctl exec "$sandbox_name" -- bash -c "$cmd"
}

# Execute a command in the sandbox and capture output
sandbox_exec_capture() {
    local sandbox_name="$1"
    shift
    local cmd="$*"

    forage-ctl exec "$sandbox_name" -- bash -c "$cmd" 2>&1
}

# Check if a file exists in the sandbox workspace
sandbox_file_exists() {
    local sandbox_name="$1"
    local filepath="$2"

    sandbox_exec "$sandbox_name" "test -f /workspace/$filepath" 2>/dev/null
}

# Get the content of a file in the sandbox workspace
sandbox_get_file_content() {
    local sandbox_name="$1"
    local filepath="$2"

    sandbox_exec_capture "$sandbox_name" "cat /workspace/$filepath"
}

# Create a file in the sandbox workspace
sandbox_create_file() {
    local sandbox_name="$1"
    local filepath="$2"
    local content="$3"

    sandbox_exec "$sandbox_name" "echo '$content' > /workspace/$filepath"
}

# Append to a file in the sandbox workspace
sandbox_append_file() {
    local sandbox_name="$1"
    local filepath="$2"
    local content="$3"

    sandbox_exec "$sandbox_name" "echo '$content' >> /workspace/$filepath"
}

# List files in the sandbox workspace
sandbox_list_workspace() {
    local sandbox_name="$1"

    sandbox_exec_capture "$sandbox_name" "ls -la /workspace/"
}

# Get sandbox status
sandbox_status() {
    local sandbox_name="$1"

    forage-ctl status "$sandbox_name"
}

# Check if sandbox is running
sandbox_is_running() {
    local sandbox_name="$1"

    forage-ctl status "$sandbox_name" 2>/dev/null | grep -q "running"
}

# Get the VCS status inside the sandbox
sandbox_vcs_status() {
    local sandbox_name="$1"
    local vcs="$2"

    case "$vcs" in
        git|git-worktree)
            sandbox_exec_capture "$sandbox_name" "cd /workspace && git status --short"
            ;;
        jj)
            sandbox_exec_capture "$sandbox_name" "cd /workspace && jj status"
            ;;
    esac
}

# Commit changes inside the sandbox
sandbox_vcs_commit() {
    local sandbox_name="$1"
    local vcs="$2"
    local message="$3"

    case "$vcs" in
        git|git-worktree)
            sandbox_exec "$sandbox_name" "cd /workspace && git add -A && git commit -m '$message'"
            ;;
        jj)
            sandbox_exec "$sandbox_name" "cd /workspace && jj commit -m '$message'"
            ;;
    esac
}

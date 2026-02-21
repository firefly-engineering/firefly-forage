#!/usr/bin/env bash
# Prerequisite checking for integration tests
#
# This library provides functions to check if required tools and backends
# are available on the system.

# Source common utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

# Check if forage-ctl is available
check_forage_ctl() {
    if ! command_exists forage-ctl; then
        return 1
    fi
    return 0
}

# Check if a specific backend is available
check_backend() {
    local backend="$1"

    case "$backend" in
        nspawn)
            check_nspawn_backend
            ;;
        docker)
            check_docker_backend
            ;;
        podman)
            check_podman_backend
            ;;
        apple)
            check_apple_backend
            ;;
        *)
            log_error "Unknown backend: $backend"
            return 1
            ;;
    esac
}

check_nspawn_backend() {
    # nspawn requires NixOS with extra-container
    if [[ ! -f /etc/NIXOS ]]; then
        return 1
    fi
    if ! command_exists extra-container; then
        return 1
    fi
    if ! command_exists machinectl; then
        return 1
    fi
    return 0
}

check_docker_backend() {
    if ! command_exists docker; then
        return 1
    fi
    # Check if docker daemon is running
    if ! docker info >/dev/null 2>&1; then
        return 1
    fi
    return 0
}

check_podman_backend() {
    if ! command_exists podman; then
        return 1
    fi
    # Basic check that podman works
    if ! podman info >/dev/null 2>&1; then
        return 1
    fi
    return 0
}

check_apple_backend() {
    # Apple backend only on macOS
    if [[ "$(uname)" != "Darwin" ]]; then
        return 1
    fi
    # Check for container CLI (Apple's virtualization)
    if ! command_exists container; then
        return 1
    fi
    return 0
}

# Check if a specific VCS is available
check_vcs() {
    local vcs="$1"

    case "$vcs" in
        git|git-worktree)
            check_git_vcs
            ;;
        jj)
            check_jj_vcs
            ;;
        *)
            log_error "Unknown VCS: $vcs"
            return 1
            ;;
    esac
}

check_git_vcs() {
    if ! command_exists git; then
        return 1
    fi
    return 0
}

check_jj_vcs() {
    if ! command_exists jj; then
        return 1
    fi
    # jj also requires git for some operations
    if ! command_exists git; then
        return 1
    fi
    return 0
}

# Check all prerequisites for a test
check_all_prerequisites() {
    local backend="$1"
    local vcs="$2"

    log_info "Checking prerequisites for backend=$backend, vcs=$vcs"

    if ! check_forage_ctl; then
        test_skip "forage-ctl not found"
    fi

    if ! check_backend "$backend"; then
        test_skip "Backend '$backend' not available on this system"
    fi

    if ! check_vcs "$vcs"; then
        test_skip "VCS '$vcs' not available on this system"
    fi

    log_info "All prerequisites satisfied"
}

# Get the runtime flag for forage-ctl based on backend
get_runtime_flag() {
    local backend="$1"
    case "$backend" in
        nspawn)
            echo "--runtime=nspawn"
            ;;
        docker)
            echo "--runtime=docker"
            ;;
        podman)
            echo "--runtime=podman"
            ;;
        apple)
            echo "--runtime=apple"
            ;;
    esac
}

# Get the workspace mode flag for forage-ctl based on VCS
get_workspace_mode_flag() {
    local vcs="$1"
    case "$vcs" in
        git|git-worktree)
            echo "--workspace-mode=git-worktree"
            ;;
        jj)
            echo "--workspace-mode=jj"
            ;;
        *)
            echo "--workspace-mode=direct"
            ;;
    esac
}

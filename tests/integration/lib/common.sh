#!/usr/bin/env bash
# Common utilities for integration tests
#
# This library provides core utilities for test output, logging, and control flow.

set -euo pipefail

# Colors for output (disabled if not a terminal)
if [[ -t 1 ]]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[0;33m'
    BLUE='\033[0;34m'
    NC='\033[0m' # No Color
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    NC=''
fi

# Test state
TEST_NAME="${TEST_NAME:-unknown}"
TEST_FAILED=0
TEST_SKIPPED=0
CLEANUP_ITEMS=()

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[PASS]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_error() {
    echo -e "${RED}[FAIL]${NC} $*"
}

log_skip() {
    echo -e "${YELLOW}[SKIP]${NC} $*"
}

# Test lifecycle functions
test_start() {
    local name="$1"
    TEST_NAME="$name"
    TEST_FAILED=0
    TEST_SKIPPED=0
    CLEANUP_ITEMS=()
    echo ""
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}TEST: ${name}${NC}"
    echo -e "${BLUE}========================================${NC}"
}

test_skip() {
    local reason="$1"
    TEST_SKIPPED=1
    log_skip "$TEST_NAME: $reason"
    exit 0
}

test_fail() {
    local reason="$1"
    TEST_FAILED=1
    log_error "$TEST_NAME: $reason"
}

test_end() {
    # Run cleanup
    run_cleanup

    if [[ $TEST_SKIPPED -eq 1 ]]; then
        exit 0
    elif [[ $TEST_FAILED -eq 1 ]]; then
        log_error "$TEST_NAME: FAILED"
        exit 1
    else
        log_success "$TEST_NAME: PASSED"
        exit 0
    fi
}

# Cleanup registration and execution
register_cleanup() {
    local item="$1"
    CLEANUP_ITEMS+=("$item")
}

run_cleanup() {
    log_info "Running cleanup..."
    # Run cleanup in reverse order
    for ((i=${#CLEANUP_ITEMS[@]}-1; i>=0; i--)); do
        local item="${CLEANUP_ITEMS[$i]}"
        log_info "  Cleaning up: $item"
        eval "$item" || log_warn "Cleanup failed: $item"
    done
}

# Temporary directory management
create_temp_dir() {
    local prefix="${1:-forage-test}"
    local dir
    dir=$(mktemp -d "/tmp/${prefix}.XXXXXX")
    register_cleanup "rm -rf '$dir'"
    echo "$dir"
}

# Command execution with output capture
run_cmd() {
    local description="$1"
    shift
    log_info "$description"
    if ! "$@"; then
        test_fail "Command failed: $*"
        return 1
    fi
}

run_cmd_quiet() {
    local description="$1"
    shift
    log_info "$description"
    if ! "$@" >/dev/null 2>&1; then
        test_fail "Command failed: $*"
        return 1
    fi
}

# Check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

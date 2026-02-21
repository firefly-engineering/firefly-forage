#!/usr/bin/env bash
# Assertion helpers for integration tests
#
# This library provides assertion functions for verifying test conditions.

# Source common utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

# Assert that a condition is true
assert_true() {
    local description="$1"
    shift
    local condition="$*"

    if eval "$condition"; then
        log_success "ASSERT: $description"
        return 0
    else
        test_fail "ASSERT FAILED: $description"
        return 1
    fi
}

# Assert that a condition is false
assert_false() {
    local description="$1"
    shift
    local condition="$*"

    if ! eval "$condition"; then
        log_success "ASSERT: $description"
        return 0
    else
        test_fail "ASSERT FAILED: $description"
        return 1
    fi
}

# Assert that two values are equal
assert_equals() {
    local description="$1"
    local expected="$2"
    local actual="$3"

    if [[ "$expected" == "$actual" ]]; then
        log_success "ASSERT: $description"
        return 0
    else
        test_fail "ASSERT FAILED: $description (expected: '$expected', actual: '$actual')"
        return 1
    fi
}

# Assert that a value contains a substring
assert_contains() {
    local description="$1"
    local haystack="$2"
    local needle="$3"

    if [[ "$haystack" == *"$needle"* ]]; then
        log_success "ASSERT: $description"
        return 0
    else
        test_fail "ASSERT FAILED: $description (expected to contain: '$needle')"
        return 1
    fi
}

# Assert that a value does not contain a substring
assert_not_contains() {
    local description="$1"
    local haystack="$2"
    local needle="$3"

    if [[ "$haystack" != *"$needle"* ]]; then
        log_success "ASSERT: $description"
        return 0
    else
        test_fail "ASSERT FAILED: $description (expected NOT to contain: '$needle')"
        return 1
    fi
}

# Assert that a file exists
assert_file_exists() {
    local description="$1"
    local filepath="$2"

    if [[ -f "$filepath" ]]; then
        log_success "ASSERT: $description"
        return 0
    else
        test_fail "ASSERT FAILED: $description (file does not exist: $filepath)"
        return 1
    fi
}

# Assert that a file does not exist
assert_file_not_exists() {
    local description="$1"
    local filepath="$2"

    if [[ ! -f "$filepath" ]]; then
        log_success "ASSERT: $description"
        return 0
    else
        test_fail "ASSERT FAILED: $description (file exists: $filepath)"
        return 1
    fi
}

# Assert that a directory exists
assert_dir_exists() {
    local description="$1"
    local dirpath="$2"

    if [[ -d "$dirpath" ]]; then
        log_success "ASSERT: $description"
        return 0
    else
        test_fail "ASSERT FAILED: $description (directory does not exist: $dirpath)"
        return 1
    fi
}

# Assert that a file contains specific content
assert_file_contains() {
    local description="$1"
    local filepath="$2"
    local expected_content="$3"

    if [[ ! -f "$filepath" ]]; then
        test_fail "ASSERT FAILED: $description (file does not exist: $filepath)"
        return 1
    fi

    local content
    content=$(cat "$filepath")

    if [[ "$content" == *"$expected_content"* ]]; then
        log_success "ASSERT: $description"
        return 0
    else
        test_fail "ASSERT FAILED: $description (file does not contain: '$expected_content')"
        return 1
    fi
}

# Assert that a command succeeds
assert_command_succeeds() {
    local description="$1"
    shift
    local cmd="$*"

    if eval "$cmd" >/dev/null 2>&1; then
        log_success "ASSERT: $description"
        return 0
    else
        test_fail "ASSERT FAILED: $description (command failed: $cmd)"
        return 1
    fi
}

# Assert that a command fails
assert_command_fails() {
    local description="$1"
    shift
    local cmd="$*"

    if ! eval "$cmd" >/dev/null 2>&1; then
        log_success "ASSERT: $description"
        return 0
    else
        test_fail "ASSERT FAILED: $description (command succeeded but should have failed: $cmd)"
        return 1
    fi
}

# Assert that command output contains a string
assert_output_contains() {
    local description="$1"
    local expected="$2"
    shift 2
    local cmd="$*"

    local output
    output=$(eval "$cmd" 2>&1) || true

    if [[ "$output" == *"$expected"* ]]; then
        log_success "ASSERT: $description"
        return 0
    else
        test_fail "ASSERT FAILED: $description (output does not contain: '$expected')"
        log_info "Actual output: $output"
        return 1
    fi
}

#!/usr/bin/env bash
# Run all integration tests
#
# This script runs all integration tests for available backend/VCS combinations.
# Tests that cannot run on the current system (missing prerequisites) will be
# skipped automatically.
#
# Usage:
#   ./run-all.sh              # Run all tests
#   ./run-all.sh --parallel   # Run tests in parallel
#   ./run-all.sh docker       # Run only docker tests
#   ./run-all.sh jj           # Run only jj VCS tests

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors
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

# Test results tracking
declare -A RESULTS
PASSED=0
FAILED=0
SKIPPED=0

# Parse arguments
FILTER="${1:-}"
PARALLEL=false

if [[ "$FILTER" == "--parallel" ]]; then
    PARALLEL=true
    FILTER="${2:-}"
fi

# Find all test scripts
find_tests() {
    local filter="$1"

    for test_script in "${SCRIPT_DIR}"/test-*.sh; do
        if [[ -f "$test_script" ]]; then
            local name
            name=$(basename "$test_script" .sh)

            # Apply filter if specified
            if [[ -n "$filter" ]]; then
                if [[ "$name" != *"$filter"* ]]; then
                    continue
                fi
            fi

            echo "$test_script"
        fi
    done
}

# Run a single test and capture result
run_test() {
    local test_script="$1"
    local name
    name=$(basename "$test_script" .sh)

    echo -e "${BLUE}Running: ${name}${NC}"

    local start_time
    start_time=$(date +%s)

    local exit_code=0
    local output
    output=$("$test_script" 2>&1) || exit_code=$?

    local end_time
    end_time=$(date +%s)
    local duration=$((end_time - start_time))

    if [[ $exit_code -eq 0 ]]; then
        # Check if it was skipped (look for SKIP in output)
        if echo "$output" | grep -q "\[SKIP\]"; then
            RESULTS[$name]="SKIPPED"
            ((SKIPPED++))
            echo -e "${YELLOW}  SKIPPED${NC} (${duration}s)"
            # Show skip reason
            echo "$output" | grep "\[SKIP\]" | head -1 | sed 's/^/    /'
        else
            RESULTS[$name]="PASSED"
            ((PASSED++))
            echo -e "${GREEN}  PASSED${NC} (${duration}s)"
        fi
    else
        RESULTS[$name]="FAILED"
        ((FAILED++))
        echo -e "${RED}  FAILED${NC} (${duration}s)"
        # Show failure details
        echo "$output" | tail -20 | sed 's/^/    /'
    fi
}

# Run tests in parallel
run_tests_parallel() {
    local tests=("$@")
    local pids=()

    for test_script in "${tests[@]}"; do
        (
            run_test "$test_script"
        ) &
        pids+=($!)
    done

    # Wait for all tests
    for pid in "${pids[@]}"; do
        wait "$pid" || true
    done
}

# Run tests sequentially
run_tests_sequential() {
    local tests=("$@")

    for test_script in "${tests[@]}"; do
        run_test "$test_script"
    done
}

# Main
main() {
    echo -e "${BLUE}=======================================${NC}"
    echo -e "${BLUE}Forage Integration Test Suite${NC}"
    echo -e "${BLUE}=======================================${NC}"
    echo ""

    if [[ -n "$FILTER" ]]; then
        echo -e "Filter: ${FILTER}"
    fi
    echo -e "Parallel: ${PARALLEL}"
    echo ""

    # Find tests
    local tests=()
    while IFS= read -r test; do
        tests+=("$test")
    done < <(find_tests "$FILTER")

    if [[ ${#tests[@]} -eq 0 ]]; then
        echo -e "${YELLOW}No tests found${NC}"
        exit 0
    fi

    echo -e "Found ${#tests[@]} test(s)"
    echo ""

    # Run tests
    if [[ "$PARALLEL" == "true" ]]; then
        run_tests_parallel "${tests[@]}"
    else
        run_tests_sequential "${tests[@]}"
    fi

    # Summary
    echo ""
    echo -e "${BLUE}=======================================${NC}"
    echo -e "${BLUE}Summary${NC}"
    echo -e "${BLUE}=======================================${NC}"
    echo -e "${GREEN}Passed:  ${PASSED}${NC}"
    echo -e "${RED}Failed:  ${FAILED}${NC}"
    echo -e "${YELLOW}Skipped: ${SKIPPED}${NC}"
    echo ""

    # Detailed results
    echo "Results by test:"
    for name in "${!RESULTS[@]}"; do
        local result="${RESULTS[$name]}"
        case "$result" in
            PASSED)
                echo -e "  ${GREEN}PASS${NC} $name"
                ;;
            FAILED)
                echo -e "  ${RED}FAIL${NC} $name"
                ;;
            SKIPPED)
                echo -e "  ${YELLOW}SKIP${NC} $name"
                ;;
        esac
    done | sort

    # Exit with failure if any test failed
    if [[ $FAILED -gt 0 ]]; then
        exit 1
    fi
}

main

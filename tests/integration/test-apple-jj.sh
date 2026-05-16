#!/usr/bin/env bash
# Integration test: apple backend + jj VCS
#
# This test verifies the full sandbox workflow using:
# - Container backend: Apple (macOS virtualization)
# - VCS mode: jj (jujutsu)
#
# Prerequisites:
# - macOS system
# - Apple's container CLI available
# - jj and git installed
# - forage-ctl configured with a 'test' template

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/scenarios.sh"

# Run the full integration scenario
run_scenario "apple" "jj" scenario_full_integration "apple-jj-integration"

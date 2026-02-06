#!/usr/bin/env bash
# Integration test: nspawn backend + jj VCS
#
# This test verifies the full sandbox workflow using:
# - Container backend: systemd-nspawn (via extra-container)
# - VCS mode: jj (jujutsu)
#
# Prerequisites:
# - NixOS system with extra-container
# - jj and git installed
# - forage-ctl configured with a 'test' template

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/scenarios.sh"

# Run the full integration scenario
run_scenario "nspawn" "jj" scenario_full_integration "nspawn-jj-integration"

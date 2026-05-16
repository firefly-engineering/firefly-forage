#!/usr/bin/env bash
# Integration test: podman backend + git-worktree VCS
#
# This test verifies the full sandbox workflow using:
# - Container backend: Podman
# - VCS mode: git-worktree
#
# Prerequisites:
# - Podman installed and working
# - git installed
# - forage-ctl configured with a 'test' template

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/scenarios.sh"

# Run the full integration scenario
run_scenario "podman" "git" scenario_full_integration "podman-git-integration"

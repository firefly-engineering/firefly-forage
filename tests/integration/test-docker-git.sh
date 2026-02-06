#!/usr/bin/env bash
# Integration test: docker backend + git-worktree VCS
#
# This test verifies the full sandbox workflow using:
# - Container backend: Docker
# - VCS mode: git-worktree
#
# Prerequisites:
# - Docker installed and running
# - git installed
# - forage-ctl configured with a 'test' template

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/scenarios.sh"

# Run the full integration scenario
run_scenario "docker" "git" scenario_full_integration "docker-git-integration"

#!/usr/bin/env bash
# VCS helpers for integration tests
#
# This library provides functions to create and manage test repositories
# using different version control systems.

# Source common utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

# Ensure git is configured for commits
ensure_git_config() {
    if [[ -z "$(git config --global user.email 2>/dev/null || true)" ]]; then
        git config --global user.email "test@forage-integration.local"
    fi
    if [[ -z "$(git config --global user.name 2>/dev/null || true)" ]]; then
        git config --global user.name "Forage Integration Test"
    fi
}

# Create a test repository
# Returns the path to the created repository
create_test_repo() {
    local vcs="$1"
    local name="${2:-test-project}"

    local repo_dir
    repo_dir=$(create_temp_dir "forage-repo")
    repo_dir="${repo_dir}/${name}"
    mkdir -p "$repo_dir"

    case "$vcs" in
        git|git-worktree)
            create_git_repo "$repo_dir"
            ;;
        jj)
            create_jj_repo "$repo_dir"
            ;;
        *)
            log_error "Unknown VCS: $vcs"
            return 1
            ;;
    esac

    echo "$repo_dir"
}

create_git_repo() {
    local repo_dir="$1"

    ensure_git_config

    log_info "Creating git repository at $repo_dir"

    (
        cd "$repo_dir"
        git init -q
        echo "# Test Project" > README.md
        echo "Created for forage integration testing" >> README.md
        git add README.md
        git commit -q -m "Initial commit"
    )

    log_info "Git repository created with initial commit"
}

create_jj_repo() {
    local repo_dir="$1"

    ensure_git_config

    log_info "Creating jj repository at $repo_dir"

    (
        cd "$repo_dir"
        jj git init --quiet
        echo "# Test Project" > README.md
        echo "Created for forage integration testing" >> README.md
        jj commit -m "Initial commit"
    )

    log_info "JJ repository created with initial commit"
}

# Make a change in the repository (outside sandbox)
repo_add_file() {
    local repo_dir="$1"
    local filename="$2"
    local content="$3"

    echo "$content" > "${repo_dir}/${filename}"
}

# Commit changes in the repository
repo_commit() {
    local repo_dir="$1"
    local vcs="$2"
    local message="$3"

    (
        cd "$repo_dir"
        case "$vcs" in
            git|git-worktree)
                git add -A
                git commit -q -m "$message"
                ;;
            jj)
                jj commit -m "$message"
                ;;
        esac
    )
}

# Get the current commit/change ID
repo_get_head() {
    local repo_dir="$1"
    local vcs="$2"

    (
        cd "$repo_dir"
        case "$vcs" in
            git|git-worktree)
                git rev-parse HEAD
                ;;
            jj)
                jj log --no-graph -r @ -T 'change_id' | head -1
                ;;
        esac
    )
}

# Check if a file exists in the repository
repo_file_exists() {
    local repo_dir="$1"
    local filename="$2"

    [[ -f "${repo_dir}/${filename}" ]]
}

# Get the content of a file in the repository
repo_get_file_content() {
    local repo_dir="$1"
    local filename="$2"

    cat "${repo_dir}/${filename}"
}

# List files in the repository
repo_list_files() {
    local repo_dir="$1"

    ls -la "$repo_dir"
}

# Get the VCS status
repo_status() {
    local repo_dir="$1"
    local vcs="$2"

    (
        cd "$repo_dir"
        case "$vcs" in
            git|git-worktree)
                git status --short
                ;;
            jj)
                jj status
                ;;
        esac
    )
}

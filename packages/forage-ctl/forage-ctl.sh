#!/usr/bin/env bash
# forage-ctl - Firefly Forage sandbox management CLI

set -euo pipefail

# Configuration
FORAGE_CONFIG_DIR="${FORAGE_CONFIG_DIR:-/etc/firefly-forage}"
FORAGE_STATE_DIR="${FORAGE_STATE_DIR:-/var/lib/firefly-forage}"
FORAGE_CONTAINER_PREFIX="forage-"

# Colors (disabled if not a terminal)
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

# Logging functions
log_info() { echo -e "${BLUE}ℹ${NC} $*"; }
log_success() { echo -e "${GREEN}✓${NC} $*"; }
log_warning() { echo -e "${YELLOW}⚠${NC} $*" >&2; }
log_error() { echo -e "${RED}✗${NC} $*" >&2; }

# Print usage
usage() {
    cat <<EOF
Usage: forage-ctl <command> [options]

Commands:
    templates           List available sandbox templates
    up <name>          Create and start a sandbox
    down <name>        Stop and remove a sandbox
    ps                 List running sandboxes
    ssh <name>         Connect to a sandbox via SSH
    ssh-cmd <name>     Print SSH command for a sandbox
    exec <name> -- <cmd>  Execute command in sandbox
    reset <name>       Reset sandbox (restart with fresh state)
    help               Show this help message

Options for 'up':
    --template, -t <name>    Template to use (required)
    --workspace, -w <path>   Workspace directory to mount (required)
    --port, -p <port>        Specific port to use (optional)

Examples:
    forage-ctl templates
    forage-ctl up myproject -t claude -w ~/projects/myproject
    forage-ctl ssh myproject
    forage-ctl down myproject

EOF
}

# List available templates
cmd_templates() {
    local templates_dir="${FORAGE_CONFIG_DIR}/templates"

    if [[ ! -d "$templates_dir" ]]; then
        log_error "Templates directory not found: $templates_dir"
        log_info "Is firefly-forage enabled in your NixOS configuration?"
        exit 1
    fi

    printf "%-15s %-20s %-10s %s\n" "TEMPLATE" "AGENTS" "NETWORK" "DESCRIPTION"
    printf "%s\n" "$(printf '%.0s-' {1..70})"

    for template_file in "$templates_dir"/*.json; do
        [[ -f "$template_file" ]] || continue

        local name description network agents
        name=$(jq -r '.name' "$template_file")
        description=$(jq -r '.description // ""' "$template_file")
        network=$(jq -r '.network // "full"' "$template_file")
        agents=$(jq -r '.agents | keys | join(",")' "$template_file")

        printf "%-15s %-20s %-10s %s\n" "$name" "$agents" "$network" "$description"
    done
}

# Get container name from sandbox name
container_name() {
    echo "${FORAGE_CONTAINER_PREFIX}$1"
}

# Check if a sandbox exists
sandbox_exists() {
    local name="$1"
    machinectl show "$(container_name "$name")" &>/dev/null
}

# Get sandbox info
sandbox_info() {
    local name="$1"
    local container
    container=$(container_name "$name")

    if ! machinectl show "$container" &>/dev/null; then
        return 1
    fi

    # Get info from machinectl
    local state
    state=$(machinectl show "$container" -p State --value 2>/dev/null || echo "unknown")

    echo "$state"
}

# Find an available port
find_available_port() {
    local start="${1:-2200}"
    local end="${2:-2299}"

    for port in $(seq "$start" "$end"); do
        if ! ss -tuln | grep -q ":$port "; then
            echo "$port"
            return 0
        fi
    done

    return 1
}

# Create and start a sandbox
cmd_up() {
    local name=""
    local template=""
    local workspace=""
    local port=""

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --template|-t)
                template="$2"
                shift 2
                ;;
            --workspace|-w)
                workspace="$2"
                shift 2
                ;;
            --port|-p)
                port="$2"
                shift 2
                ;;
            -*)
                log_error "Unknown option: $1"
                exit 1
                ;;
            *)
                if [[ -z "$name" ]]; then
                    name="$1"
                else
                    log_error "Unexpected argument: $1"
                    exit 1
                fi
                shift
                ;;
        esac
    done

    # Validate arguments
    if [[ -z "$name" ]]; then
        log_error "Sandbox name is required"
        usage
        exit 1
    fi

    if [[ -z "$template" ]]; then
        log_error "Template is required (--template)"
        exit 1
    fi

    if [[ -z "$workspace" ]]; then
        log_error "Workspace is required (--workspace)"
        exit 1
    fi

    # Resolve workspace to absolute path
    workspace=$(realpath "$workspace")

    if [[ ! -d "$workspace" ]]; then
        log_error "Workspace directory does not exist: $workspace"
        exit 1
    fi

    # Check template exists
    local template_file="${FORAGE_CONFIG_DIR}/templates/${template}.json"
    if [[ ! -f "$template_file" ]]; then
        log_error "Template not found: $template"
        log_info "Available templates:"
        cmd_templates
        exit 3
    fi

    # Check if sandbox already exists
    if sandbox_exists "$name"; then
        log_error "Sandbox already exists: $name"
        log_info "Use 'forage-ctl down $name' to remove it first"
        exit 1
    fi

    # Find available port if not specified
    if [[ -z "$port" ]]; then
        port=$(find_available_port) || {
            log_error "No available ports in range"
            exit 4
        }
    fi

    local container
    container=$(container_name "$name")

    log_info "Creating sandbox '$name' from template '$template'"
    log_info "Workspace: $workspace → /workspace"
    log_info "SSH port: $port"

    # TODO: Implement actual container creation
    # This will use nixos-container or systemd-nspawn directly
    # For now, this is a placeholder

    log_warning "Container creation not yet implemented"
    log_info "This is a placeholder - actual implementation coming soon"

    # The implementation will:
    # 1. Generate a NixOS container config from the template
    # 2. Create the container with appropriate bind mounts
    # 3. Start the container
    # 4. Configure SSH port forwarding

    log_success "Sandbox '$name' created (port $port)"
}

# Stop and remove a sandbox
cmd_down() {
    local name="$1"
    local all=false

    if [[ "$name" == "--all" ]]; then
        all=true
    fi

    if $all; then
        log_info "Stopping all sandboxes..."
        # List all forage containers and stop them
        for container in $(machinectl list --no-legend | awk '{print $1}' | grep "^${FORAGE_CONTAINER_PREFIX}"); do
            local sandbox_name="${container#$FORAGE_CONTAINER_PREFIX}"
            log_info "Stopping $sandbox_name..."
            machinectl terminate "$container" 2>/dev/null || true
        done
        log_success "All sandboxes stopped"
    else
        if [[ -z "$name" ]]; then
            log_error "Sandbox name is required"
            exit 1
        fi

        local container
        container=$(container_name "$name")

        if ! sandbox_exists "$name"; then
            log_error "Sandbox not found: $name"
            exit 2
        fi

        log_info "Stopping sandbox '$name'..."
        machinectl terminate "$container" 2>/dev/null || true

        # TODO: Clean up any additional resources

        log_success "Sandbox '$name' stopped"
    fi
}

# List running sandboxes
cmd_ps() {
    printf "%-15s %-12s %-6s %-35s %s\n" "NAME" "TEMPLATE" "PORT" "WORKSPACE" "STATUS"
    printf "%s\n" "$(printf '%.0s-' {1..80})"

    # List all forage containers
    while IFS= read -r line; do
        [[ -z "$line" ]] && continue

        local container state
        container=$(echo "$line" | awk '{print $1}')
        state=$(echo "$line" | awk '{print $3}')

        # Skip non-forage containers
        [[ "$container" != ${FORAGE_CONTAINER_PREFIX}* ]] && continue

        local name="${container#$FORAGE_CONTAINER_PREFIX}"

        # TODO: Get template and workspace from container metadata
        local template="unknown"
        local port="?"
        local workspace="?"

        printf "%-15s %-12s %-6s %-35s %s\n" "$name" "$template" "$port" "$workspace" "$state"
    done < <(machinectl list --no-legend 2>/dev/null || true)
}

# Connect to sandbox via SSH
cmd_ssh() {
    local name="$1"

    if [[ -z "$name" ]]; then
        log_error "Sandbox name is required"
        exit 1
    fi

    if ! sandbox_exists "$name"; then
        log_error "Sandbox not found: $name"
        exit 2
    fi

    # TODO: Get port from container metadata
    local port="2200"  # Placeholder

    log_info "Connecting to sandbox '$name' on port $port..."
    exec ssh -p "$port" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null agent@localhost
}

# Print SSH command
cmd_ssh_cmd() {
    local name="$1"

    if [[ -z "$name" ]]; then
        log_error "Sandbox name is required"
        exit 1
    fi

    if ! sandbox_exists "$name"; then
        log_error "Sandbox not found: $name"
        exit 2
    fi

    # TODO: Get port and hostname from container metadata
    local port="2200"  # Placeholder
    local hostname
    hostname=$(hostname)

    echo "ssh -p $port -o StrictHostKeyChecking=no agent@$hostname"
}

# Execute command in sandbox
cmd_exec() {
    local name="$1"
    shift

    if [[ -z "$name" ]]; then
        log_error "Sandbox name is required"
        exit 1
    fi

    # Skip the -- separator if present
    if [[ "${1:-}" == "--" ]]; then
        shift
    fi

    if [[ $# -eq 0 ]]; then
        log_error "Command is required"
        exit 1
    fi

    local container
    container=$(container_name "$name")

    if ! sandbox_exists "$name"; then
        log_error "Sandbox not found: $name"
        exit 2
    fi

    exec machinectl shell "$container" /bin/bash -c "$*"
}

# Reset sandbox
cmd_reset() {
    local name="$1"

    if [[ -z "$name" ]]; then
        log_error "Sandbox name is required"
        exit 1
    fi

    if ! sandbox_exists "$name"; then
        log_error "Sandbox not found: $name"
        exit 2
    fi

    log_info "Resetting sandbox '$name'..."

    # TODO: Get template and workspace from container metadata
    # For now, just restart the container
    local container
    container=$(container_name "$name")

    machinectl terminate "$container" 2>/dev/null || true
    sleep 1
    # TODO: Restart with same config

    log_success "Sandbox '$name' reset"
}

# Main entry point
main() {
    if [[ $# -eq 0 ]]; then
        usage
        exit 0
    fi

    local cmd="$1"
    shift

    case "$cmd" in
        templates)
            cmd_templates
            ;;
        up)
            cmd_up "$@"
            ;;
        down)
            cmd_down "$@"
            ;;
        ps|list)
            cmd_ps
            ;;
        ssh)
            cmd_ssh "$@"
            ;;
        ssh-cmd)
            cmd_ssh_cmd "$@"
            ;;
        exec)
            cmd_exec "$@"
            ;;
        reset)
            cmd_reset "$@"
            ;;
        help|--help|-h)
            usage
            ;;
        *)
            log_error "Unknown command: $cmd"
            usage
            exit 1
            ;;
    esac
}

main "$@"

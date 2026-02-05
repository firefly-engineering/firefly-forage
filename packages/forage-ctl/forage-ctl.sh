#!/usr/bin/env bash
# forage-ctl - Firefly Forage sandbox management CLI

set -euo pipefail

# Configuration
FORAGE_CONFIG_DIR="${FORAGE_CONFIG_DIR:-/etc/firefly-forage}"
FORAGE_STATE_DIR="${FORAGE_STATE_DIR:-/var/lib/firefly-forage}"
FORAGE_CONTAINER_PREFIX="forage-"
FORAGE_SANDBOXES_DIR="${FORAGE_STATE_DIR}/sandboxes"
FORAGE_WORKSPACES_DIR="${FORAGE_STATE_DIR}/workspaces"
FORAGE_SECRETS_DIR="/run/forage-secrets"

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
    ssh <name>         Connect to a sandbox via SSH (attaches to tmux)
    ssh-cmd <name>     Print SSH command for a sandbox
    exec <name> -- <cmd>  Execute command in sandbox
    start <name> [agent]  Start an agent in the sandbox tmux session
    shell <name>       Open a shell in a new tmux window
    logs <name>        Show container logs
    reset <name>       Reset sandbox (restart with fresh state)
    help               Show this help message

Options for 'up':
    --template, -t <name>    Template to use (required)
    --workspace, -w <path>   Workspace directory to mount (mutually exclusive with --repo)
    --repo, -r <path>        JJ repository to create workspace from (mutually exclusive with --workspace)
    --port, -p <port>        Specific port to use (optional)

Examples:
    forage-ctl templates
    forage-ctl up myproject -t claude -w ~/projects/myproject
    forage-ctl up agent-a -t claude --repo ~/projects/myrepo
    forage-ctl up agent-b -t claude --repo ~/projects/myrepo  # Same repo, different workspace
    forage-ctl ssh myproject
    forage-ctl down myproject

EOF
}

# Read host configuration
read_host_config() {
    local config_file="${FORAGE_CONFIG_DIR}/config.json"
    if [[ ! -f "$config_file" ]]; then
        log_error "Host configuration not found: $config_file"
        log_info "Is firefly-forage enabled in your NixOS configuration?"
        exit 1
    fi
    cat "$config_file"
}

# Get value from host config
get_config() {
    local key="$1"
    read_host_config | jq -r "$key"
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

# Get metadata file path
metadata_file() {
    echo "${FORAGE_SANDBOXES_DIR}/$1.json"
}

# Read sandbox metadata
read_metadata() {
    local name="$1"
    local meta_file
    meta_file=$(metadata_file "$name")
    if [[ -f "$meta_file" ]]; then
        cat "$meta_file"
    else
        return 1
    fi
}

# Write sandbox metadata
write_metadata() {
    local name="$1"
    local data="$2"
    local meta_file
    meta_file=$(metadata_file "$name")
    mkdir -p "$(dirname "$meta_file")"
    echo "$data" > "$meta_file"
}

# Delete sandbox metadata
delete_metadata() {
    local name="$1"
    local meta_file
    meta_file=$(metadata_file "$name")
    rm -f "$meta_file"
}

# Check if a sandbox exists (via metadata)
sandbox_exists() {
    local name="$1"
    [[ -f "$(metadata_file "$name")" ]]
}

# Check if container is running
container_running() {
    local name="$1"
    local container
    container=$(container_name "$name")
    machinectl show "$container" &>/dev/null
}

# Get sandbox status
sandbox_status() {
    local name="$1"
    if ! sandbox_exists "$name"; then
        echo "not-found"
        return
    fi

    if container_running "$name"; then
        echo "running"
    else
        echo "stopped"
    fi
}

# Find an available port
find_available_port() {
    local start="${1:-2200}"
    local end="${2:-2299}"

    # Also check existing metadata to avoid conflicts
    local used_ports=()
    for meta_file in "${FORAGE_SANDBOXES_DIR}"/*.json; do
        [[ -f "$meta_file" ]] || continue
        used_ports+=("$(jq -r '.port' "$meta_file")")
    done

    for port in $(seq "$start" "$end"); do
        # Check if port is in use by OS
        if ss -tuln | grep -q ":$port "; then
            continue
        fi
        # Check if port is reserved by another sandbox
        local reserved=false
        for used in "${used_ports[@]}"; do
            if [[ "$port" == "$used" ]]; then
                reserved=true
                break
            fi
        done
        if ! $reserved; then
            echo "$port"
            return 0
        fi
    done

    return 1
}

# Find an available network slot
find_available_network_slot() {
    local used_slots=()
    for meta_file in "${FORAGE_SANDBOXES_DIR}"/*.json; do
        [[ -f "$meta_file" ]] || continue
        used_slots+=("$(jq -r '.networkSlot' "$meta_file")")
    done

    for slot in $(seq 1 200); do
        local reserved=false
        for used in "${used_slots[@]}"; do
            if [[ "$slot" == "$used" ]]; then
                reserved=true
                break
            fi
        done
        if ! $reserved; then
            echo "$slot"
            return 0
        fi
    done

    return 1
}

# Get UID/GID for configured user
get_user_uid() {
    local username="$1"
    id -u "$username" 2>/dev/null || echo "1000"
}

get_user_gid() {
    local username="$1"
    id -g "$username" 2>/dev/null || echo "1000"
}

# Set up secrets for a sandbox
setup_secrets() {
    local name="$1"
    local template_file="$2"

    local secrets_dir="${FORAGE_SECRETS_DIR}/${name}"
    mkdir -p "$secrets_dir"
    chmod 700 "$secrets_dir"

    # Get host secrets configuration
    local host_secrets
    host_secrets=$(get_config '.secrets')

    # Copy secrets needed by template agents
    local agents
    agents=$(jq -r '.agents | keys[]' "$template_file")

    for agent in $agents; do
        local secret_name
        secret_name=$(jq -r ".agents.\"$agent\".secretName" "$template_file")
        local secret_path
        secret_path=$(echo "$host_secrets" | jq -r ".\"$secret_name\" // empty")

        if [[ -n "$secret_path" && -f "$secret_path" ]]; then
            cp "$secret_path" "${secrets_dir}/${secret_name}"
            chmod 400 "${secrets_dir}/${secret_name}"
        else
            log_warning "Secret not found for agent $agent: $secret_name"
        fi
    done
}

# Clean up secrets for a sandbox
cleanup_secrets() {
    local name="$1"
    rm -rf "${FORAGE_SECRETS_DIR:?}/${name:?}"
}

# Check if a path is a jj repository
is_jj_repo() {
    local path="$1"
    [[ -d "${path}/.jj/repo" ]]
}

# Check if a jj workspace name already exists in a repo
jj_workspace_exists() {
    local repo_path="$1"
    local workspace_name="$2"
    jj workspace list -R "$repo_path" 2>/dev/null | grep -q "^${workspace_name}:"
}

# Create a jj workspace for a sandbox
create_jj_workspace() {
    local repo_path="$1"
    local workspace_name="$2"
    local workspace_dir="${FORAGE_WORKSPACES_DIR}/${workspace_name}"

    # Create workspace directory
    mkdir -p "$workspace_dir"

    # Create jj workspace
    if ! jj workspace add "$workspace_dir" --name "$workspace_name" -R "$repo_path"; then
        rm -rf "$workspace_dir"
        return 1
    fi

    echo "$workspace_dir"
}

# Clean up a jj workspace
cleanup_jj_workspace() {
    local repo_path="$1"
    local workspace_name="$2"
    local workspace_dir="${FORAGE_WORKSPACES_DIR}/${workspace_name}"

    # Forget workspace in jj
    jj workspace forget "$workspace_name" -R "$repo_path" 2>/dev/null || true

    # Remove workspace directory
    rm -rf "$workspace_dir"
}

# Inject skills into workspace
inject_skills() {
    local name="$1"
    local workspace="$2"
    local template_file="$3"
    local workspace_mode="${4:-direct}"

    local claude_dir="${workspace}/.claude"
    mkdir -p "$claude_dir"

    # Read template info
    local network allowed_hosts agents_json
    network=$(jq -r '.network // "full"' "$template_file")
    allowed_hosts=$(jq -r '.allowedHosts | join(", ")' "$template_file")
    agents_json=$(jq -r '.agents | keys | join(", ")' "$template_file")

    # Generate skills content
    local network_desc
    case "$network" in
        full) network_desc="Full internet access" ;;
        restricted) network_desc="Restricted to allowed hosts: $allowed_hosts" ;;
        none) network_desc="No network access (air-gapped)" ;;
        *) network_desc="Unknown" ;;
    esac

    # JJ-specific section
    local vcs_section=""
    if [[ "$workspace_mode" == "jj" ]]; then
        vcs_section="
## Version Control: JJ (Jujutsu)

This workspace uses \`jj\` for version control:

\`\`\`bash
jj status         # Show working copy status
jj diff           # Show changes
jj new            # Create new change
jj describe -m \"\" # Set commit message
jj bookmark set   # Update bookmark
\`\`\`

This is an isolated jj workspace - changes don't affect other workspaces.
"
    fi

    cat > "${claude_dir}/forage-skills.md" <<EOF
# Forage Sandbox Skills

You are running inside a Firefly Forage sandbox named \`${name}\`.

## Environment

- **Workspace**: \`/workspace\` (your working directory)
- **Network**: ${network_desc}
- **Session**: tmux session \`forage\` (persistent across reconnections)

## Available Agents

${agents_json:-No agents configured}
${vcs_section}
## Sandbox Constraints

- The root filesystem is ephemeral (tmpfs) - changes outside /workspace are lost on restart
- \`/nix/store\` is read-only (shared from host)
- \`/workspace\` is your persistent working directory
- Secrets are mounted read-only at \`/run/secrets/\`

## Installing Additional Tools

Any tool not pre-installed can be used via Nix. The sandbox has access to the
host's Nix daemon, so you can run any package from nixpkgs:

\`\`\`bash
# Run a tool once
nix run nixpkgs#ripgrep -- --help

# Enter a shell with multiple tools
nix shell nixpkgs#jq nixpkgs#yq

# Build and run a flake
nix run github:owner/repo
\`\`\`

This works because \`/nix/store\` is shared (read-only) and the Nix daemon
handles all builds on the host. New packages appear instantly in the sandbox.

## Tips

- Use \`tmux\` for long-running processes - your session persists across SSH disconnections
- All project work should be done in \`/workspace\`
- The sandbox can be reset with \`forage-ctl reset ${name}\` from the host

## Sub-Agent Spawning

When spawning sub-agents (e.g., with Claude Code's Task tool), be aware:
- Sub-agents share this same sandbox environment
- Use tmux windows/panes for parallel agent work
- Each sub-agent has access to the same workspace and tools

---
*This file is auto-generated by Firefly Forage. Do not edit manually.*
EOF

    log_info "Injected skills to ${claude_dir}/forage-skills.md"
}

# Clean up injected skills
cleanup_skills() {
    local workspace="$1"
    rm -f "${workspace}/.claude/forage-skills.md"
}

# Wait for SSH to become available
wait_for_ssh() {
    local port="$1"
    local timeout="${2:-60}"
    local start_time
    start_time=$(date +%s)

    log_info "Waiting for SSH to become available on port $port..."

    while true; do
        if ssh -o ConnectTimeout=1 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
           -p "$port" agent@localhost true 2>/dev/null; then
            return 0
        fi

        local current_time
        current_time=$(date +%s)
        local elapsed=$((current_time - start_time))

        if [[ $elapsed -ge $timeout ]]; then
            log_error "Timeout waiting for SSH (${timeout}s)"
            return 1
        fi

        sleep 1
    done
}

# Generate container configuration and create with extra-container
create_container() {
    local name="$1"
    local template_file="$2"
    local workspace="$3"
    local port="$4"
    local network_slot="$5"
    local source_repo="${6:-}"  # Optional: source repo for jj mode

    # Get host configuration
    local config_user authorized_keys_json
    config_user=$(get_config '.user')
    authorized_keys_json=$(get_config '.authorizedKeys')

    local host_uid host_gid
    host_uid=$(get_user_uid "$config_user")
    host_gid=$(get_user_gid "$config_user")

    # Convert authorized keys to Nix list format
    local authorized_keys_nix
    authorized_keys_nix=$(echo "$authorized_keys_json" | jq -r '. | map("\"" + . + "\"") | "[ " + join(" ") + " ]"')

    # Read template data
    local template_json
    template_json=$(cat "$template_file")
    # Note: network mode (full/restricted/none) will be used for firewall rules in future
    # local network
    # network=$(echo "$template_json" | jq -r '.network // "full"')

    # Get extra-container path
    local extra_container_cmd
    extra_container_cmd=$(get_config '.extraContainerPath')

    # Container IP based on network slot
    local container_ip="192.168.100.$((network_slot + 10))"
    local host_ip="192.168.100.1"

    # Extra bind mount for jj mode (allows symlink in workspace/.jj/repo to resolve)
    local extra_bind_mount=""
    if [[ -n "$source_repo" ]]; then
        extra_bind_mount="
      \"${source_repo}/.jj\" = {
        hostPath = \"${source_repo}/.jj\";
        isReadOnly = false;
      };"
    fi

    # Build agent packages list
    local agent_packages=""
    for agent in $(echo "$template_json" | jq -r '.agents | keys[]'); do
        local pkg_path auth_env secret_name
        pkg_path=$(echo "$template_json" | jq -r ".agents.\"$agent\".packagePath")
        auth_env=$(echo "$template_json" | jq -r ".agents.\"$agent\".authEnvVar")
        secret_name=$(echo "$template_json" | jq -r ".agents.\"$agent\".secretName")

        # Create wrapper script for agent
        agent_packages+="
          (pkgs.writeShellApplication {
            name = \"$agent\";
            runtimeInputs = [ (builtins.storePath \"$pkg_path\") ];
            text = '''
              if [ -f \"/run/secrets/$secret_name\" ]; then
                export $auth_env=\"\$(cat /run/secrets/$secret_name)\"
              fi
              exec \$(readlink -f $pkg_path/bin/*) \"\$@\"
            ''';
          })"
    done

    # Build extra packages list
    local extra_pkgs=""
    for pkg_path in $(echo "$template_json" | jq -r '.extraPackages[]'); do
        extra_pkgs+=" (builtins.storePath \"$pkg_path\")"
    done

    # Create temporary Nix file for container config
    local config_file
    config_file=$(mktemp --suffix=.nix)

    cat > "$config_file" <<EOF
{ pkgs, lib, ... }:
{
  containers."forage-${name}" = {
    ephemeral = true;
    privateNetwork = true;
    hostAddress = "$host_ip";
    localAddress = "$container_ip";

    forwardPorts = [{
      containerPort = 22;
      hostPort = $port;
      protocol = "tcp";
    }];

    bindMounts = {
      "/nix/store" = {
        hostPath = "/nix/store";
        isReadOnly = true;
      };
      "/workspace" = {
        hostPath = "$workspace";
        isReadOnly = false;
      };
      "/run/secrets" = {
        hostPath = "${FORAGE_SECRETS_DIR}/${name}";
        isReadOnly = true;
      };${extra_bind_mount}
    };

    config = { config, pkgs, ... }: {
      system.stateVersion = "24.11";
      boot.isContainer = true;

      networking = {
        hostName = "forage-${name}";
        firewall.allowedTCPPorts = [ 22 ];
        useHostResolvConf = lib.mkForce true;
      };

      users.users.agent = {
        isNormalUser = true;
        uid = $host_uid;
        group = "agent";
        home = "/home/agent";
        shell = pkgs.bash;
        openssh.authorizedKeys.keys = $authorized_keys_nix;
        extraGroups = [ "wheel" ];
      };

      users.groups.agent.gid = $host_gid;

      services.openssh = {
        enable = true;
        settings = {
          PermitRootLogin = "no";
          PasswordAuthentication = false;
        };
      };

      systemd.services.forage-tmux = {
        description = "Forage tmux session";
        after = [ "multi-user.target" ];
        wantedBy = [ "multi-user.target" ];
        serviceConfig = {
          Type = "forking";
          User = "agent";
          Group = "agent";
          WorkingDirectory = "/workspace";
          ExecStart = "\${pkgs.tmux}/bin/tmux new-session -d -s forage -c /workspace";
          ExecStop = "\${pkgs.tmux}/bin/tmux kill-session -t forage";
          Restart = "on-failure";
          RestartSec = "5s";
        };
      };

      environment.systemPackages = [
        pkgs.tmux
        pkgs.git
        pkgs.curl
        pkgs.vim
        pkgs.coreutils
        pkgs.bash
        $agent_packages
        $extra_pkgs
      ];

      environment.variables.WORKSPACE = "/workspace";

      security.sudo = {
        enable = true;
        wheelNeedsPassword = false;
      };
    };
  };
}
EOF

    log_info "Creating container with extra-container..."

    # Run extra-container to create and start
    if ! sudo "$extra_container_cmd" create --start "$config_file"; then
        log_error "Failed to create container"
        rm -f "$config_file"
        return 1
    fi

    rm -f "$config_file"
    return 0
}

# Create and start a sandbox
cmd_up() {
    local name=""
    local template=""
    local workspace=""
    local repo=""
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
            --repo|-r)
                repo="$2"
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

    # Check for mutually exclusive options
    if [[ -n "$workspace" && -n "$repo" ]]; then
        log_error "--workspace and --repo are mutually exclusive"
        exit 1
    fi

    if [[ -z "$workspace" && -z "$repo" ]]; then
        log_error "Either --workspace or --repo is required"
        exit 1
    fi

    # Variables for jj mode
    local workspace_mode="direct"
    local source_repo=""
    local effective_workspace=""

    if [[ -n "$repo" ]]; then
        # JJ mode
        repo=$(realpath "$repo")

        if [[ ! -d "$repo" ]]; then
            log_error "Repository directory does not exist: $repo"
            exit 1
        fi

        if ! is_jj_repo "$repo"; then
            log_error "Not a jj repository: $repo"
            log_info "Initialize with: jj git init"
            exit 1
        fi

        if jj_workspace_exists "$repo" "$name"; then
            log_error "JJ workspace '$name' already exists in $repo"
            log_info "Use a different sandbox name or remove the existing workspace"
            exit 1
        fi

        workspace_mode="jj"
        source_repo="$repo"

        # Create jj workspace
        log_info "Creating jj workspace '$name' at ${FORAGE_WORKSPACES_DIR}/${name}..."
        effective_workspace=$(create_jj_workspace "$repo" "$name") || {
            log_error "Failed to create jj workspace"
            exit 1
        }
    else
        # Direct workspace mode
        workspace=$(realpath "$workspace")

        if [[ ! -d "$workspace" ]]; then
            log_error "Workspace directory does not exist: $workspace"
            exit 1
        fi

        effective_workspace="$workspace"
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

    # Read port range from config
    local port_from port_to
    port_from=$(get_config '.portRange.from')
    port_to=$(get_config '.portRange.to')

    # Find available port if not specified
    if [[ -z "$port" ]]; then
        port=$(find_available_port "$port_from" "$port_to") || {
            log_error "No available ports in range $port_from-$port_to"
            exit 4
        }
    fi

    # Find available network slot
    local network_slot
    network_slot=$(find_available_network_slot) || {
        log_error "No available network slots"
        exit 4
    }

    log_info "Creating sandbox '$name' from template '$template'"
    if [[ "$workspace_mode" == "jj" ]]; then
        log_info "Mode: jj workspace"
        log_info "Source repo: $source_repo"
    else
        log_info "Mode: direct workspace"
    fi
    log_info "Workspace: $effective_workspace → /workspace"
    log_info "SSH port: $port"
    log_info "Network slot: $network_slot (IP: 192.168.100.$((network_slot + 10)))"

    # Set up secrets
    setup_secrets "$name" "$template_file"

    # Inject skills into workspace
    inject_skills "$name" "$effective_workspace" "$template_file" "$workspace_mode"

    # Create metadata
    local created_at
    created_at=$(date -Iseconds)
    local metadata
    if [[ "$workspace_mode" == "jj" ]]; then
        metadata=$(jq -n \
            --arg name "$name" \
            --arg template "$template" \
            --argjson port "$port" \
            --arg workspace "$effective_workspace" \
            --argjson networkSlot "$network_slot" \
            --arg createdAt "$created_at" \
            --arg workspaceMode "$workspace_mode" \
            --arg sourceRepo "$source_repo" \
            --arg jjWorkspaceName "$name" \
            '{name: $name, template: $template, port: $port, workspace: $workspace, networkSlot: $networkSlot, createdAt: $createdAt, workspaceMode: $workspaceMode, sourceRepo: $sourceRepo, jjWorkspaceName: $jjWorkspaceName}')
    else
        metadata=$(jq -n \
            --arg name "$name" \
            --arg template "$template" \
            --argjson port "$port" \
            --arg workspace "$effective_workspace" \
            --argjson networkSlot "$network_slot" \
            --arg createdAt "$created_at" \
            --arg workspaceMode "$workspace_mode" \
            '{name: $name, template: $template, port: $port, workspace: $workspace, networkSlot: $networkSlot, createdAt: $createdAt, workspaceMode: $workspaceMode}')
    fi

    write_metadata "$name" "$metadata"

    # Create and start container
    if ! create_container "$name" "$template_file" "$effective_workspace" "$port" "$network_slot" "$source_repo"; then
        log_error "Failed to create container"
        delete_metadata "$name"
        cleanup_secrets "$name"
        cleanup_skills "$effective_workspace"
        # Clean up jj workspace if we created one
        if [[ "$workspace_mode" == "jj" ]]; then
            cleanup_jj_workspace "$source_repo" "$name"
        fi
        exit 5
    fi

    # Wait for SSH
    if ! wait_for_ssh "$port" 60; then
        log_warning "SSH did not become available, but container may still be starting"
    fi

    log_success "Sandbox '$name' created successfully"
    log_info "Connect with: forage-ctl ssh $name"
}

# Stop and remove a sandbox
cmd_down() {
    local name="${1:-}"
    local all=false
    local keep_skills=false

    # Parse flags
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --all)
                all=true
                shift
                ;;
            --keep-skills)
                keep_skills=true
                shift
                ;;
            -*)
                log_error "Unknown option: $1"
                exit 1
                ;;
            *)
                name="$1"
                shift
                ;;
        esac
    done

    if $all; then
        log_info "Stopping all sandboxes..."
        for meta_file in "${FORAGE_SANDBOXES_DIR}"/*.json; do
            [[ -f "$meta_file" ]] || continue
            local sandbox_name
            sandbox_name=$(jq -r '.name' "$meta_file")
            log_info "Stopping $sandbox_name..."
            cmd_down "$sandbox_name"
        done
        log_success "All sandboxes stopped"
        return
    fi

    if [[ -z "$name" ]]; then
        log_error "Sandbox name is required"
        exit 1
    fi

    if ! sandbox_exists "$name"; then
        log_error "Sandbox not found: $name"
        exit 2
    fi

    local container
    container=$(container_name "$name")

    # Get extra-container path
    local extra_container_cmd
    extra_container_cmd=$(get_config '.extraContainerPath')

    # Read metadata
    local metadata workspace workspace_mode source_repo jj_workspace_name
    metadata=$(read_metadata "$name")
    workspace=$(echo "$metadata" | jq -r '.workspace')
    workspace_mode=$(echo "$metadata" | jq -r '.workspaceMode // "direct"')
    source_repo=$(echo "$metadata" | jq -r '.sourceRepo // empty')
    jj_workspace_name=$(echo "$metadata" | jq -r '.jjWorkspaceName // empty')

    log_info "Stopping sandbox '$name'..."

    # Destroy container with extra-container
    sudo "$extra_container_cmd" destroy "$container" 2>/dev/null || true

    # Clean up secrets
    cleanup_secrets "$name"

    # Handle workspace cleanup based on mode
    if [[ "$workspace_mode" == "jj" ]]; then
        # Clean up jj workspace
        if [[ -n "$source_repo" && -n "$jj_workspace_name" ]]; then
            log_info "Removing jj workspace '$jj_workspace_name'..."
            cleanup_jj_workspace "$source_repo" "$jj_workspace_name"
        fi
    else
        # Direct mode: optionally clean up skills
        if ! $keep_skills && [[ -n "$workspace" ]]; then
            cleanup_skills "$workspace"
        fi
    fi

    # Remove metadata
    delete_metadata "$name"

    log_success "Sandbox '$name' stopped and removed"
}

# List running sandboxes
cmd_ps() {
    printf "%-15s %-12s %-6s %-5s %-30s %s\n" "NAME" "TEMPLATE" "PORT" "MODE" "WORKSPACE" "STATUS"
    printf "%s\n" "$(printf '%.0s-' {1..85})"

    for meta_file in "${FORAGE_SANDBOXES_DIR}"/*.json; do
        [[ -f "$meta_file" ]] || continue

        local name template port workspace workspace_mode status
        name=$(jq -r '.name' "$meta_file")
        template=$(jq -r '.template' "$meta_file")
        port=$(jq -r '.port' "$meta_file")
        workspace=$(jq -r '.workspace' "$meta_file")
        workspace_mode=$(jq -r '.workspaceMode // "dir"' "$meta_file")

        # Normalize mode display
        if [[ "$workspace_mode" == "direct" ]]; then
            workspace_mode="dir"
        fi

        # Check actual container status
        status=$(sandbox_status "$name")

        # Truncate workspace if too long
        if [[ ${#workspace} -gt 28 ]]; then
            workspace="...${workspace: -25}"
        fi

        printf "%-15s %-12s %-6s %-5s %-30s %s\n" "$name" "$template" "$port" "$workspace_mode" "$workspace" "$status"
    done
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

    # Get port from metadata
    local port
    port=$(read_metadata "$name" | jq -r '.port')

    log_info "Connecting to sandbox '$name' on port $port..."

    # Connect and attach to tmux session
    exec ssh -p "$port" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
        -t agent@localhost 'tmux attach -t forage || tmux new -s forage'
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

    # Get port from metadata
    local port
    port=$(read_metadata "$name" | jq -r '.port')

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

    if ! sandbox_exists "$name"; then
        log_error "Sandbox not found: $name"
        exit 2
    fi

    # Get port from metadata
    local port
    port=$(read_metadata "$name" | jq -r '.port')

    # Execute via SSH
    exec ssh -p "$port" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
        agent@localhost "$@"
}

# Show container logs
cmd_logs() {
    local name=""
    local follow=false
    local lines=100

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -f|--follow)
                follow=true
                shift
                ;;
            -n|--lines)
                lines="$2"
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

    if [[ -z "$name" ]]; then
        log_error "Sandbox name is required"
        exit 1
    fi

    if ! sandbox_exists "$name"; then
        log_error "Sandbox not found: $name"
        exit 2
    fi

    local container
    container=$(container_name "$name")

    local journal_args=("-M" "$container" "-n" "$lines")
    if $follow; then
        journal_args+=("-f")
    fi

    exec journalctl "${journal_args[@]}"
}

# Start an agent in the sandbox tmux session
cmd_start() {
    local name="$1"
    local agent="${2:-}"

    if [[ -z "$name" ]]; then
        log_error "Sandbox name is required"
        exit 1
    fi

    if ! sandbox_exists "$name"; then
        log_error "Sandbox not found: $name"
        exit 2
    fi

    if ! container_running "$name"; then
        log_error "Sandbox is not running: $name"
        exit 1
    fi

    # Get metadata
    local metadata template
    metadata=$(read_metadata "$name")
    template=$(echo "$metadata" | jq -r '.template')

    # If no agent specified, try to find the default one from template
    if [[ -z "$agent" ]]; then
        local template_file="${FORAGE_CONFIG_DIR}/templates/${template}.json"
        agent=$(jq -r '.agents | keys | first' "$template_file")
    fi

    if [[ -z "$agent" || "$agent" == "null" ]]; then
        log_error "No agent specified and no default agent found in template"
        exit 1
    fi

    # Get port from metadata
    local port
    port=$(echo "$metadata" | jq -r '.port')

    log_info "Starting agent '$agent' in sandbox '$name'..."

    # Send the agent command to the tmux session
    ssh -p "$port" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
        agent@localhost "tmux send-keys -t forage '$agent' Enter"

    log_success "Agent '$agent' started"
    log_info "Connect with: forage-ctl ssh $name"
}

# Open a shell in a new tmux window
cmd_shell() {
    local name="$1"

    if [[ -z "$name" ]]; then
        log_error "Sandbox name is required"
        exit 1
    fi

    if ! sandbox_exists "$name"; then
        log_error "Sandbox not found: $name"
        exit 2
    fi

    if ! container_running "$name"; then
        log_error "Sandbox is not running: $name"
        exit 1
    fi

    # Get port from metadata
    local port
    port=$(read_metadata "$name" | jq -r '.port')

    log_info "Opening shell in sandbox '$name'..."

    # Create a new tmux window and attach
    exec ssh -p "$port" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
        -t agent@localhost 'tmux new-window -t forage -c /workspace && tmux attach -t forage'
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

    # Read metadata to preserve settings
    local metadata template workspace port network_slot workspace_mode source_repo
    metadata=$(read_metadata "$name")
    template=$(echo "$metadata" | jq -r '.template')
    workspace=$(echo "$metadata" | jq -r '.workspace')
    port=$(echo "$metadata" | jq -r '.port')
    network_slot=$(echo "$metadata" | jq -r '.networkSlot')
    workspace_mode=$(echo "$metadata" | jq -r '.workspaceMode // "direct"')
    source_repo=$(echo "$metadata" | jq -r '.sourceRepo // empty')

    local template_file="${FORAGE_CONFIG_DIR}/templates/${template}.json"
    local container
    container=$(container_name "$name")

    # Get extra-container path
    local extra_container_cmd
    extra_container_cmd=$(get_config '.extraContainerPath')

    # Destroy existing container
    sudo "$extra_container_cmd" destroy "$container" 2>/dev/null || true

    # Re-setup secrets
    cleanup_secrets "$name"
    setup_secrets "$name" "$template_file"

    # Re-inject skills
    inject_skills "$name" "$workspace" "$template_file" "$workspace_mode"

    # Recreate container
    if ! create_container "$name" "$template_file" "$workspace" "$port" "$network_slot" "$source_repo"; then
        log_error "Failed to recreate container"
        exit 5
    fi

    # Wait for SSH
    if ! wait_for_ssh "$port" 60; then
        log_warning "SSH did not become available, but container may still be starting"
    fi

    log_success "Sandbox '$name' reset successfully"
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
        start)
            cmd_start "$@"
            ;;
        shell)
            cmd_shell "$@"
            ;;
        logs)
            cmd_logs "$@"
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

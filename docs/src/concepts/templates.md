# Templates

Templates are declarative specifications for sandbox environments. They define which agents are available, what packages are installed, and how the sandbox can access the network.

## Template Structure

```nix
services.firefly-forage.templates.<name> = {
  description = "Human-readable description";

  agents = {
    <agent-name> = {
      package = <derivation>;
      secretName = "<secret-key>";
      authEnvVar = "<ENV_VAR_NAME>";
    };
  };

  extraPackages = [ ... ];

  network = "full" | "restricted" | "none";
  allowedHosts = [ ... ];  # for restricted mode

  initCommands = [ ... ];  # commands to run after creation

  workspace.mounts = { ... };   # composable workspace mounts (optional)
  workspace.useBeads = { ... }; # beads overlay shorthand (optional)
};
```

## Components

### Description

A human-readable description shown by `forage-ctl templates`:

```nix
description = "Claude Code with development tools";
```

### Agents

Agents are AI coding tools that will be available in the sandbox. Each agent needs:

| Field | Description |
|-------|-------------|
| `package` | Nix derivation for the agent |
| `secretName` | Key in `services.firefly-forage.secrets` |
| `authEnvVar` | Environment variable for authentication |
| `hostConfigDir` | Host directory to mount for persistent config (optional) |
| `containerConfigDir` | Override container mount point (optional) |
| `hostConfigDirReadOnly` | Mount config dir as read-only (default: `false`) |
| `permissions` | Agent permission rules (optional, see below) |

```nix
agents.claude = {
  package = pkgs.claude-code;
  secretName = "anthropic";
  authEnvVar = "ANTHROPIC_API_KEY";
};
```

Forage creates a wrapper script that:
1. Reads the secret from `/run/secrets/<secretName>`
2. Sets the environment variable
3. Executes the real agent binary

### Permissions

The `permissions` option controls what actions agents can take without prompting. When set, Forage generates a settings file that is bind-mounted read-only into the container.

| Field | Description |
|-------|-------------|
| `skipAll` | Bypass all permission checks (grants all tool families) |
| `allow` | List of permission rules to auto-approve |
| `deny` | List of permission rules to always block |

`skipAll` cannot be combined with `allow` or `deny`.

**Full autonomy** (no permission prompts):

```nix
agents.claude = {
  package = pkgs.claude-code;
  secretName = "anthropic";
  authEnvVar = "ANTHROPIC_API_KEY";
  permissions.skipAll = true;
};
```

**Granular allowlist**:

```nix
agents.claude = {
  package = pkgs.claude-code;
  secretName = "anthropic";
  authEnvVar = "ANTHROPIC_API_KEY";
  permissions = {
    allow = [ "Read" "Glob" "Grep" "Edit(src/**)" "Bash(npm run *)" ];
    deny = [ "Bash(rm -rf *)" ];
  };
};
```

For Claude, the settings file is written to `/etc/claude-code/managed-settings.json` (managed scope — highest precedence, cannot be overridden by user or project settings). `permissions` and `hostConfigDir` can coexist — they target different paths.

### Extra Packages

Additional packages available in the sandbox:

```nix
extraPackages = with pkgs; [
  ripgrep
  fd
  jq
  yq
  tree
  htop
  git
];
```

These are added to `environment.systemPackages` in the container.

### Init Commands

Shell commands to run inside the container after creation. These execute after SSH is ready, as the container user in the workspace directory. Failures are logged as warnings but do not block sandbox creation.

```nix
initCommands = [
  "npm install"
  "pip install pytest"
];
```

Commands execute in order via `sh -c`. Each command runs independently — a failing command does not prevent subsequent commands from running.

#### Per-Project Init Script

In addition to template-level `initCommands`, you can place a `.forage/init` script in your repository. If present, it runs automatically after template init commands complete.

```bash
# .forage/init — runs inside the container after creation
#!/bin/sh
jj git fetch
jj new main
```

**Execution order:**
1. Template `initCommands` (in declaration order)
2. `.forage/init` script (if present in workspace)

#### Example: Beads Setup

```nix
templates.beads = {
  description = "Beads development sandbox";

  agents.claude = {
    package = pkgs.claude-code;
    hostConfigDir = "~/.claude";
    permissions.skipAll = true;
  };

  extraPackages = with pkgs; [ git nodejs ];

  initCommands = [
    "npm install -g beads"
  ];
};
```

Combined with a `.forage/init` in the repo:

```bash
#!/bin/sh
git fetch origin beads-sync
git checkout -b beads-sync origin/beads-sync 2>/dev/null || true
```

### Network Mode

Controls network access:

| Mode | Description |
|------|-------------|
| `full` | Unrestricted internet access (default) |
| `restricted` | Only allowed hosts can be accessed |
| `none` | No network access |

```nix
network = "full";
```

For restricted mode:

```nix
network = "restricted";
allowedHosts = [
  "api.anthropic.com"
  "api.openai.com"
];
```

You can also change network modes at runtime using `forage-ctl network`.

### Workspace Mounts

Templates can declare composable workspace mounts — multiple mount points assembled from different sources:

```nix
workspace.mounts = {
  main = {
    containerPath = "/workspace";
    mode = "jj";
    # repo = null → uses default --repo
  };
  data = {
    containerPath = "/workspace/data";
    repo = "data";  # references --repo data=<path>
    readOnly = true;
  };
};
```

When `workspace.mounts` is set, the `--repo` flag becomes optional (if all mounts specify their sources). See the [Workspace Mounts](../usage/workspace-mounts.md) usage guide for full details.

### Beads Overlay (`useBeads`)

A convenience option for overlaying a beads workspace:

```nix
workspace.useBeads = {
  enable = true;
  branch = "beads-sync";              # default
  containerPath = "/workspace/.beads"; # default
  package = pkgs.beads;               # added to extraPackages
};
```

This automatically injects a jj mount and the beads package. See [Workspace Mounts: useBeads](../usage/workspace-mounts.md#usebeads-convenience-option).

## Example Templates

### Minimal Claude Template

```nix
templates.claude = {
  agents.claude = {
    package = pkgs.claude-code;
    secretName = "anthropic";
    authEnvVar = "ANTHROPIC_API_KEY";
  };
};
```

### Full-Featured Development Template

```nix
templates.claude-dev = {
  description = "Claude Code with full development tooling";

  agents.claude = {
    package = pkgs.claude-code;
    secretName = "anthropic";
    authEnvVar = "ANTHROPIC_API_KEY";
  };

  extraPackages = with pkgs; [
    # Search and navigation
    ripgrep
    fd
    fzf
    tree

    # Data processing
    jq
    yq
    miller

    # Development
    git
    gh
    gnumake
    nodejs

    # Debugging
    htop
    strace
    lsof
  ];

  network = "full";
};
```

### Multi-Agent Template

```nix
templates.multi = {
  description = "Multiple AI assistants";

  agents = {
    claude = {
      package = pkgs.claude-code;
      secretName = "anthropic";
      authEnvVar = "ANTHROPIC_API_KEY";
    };

    aider = {
      package = pkgs.aider-chat;
      secretName = "openai";
      authEnvVar = "OPENAI_API_KEY";
    };
  };

  extraPackages = with pkgs; [ ripgrep fd git ];
};
```

### Autonomous Template

```nix
templates.claude-auto = {
  description = "Claude Code with full autonomy";

  agents.claude = {
    package = pkgs.claude-code;
    secretName = "anthropic";
    authEnvVar = "ANTHROPIC_API_KEY";
    permissions.skipAll = true;
  };

  network = "full";
};
```

### Multi-Mount Template with Beads

```nix
templates.claude-beads = {
  description = "Claude with beads overlay";

  agents.claude = {
    package = pkgs.claude-code;
    secretName = "anthropic";
    authEnvVar = "ANTHROPIC_API_KEY";
  };

  workspace.mounts.main = {
    containerPath = "/workspace";
    mode = "jj";
  };

  workspace.useBeads = {
    enable = true;
    package = pkgs.beads;
  };

  extraPackages = with pkgs; [ ripgrep fd jq ];
};
```

### Air-Gapped Template

```nix
templates.isolated = {
  description = "No network access for sensitive work";

  agents.claude = {
    package = pkgs.claude-code;
    secretName = "anthropic";
    authEnvVar = "ANTHROPIC_API_KEY";
  };

  network = "none";
};
```

## Template Selection

List available templates:

```bash
forage-ctl templates
```

Output:
```
TEMPLATE        AGENTS              NETWORK    DESCRIPTION
claude          claude              full       Claude Code sandbox
claude-dev      claude              full       Claude Code with full development tooling
multi           claude,aider        full       Multiple AI assistants
isolated        claude              none       No network access for sensitive work
```

Use a template when creating a sandbox:

```bash
forage-ctl up myproject --template claude-dev --workspace ~/projects/myproject
```

## How Templates Are Processed

1. **At NixOS build time**: Templates are converted to JSON files in `/etc/firefly-forage/templates/`

2. **At sandbox creation**: `forage-ctl` reads the template JSON and generates a container configuration

3. **Agent wrappers**: For each agent, a wrapper script is generated that injects authentication

The template JSON format:

```json
{
  "name": "claude",
  "description": "Claude Code sandbox",
  "network": "full",
  "allowedHosts": [],
  "agents": {
    "claude": {
      "packagePath": "/nix/store/...-claude-code",
      "secretName": "anthropic",
      "authEnvVar": "ANTHROPIC_API_KEY",
      "permissions": { "skipAll": true }
    }
  },
  "extraPackages": [
    "/nix/store/...-ripgrep",
    "/nix/store/...-fd"
  ]
}
```

When `workspace.mounts` is configured, the JSON includes a `workspaceMounts` field:

```json
{
  "workspaceMounts": {
    "main": {
      "containerPath": "/workspace",
      "mode": "jj"
    },
    "beads": {
      "containerPath": "/workspace/.beads",
      "mode": "jj",
      "branch": "beads-sync"
    }
  }
}
```

The `permissions` field is `null` when not configured. When set, it can contain:
- `{"skipAll": true}` — grants all tool families
- `{"allow": [...], "deny": [...]}` — granular rules

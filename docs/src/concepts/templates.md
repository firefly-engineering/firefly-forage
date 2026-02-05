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
      "authEnvVar": "ANTHROPIC_API_KEY"
    }
  },
  "extraPackages": [
    "/nix/store/...-ripgrep",
    "/nix/store/...-fd"
  ]
}
```

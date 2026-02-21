# Configuration

Forage is configured through your NixOS configuration. This page covers all available options.

## Minimal Configuration

```nix
services.firefly-forage = {
  enable = true;
  user = "myuser";
  authorizedKeys = [ "ssh-ed25519 AAAA..." ];

  secrets = {
    anthropic = "/run/secrets/anthropic-api-key";
  };

  templates.claude = {
    agents.claude = {
      package = pkgs.claude-code;
      secretName = "anthropic";
      authEnvVar = "ANTHROPIC_API_KEY";
    };
  };
};
```

## Full Configuration Reference

### Top-Level Options

#### `enable`

Whether to enable Firefly Forage.

```nix
services.firefly-forage.enable = true;
```

#### `user`

The host user whose UID/GID will be used inside sandboxes. This ensures files created in the workspace have correct ownership.

```nix
services.firefly-forage.user = "myuser";
```

#### `authorizedKeys`

SSH public keys that can access sandboxes. Typically you'll use the same keys as your user account:

```nix
services.firefly-forage.authorizedKeys =
  config.users.users.myuser.openssh.authorizedKeys.keys;
```

#### `portRange`

Port range for sandbox SSH servers. Each sandbox gets one port from this range.

```nix
services.firefly-forage.portRange = {
  from = 2200;  # default
  to = 2299;    # default
};
```

#### `stateDir`

Directory for Forage state (sandbox metadata, JJ workspaces).

```nix
services.firefly-forage.stateDir = "/var/lib/firefly-forage";  # default
```

### Secrets

Map secret names to file paths containing API keys:

```nix
services.firefly-forage.secrets = {
  anthropic = "/run/secrets/anthropic-api-key";
  openai = "/run/secrets/openai-api-key";
};
```

**With sops-nix:**

```nix
services.firefly-forage.secrets = {
  anthropic = config.sops.secrets.anthropic-api-key.path;
};
```

**With agenix:**

```nix
services.firefly-forage.secrets = {
  anthropic = config.age.secrets.anthropic-api-key.path;
};
```

### Templates

Templates define sandbox configurations that can be instantiated multiple times.

#### Basic Template

```nix
services.firefly-forage.templates.claude = {
  description = "Claude Code sandbox";

  agents.claude = {
    package = pkgs.claude-code;
    secretName = "anthropic";
    authEnvVar = "ANTHROPIC_API_KEY";
  };
};
```

#### Template with Extra Packages

```nix
services.firefly-forage.templates.claude = {
  description = "Claude Code with dev tools";

  agents.claude = {
    package = pkgs.claude-code;
    secretName = "anthropic";
    authEnvVar = "ANTHROPIC_API_KEY";
  };

  extraPackages = with pkgs; [
    ripgrep
    fd
    jq
    tree
    htop
  ];
};
```

#### Multi-Agent Template

```nix
services.firefly-forage.templates.multi = {
  description = "Multiple AI agents";

  agents = {
    claude = {
      package = pkgs.claude-code;
      secretName = "anthropic";
      authEnvVar = "ANTHROPIC_API_KEY";
    };

    aider = {
      package = pkgs.aider;
      secretName = "openai";
      authEnvVar = "OPENAI_API_KEY";
    };
  };

  extraPackages = with pkgs; [ ripgrep fd ];
};
```

#### Host Config Directory Mounting

Mount host configuration directories into sandboxes for persistent authentication. This is useful for agents like Claude Code that store credentials in `~/.claude/`:

```nix
services.firefly-forage.templates.claude = {
  agents.claude = {
    package = pkgs.claude-code;
    secretName = "anthropic";
    authEnvVar = "ANTHROPIC_API_KEY";
    hostConfigDir = "~/.claude";  # mounts to /home/agent/.claude
  };
};
```

Options:
- `hostConfigDir` - Host directory to mount (supports `~` expansion)
- `containerConfigDir` - Override the container mount point (default: `/home/agent/.<dirname>`)
- `hostConfigDirReadOnly` - Mount as read-only (default: `false` to allow token refresh)

Example with all options:

```nix
services.firefly-forage.templates.claude = {
  agents.claude = {
    package = pkgs.claude-code;
    secretName = "anthropic";
    authEnvVar = "ANTHROPIC_API_KEY";
    hostConfigDir = "~/.claude";
    containerConfigDir = "/home/agent/.claude";  # explicit path
    hostConfigDirReadOnly = false;  # allow writing (default)
  };
};
```

#### Agent Permissions

Control what agents can do without prompting. Permissions are written to a settings file and bind-mounted read-only into the container.

**Full autonomy** — skip all permission prompts:

```nix
services.firefly-forage.templates.claude-auto = {
  agents.claude = {
    package = pkgs.claude-code;
    secretName = "anthropic";
    authEnvVar = "ANTHROPIC_API_KEY";
    permissions.skipAll = true;
  };
};
```

**Granular allowlist** — approve specific tools/patterns:

```nix
services.firefly-forage.templates.claude-restricted = {
  agents.claude = {
    package = pkgs.claude-code;
    secretName = "anthropic";
    authEnvVar = "ANTHROPIC_API_KEY";
    permissions = {
      allow = [ "Read" "Glob" "Grep" "Edit(src/**)" "Bash(npm run *)" ];
      deny = [ "Bash(rm -rf *)" ];
    };
  };
};
```

Options:
- `permissions.skipAll` - Bypass all permission checks (cannot be combined with `allow`/`deny`)
- `permissions.allow` - Rules to auto-approve (agent-specific format)
- `permissions.deny` - Rules to always block

For Claude, this generates `/etc/claude-code/managed-settings.json` in the container (managed scope — highest precedence). Permissions and `hostConfigDir` can coexist — they target different paths.

#### Workspace Mounts

Templates can define composable workspace mounts — multiple mount points from different sources:

```nix
services.firefly-forage.templates.multi-mount = {
  description = "Multi-mount workspace";

  agents.claude = {
    package = pkgs.claude-code;
    secretName = "anthropic";
    authEnvVar = "ANTHROPIC_API_KEY";
  };

  workspace.mounts = {
    main = {
      containerPath = "/workspace";
      mode = "jj";
    };
    docs = {
      containerPath = "/workspace/docs";
      hostPath = "~/shared-docs";
      readOnly = true;
    };
  };
};
```

When `workspace.mounts` is set, the `--repo` flag becomes optional. See [Workspace Mounts](../usage/workspace-mounts.md) for the full guide.

The `workspace.useBeads` shorthand overlays a beads workspace:

```nix
workspace.useBeads = {
  enable = true;
  package = pkgs.beads;
  # branch = "beads-sync";         # default
  # containerPath = "/workspace/.beads";  # default
};
```

#### Network Modes

Control network access for sandboxes:

```nix
services.firefly-forage.templates = {
  # Full internet access (default)
  claude = {
    network = "full";
    # ...
  };

  # No network access (air-gapped)
  isolated = {
    network = "none";
    # ...
  };

  # Restricted to specific hosts
  restricted = {
    network = "restricted";
    allowedHosts = [ "api.anthropic.com" "api.openai.com" ];
    # ...
  };
};
```

You can also change network modes at runtime using `forage-ctl network`.

## Complete Example

```nix
{ config, pkgs, ... }:
{
  services.firefly-forage = {
    enable = true;
    user = "developer";
    authorizedKeys = config.users.users.developer.openssh.authorizedKeys.keys;

    portRange = {
      from = 2200;
      to = 2250;
    };

    secrets = {
      anthropic = config.sops.secrets.anthropic-api-key.path;
      openai = config.sops.secrets.openai-api-key.path;
    };

    templates = {
      claude = {
        description = "Claude Code for general development";
        agents.claude = {
          package = pkgs.claude-code;
          secretName = "anthropic";
          authEnvVar = "ANTHROPIC_API_KEY";
        };
        extraPackages = with pkgs; [ ripgrep fd jq yq tree ];
        network = "full";
      };

      claude-auto = {
        description = "Claude Code with full autonomy";
        agents.claude = {
          package = pkgs.claude-code;
          secretName = "anthropic";
          authEnvVar = "ANTHROPIC_API_KEY";
          permissions.skipAll = true;
        };
      };

      claude-isolated = {
        description = "Claude Code without network";
        agents.claude = {
          package = pkgs.claude-code;
          secretName = "anthropic";
          authEnvVar = "ANTHROPIC_API_KEY";
        };
        network = "none";
      };
    };
  };
}
```

## Next Steps

With configuration in place, [create your first sandbox](./first-sandbox.md).

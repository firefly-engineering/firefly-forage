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

  # Restricted to specific hosts (future)
  restricted = {
    network = "restricted";
    allowedHosts = [ "api.anthropic.com" "api.openai.com" ];
    # ...
  };
};
```

> **Note:** `restricted` mode is not yet implemented.

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

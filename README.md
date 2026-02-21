# Firefly Forage

Isolated, ephemeral sandboxes for AI coding agents on NixOS.

## Features

- **Isolation**: Run AI agents in contained systemd-nspawn environments
- **Efficiency**: Read-only nix store sharing via bind mounts
- **Disposability**: Ephemeral container roots, easy reset
- **Multi-agent**: Support multiple concurrent sandboxes
- **Security**: Auth obfuscation, optional network isolation

## Quick Start

### Installation

Add to your flake inputs:

```nix
{
  inputs.firefly-forage.url = "github:firefly-engineering/firefly-forage";
}
```

Import the module:

```nix
{ inputs, ... }:
{
  imports = [ inputs.firefly-forage.nixosModules.default ];

  services.firefly-forage = {
    enable = true;
    user = "myuser";
    authorizedKeys = [ "ssh-ed25519 AAAA..." ];

    secrets = {
      anthropic = config.sops.secrets.anthropic-api-key.path;
    };

    templates.claude = {
      description = "Claude Code sandbox";
      agents.claude = {
        package = pkgs.claude-code;
        secretName = "anthropic";
        authEnvVar = "ANTHROPIC_API_KEY";
      };
      extraPackages = with pkgs; [ ripgrep fd ];
    };
  };
}
```

### Usage

```bash
# List templates
forage-ctl templates

# Create a sandbox
forage-ctl up myproject --template claude --workspace ~/projects/myproject

# Connect via SSH
forage-ctl ssh myproject

# From inside the sandbox
claude chat "Hello!"

# Reset if polluted
forage-ctl reset myproject

# Clean up
forage-ctl down myproject
```

## Documentation

- **[User Guide](docs/src/SUMMARY.md)** - Getting started, configuration, and usage
- **[DESIGN.md](DESIGN.md)** - Architecture and design decisions

Build the docs locally:
```bash
nix build .#docs
# Or with mdbook directly
mdbook serve docs
```

## License

MIT

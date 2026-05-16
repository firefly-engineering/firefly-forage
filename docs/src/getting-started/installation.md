# Installation

Firefly Forage is distributed as a Nix flake. Add it to your NixOS configuration to get started.

## Add the Flake Input

In your `flake.nix`:

```nix
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

    firefly-forage = {
      url = "github:firefly-engineering/firefly-forage";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, firefly-forage, ... }: {
    nixosConfigurations.myhost = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      modules = [
        ./configuration.nix
        firefly-forage.nixosModules.default
      ];
    };
  };
}
```

## Import the Module

The module is automatically available after adding the flake input. You can also import it explicitly:

```nix
{ inputs, ... }:
{
  imports = [ inputs.firefly-forage.nixosModules.default ];
}
```

## Enable the Service

Add basic configuration to enable Forage:

```nix
{ config, pkgs, ... }:
{
  services.firefly-forage = {
    enable = true;
    user = "myuser";  # Your username
    authorizedKeys = config.users.users.myuser.openssh.authorizedKeys.keys;
  };
}
```

## Rebuild

Apply the configuration:

```bash
sudo nixos-rebuild switch --flake .#myhost
```

After rebuilding, the `forage-ctl` command will be available system-wide.

## Verify Installation

```bash
# Should show help
forage-ctl --help

# Should show no templates yet
forage-ctl templates
```

## Next Steps

Now [configure your first template](./configuration.md) to define what agents and packages your sandboxes will include.

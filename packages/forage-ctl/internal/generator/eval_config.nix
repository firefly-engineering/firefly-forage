# Minimal eval-config for evaluating outer container definitions.
# Matches extra-container's 8-module set but WITHOUT the extraModule that
# forces full inner NixOS evaluation. This makes outer eval fast (~0.5s)
# because the inner system is referenced by store path, not re-evaluated.
{ nixosPath, systemConfig }:
let
  baseModules = [
    (nixosPath + "/nixos/modules/misc/assertions.nix")
    (nixosPath + "/nixos/modules/misc/nixpkgs.nix")
    (nixosPath + "/nixos/modules/system/activation/top-level.nix")
    (nixosPath + "/nixos/modules/system/etc/etc.nix")
    (nixosPath + "/nixos/modules/system/activation/activation-script.nix")
    (nixosPath + "/nixos/modules/system/boot/systemd.nix")
    (nixosPath + "/nixos/modules/tasks/filesystems.nix")
    (nixosPath + "/nixos/modules/virtualisation/nixos-containers.nix")
  ];
in
import (nixosPath + "/nixos/lib/eval-config.nix") {
  inherit baseModules;
  system = builtins.currentSystem or "x86_64-linux";
  modules = [ systemConfig ];
}

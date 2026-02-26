# Stripped version of extra-container's eval-config.nix.
# The only difference: the `extraModule` is removed from the modules list.
# This means our outer config evaluates in ~0.5s instead of ~13s because
# no inner NixOS system is evaluated via the extraModule's injected `config`.
{ nixosPath
, systemConfig
, system ? builtins.currentSystem
}:

let
  # A minimal module set for evaluating container configs.
  # This significantly reduces extra-container evaluation overhead (total eval time - container eval time)
  # Compatible with nixpkgs >= 16.09
  baseModules = [
    (nixosPath + "/modules/misc/assertions.nix")
    (nixosPath + "/modules/misc/nixpkgs.nix")
    (nixosPath + "/modules/misc/extra-arguments.nix")
    (nixosPath + "/modules/system/activation/top-level.nix")
    (nixosPath + "/modules/system/etc/etc.nix")
    (nixosPath + "/modules/system/boot/systemd.nix")
    nixosContainerModule
    dummyOptions
  ];

  nixosContainerModule = let
    new = nixosPath + "/modules/virtualisation/nixos-containers.nix";
    old = nixosPath + "/modules/virtualisation/containers.nix"; # For nixpkgs < 20.09)
  in
    if builtins.pathExists new then new else old;

  dummyOptions = { pkgs, lib, options, ... }: let
    optionValue = default: lib.mkOption { inherit default; };
    dummy = optionValue [];
  in {
    options = {
      boot.kernel.sysctl = dummy;
      boot.kernelModules = dummy;
      boot.kernelPackages.kernel.version = optionValue "";
      boot.kernelParams = dummy;
      boot.loader.systemd-boot.bootCounting.enable = optionValue false;
      environment.systemPackages = dummy;
      networking.dhcpcd.denyInterfaces = dummy;
      networking.hosts = dummy;
      networking.extraHosts = dummy;
      networking.proxy.envVars = optionValue {};
      nix.package = optionValue pkgs.nix;
      security = dummy;
      services = {
        dbus = dummy;
        logrotate = dummy;
        udev = dummy;
        rsyslogd.enable = optionValue false;
        syslog-ng.enable = optionValue false;
      };
      system.activationScripts = dummy;
      system.fsPackages = dummy;
      system.nssDatabases = dummy;
      system.nssModules = dummy;
      system.path = optionValue "";
      system.nixos-init.package = optionValue pkgs.hello;
      system.requiredKernelConfig = dummy;
      system.stateVersion = optionValue "22.05";
      systemd.oomd = dummy;
      systemd.user.generators = optionValue {};
      ids.gids.keys = dummy;
      ids.uids.systemd-coredump = dummy;
      ids.gids.systemd-journal = dummy;
      ids.gids.systemd-journal-gateway = dummy;
      ids.uids.systemd-journal-gateway = dummy;
      ids.gids.systemd-network = dummy;
      ids.uids.systemd-network = dummy;
      ids.uids.systemd-resolve = dummy;
      ids.gids.systemd-resolve = dummy;
      users.users.systemd-coredump = dummy;
      users.users.systemd-network.group = dummy;
      users.users.systemd-network.uid = dummy;
      users.users.systemd-resolve.group = dummy;
      users.users.systemd-resolve.uid = dummy;
      users.users.systemd-journal-gateway.group = dummy;
      users.users.systemd-journal-gateway.uid = dummy;
      users.groups.systemd-coredump = dummy;
      users.groups.systemd-network.gid = dummy;
      users.groups.systemd-resolve.gid = dummy;
      users.groups.keys.gid = dummy;
      users.groups.systemd-journal.gid = dummy;
      users.groups.systemd-journal-gateway.gid = dummy;
    };

    config = {
      systemd.timers = lib.mkForce {};
      systemd.targets = lib.mkForce {};
    } // lib.optionalAttrs (options.systemd ? managerEnvironment) {
      systemd.managerEnvironment = lib.mkForce {};
    };
  };

in
# Key difference from extra-container's version:
# modules = [ systemConfig ] instead of [ extraModule systemConfig ]
# This skips the extraModule that injects a `config` attribute into every
# container definition, which would trigger a full NixOS module evaluation
# even when our outer config uses `path = lib.mkForce <cached-path>`.
import (nixosPath + "/lib/eval-config.nix") {
  inherit baseModules system;
  modules = [ systemConfig ];
}

# E2E test VM configuration for Firefly Forage
#
# This builds a QEMU VM via NixOS's config.system.build.vm mechanism.
# The VM boots with the forage module fully configured, SSH access for the
# test driver, and a pre-built container closure so extra-container's
# nix-build is essentially a no-op.
#
# Architecture: Host -> QEMU/KVM (this VM) -> systemd-nspawn (forage sandboxes)
#
# Usage:
#   nix build .#packages.x86_64-linux.e2e-vm    # Build the VM
#   nix build .#packages.x86_64-linux.e2e-driver # Build the test driver
#   just test-e2e                                 # Build + run
{ pkgs, self }:
let
  inherit (pkgs) lib;

  sshPubKey = lib.trim (builtins.readFile ./ssh-key.pub);
  sshPrivKey = ./ssh-key;

  # Pre-build a NixOS container system closure that closely matches what
  # forage-ctl generates (see internal/generator/templates.go). This ensures
  # all required store paths are available in the VM's shared nix store,
  # making extra-container's nix-build essentially a no-op.
  #
  # The packages here mirror the container template in templates.go:
  # git, jujutsu, tmux, neovim, ripgrep, fd, plus openssh for the container.
  prebuiltContainerSystem =
    (import (pkgs.path + "/nixos/lib/eval-config.nix") {
      inherit (pkgs.stdenv.hostPlatform) system;
      modules = [
        {
          boot.isContainer = true;
          system.stateVersion = "24.11";
          nixpkgs.config.allowUnfree = true;

          networking = {
            hostName = "forage-prebuilt";
            # Match the "none" network mode from network.go:
            nameservers = [ ];
            defaultGateway = null;
            useDHCP = false;
            firewall.enable = false;
            nftables.enable = true;
            useHostResolvConf = lib.mkForce false;
          };

          users.users.agent = {
            isNormalUser = true;
            uid = 1000;
            group = "users";
            home = "/home/agent";
            shell = pkgs.bash;
            openssh.authorizedKeys.keys = [ sshPubKey ];
          };
          users.groups.users.gid = 100;

          security.sudo.enable = false;

          services.openssh = {
            enable = true;
            ports = [ 22 ];
            settings = {
              PasswordAuthentication = false;
              PermitRootLogin = "no";
            };
          };

          # Match the packages from generator/templates.go
          environment.systemPackages = with pkgs; [
            git
            jujutsu
            tmux
            neovim
            ripgrep
            fd
            bash
            coreutils
            hello # test agent package
          ];

          # nftables already enabled via networking block above
        }
      ];
    }).config.system.build.toplevel;

  # The NixOS VM system evaluation
  vmSystem = import (pkgs.path + "/nixos/lib/eval-config.nix") {
    inherit (pkgs.stdenv.hostPlatform) system;
    modules = [
      self.nixosModules.host
      (pkgs.path + "/nixos/modules/virtualisation/qemu-vm.nix")
      (
        { config, pkgs, ... }:
        {
          # Predictable hostname for the VM script name
          networking.hostName = "forage-e2e";
          system.stateVersion = "24.11";

          # --- Test user ---
          users.users.testuser = {
            isNormalUser = true;
            uid = 1000;
            group = "users";
          };

          # --- Root SSH access for test driver ---
          users.users.root.openssh.authorizedKeys.keys = [ sshPubKey ];
          services.openssh = {
            enable = true;
            settings = {
              PermitRootLogin = "prohibit-password";
            };
          };

          # --- Install the test SSH key for forage-ctl exec ---
          # forage-ctl exec uses SSH to connect to containers. It needs
          # the private key matching the authorized keys in the container.
          system.activationScripts.forage-test-ssh-key = ''
            mkdir -p /root/.ssh
            cp ${sshPrivKey} /root/.ssh/id_ed25519
            chmod 600 /root/.ssh/id_ed25519
            cp ${./ssh-key.pub} /root/.ssh/id_ed25519.pub
            chmod 644 /root/.ssh/id_ed25519.pub

            # Also for testuser
            mkdir -p /home/testuser/.ssh
            cp ${sshPrivKey} /home/testuser/.ssh/id_ed25519
            chmod 600 /home/testuser/.ssh/id_ed25519
            cp ${./ssh-key.pub} /home/testuser/.ssh/id_ed25519.pub
            chmod 644 /home/testuser/.ssh/id_ed25519.pub
            chown -R testuser:users /home/testuser/.ssh
          '';

          # --- Dummy secret file for testing ---
          environment.etc."forage-test-secret".text = "test-api-key-e2e";

          # --- Forage module configuration ---
          services.firefly-forage = {
            enable = true;
            user = "testuser";
            authorizedKeys = [ sshPubKey ];
            secrets.test-secret = "/etc/forage-test-secret";
            templates.test = {
              description = "E2E test template";
              network = "none";
              agents.test-agent = {
                package = pkgs.hello;
                secretName = "test-secret";
                authEnvVar = "TEST_KEY";
              };
            };
          };

          # --- Additional packages for testing ---
          environment.systemPackages = with pkgs; [
            jujutsu
            git
            openssh
            curl
          ];

          # --- NAT for container networking ---
          # Containers use private networks (10.100.x.x). NAT allows them
          # to reach the VM's network. The ve-+ wildcard matches all veth
          # interfaces created by systemd-nspawn.
          networking.nat = {
            enable = true;
            internalInterfaces = [ "ve-+" ];
          };

          # --- Nix configuration ---
          nix.nixPath = [ "nixpkgs=${pkgs.path}" ];
          nix.settings = {
            experimental-features = [
              "nix-command"
              "flakes"
            ];
            # Limit to 1 build job to reduce memory/IO pressure during
            # extra-container's nix-build of container config derivations.
            max-jobs = 1;
          };

          # --- Git configuration (required for jj and forage-ctl) ---
          environment.etc."gitconfig".text = ''
            [user]
              email = test@forage-e2e.local
              name = Forage E2E Test
          '';

          # --- JJ configuration (set via environment variable) ---
          environment.variables.JJ_USER = "Forage E2E Test";
          environment.variables.JJ_EMAIL = "test@forage-e2e.local";

          # --- Pre-build container closure ---
          # This ensures all store paths needed by the container are available
          # in the VM's nix store (shared from host via 9p). When extra-container
          # runs nix-build, it becomes essentially a no-op for packages.
          #
          # The prebuiltContainerSystem provides all runtime packages.
          # stdenv + perl provide the build tools (gcc, binutils, etc.) that
          # nix-build needs to produce the container's config derivations
          # (systemd units, etc files, activation scripts). Without these,
          # nix-build downloads ~400MB of build dependencies on every run.
          system.extraDependencies = [
            prebuiltContainerSystem
            pkgs.stdenv
            pkgs.stdenv.cc
            pkgs.perl
            pkgs.desktop-file-utils # needed by NixOS activation
            pkgs.texinfo # NixOS build dependency
            pkgs.libxslt # NixOS build dependency
            pkgs.lndir # NixOS build dependency
            pkgs.shellcheck # NixOS check phase
          ];

          # --- QEMU VM settings ---
          virtualisation = {
            memorySize = 8192;
            cores = 4;
            diskSize = 30720; # 30GB for nix store overlay + container roots

            # Use an erofs image for the nix store instead of 9p.
            # 9p is too slow for NixOS evaluation (reads thousands of files)
            # and causes QEMU crashes under heavy load. The erofs image is
            # mounted as a block device with a writable tmpfs overlay.
            #
            # Note: writableStoreUseTmpfs MUST be true. Disk-backed overlays
            # (false) cause silent VM crashes during nix-build, likely due to
            # I/O contention between the erofs block device and qcow2 overlay.
            useNixStoreImage = true;
            writableStore = true;
            writableStoreUseTmpfs = true;

            forwardPorts = [
              {
                from = "host";
                host.port = 2222;
                guest.port = 22;
              }
            ];
          };
        }
      )
    ];
  };

  vm = vmSystem.config.system.build.vm;

  # Build the Go E2E test binary with the e2e build tag.
  # This produces a standalone binary that boots the VM, runs all test
  # scenarios via SSH, and reports results using Go's testing framework.
  e2eTestBin = pkgs.buildGoModule {
    pname = "forage-e2e-test-bin";
    version = "0.1.0";

    src = ../../packages/forage-ctl;

    vendorHash = "sha256-1bHdfu/a6E7gjrU9z+xwi4t+bBrzwdXgADX5aAffHNk=";

    # Use proxy vendor because `go mod vendor` doesn't include packages
    # only imported by build-tagged files (e2e tag). proxyVendor uses the
    # Go module cache instead, making all go.mod dependencies available.
    proxyVendor = true;

    # Only build the e2e test binary, not the main CLI
    buildPhase = ''
      runHook preBuild
      go test -c -tags=e2e -o $GOPATH/bin/forage-e2e-test ./e2e/
      runHook postBuild
    '';

    installPhase = ''
      mkdir -p $out/bin
      cp $GOPATH/bin/forage-e2e-test $out/bin/
    '';

    # Skip normal check phase — this is a test binary, not a library
    doCheck = false;

    env.CGO_ENABLED = "0";
  };

  # Wrapper that sets environment variables and runs the Go test binary
  testDriver = pkgs.writeShellApplication {
    name = "forage-e2e-test";
    text = ''
      export E2E_SSH_KEY="${sshPrivKey}"
      export E2E_VM="${vm}/bin/run-forage-e2e-vm"
      exec "${e2eTestBin}/bin/forage-e2e-test" -test.v -test.timeout=900s "$@"
    '';
  };

in
{
  inherit vm testDriver;
}

{
  description = "Firefly Forage - Isolated sandboxes for AI coding agents";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    toolbox.url = "github:firefly-engineering/toolbox";
    toolbox.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs =
    {
      self,
      nixpkgs,
      toolbox,
    }:
    let
      supportedSystems = [
        "x86_64-linux"
        "aarch64-linux"
        "aarch64-darwin"
        "x86_64-darwin"
      ];

      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;

      pkgsFor = system: nixpkgs.legacyPackages.${system};
    in
    {
      nixosModules = {
        default = self.nixosModules.host;
        host = import ./modules/host.nix { inherit self nixpkgs; };
      };

      lib = import ./lib { inherit (nixpkgs) lib; };

      packages = forAllSystems (
        system:
        let
          pkgs = pkgsFor system;
          isLinux = pkgs.stdenv.isLinux;
          e2e = import ./tests/e2e/vm.nix { inherit pkgs self; };
        in
        {
          forage-ctl = pkgs.callPackage ./packages/forage-ctl { };
          docs = pkgs.stdenvNoCC.mkDerivation {
            pname = "firefly-forage-docs";
            version = "0.1.0";
            src = ./docs;
            nativeBuildInputs = [ pkgs.mdbook ];
            buildPhase = ''
              mdbook build
            '';
            installPhase = ''
              mv book $out
            '';
          };
          default = self.packages.${system}.forage-ctl;
        }
        // nixpkgs.lib.optionalAttrs isLinux {
          # E2E test VM (QEMU) - builds the bootable VM image
          e2e-vm = e2e.vm;
          # E2E test driver - boots VM and runs full lifecycle tests
          e2e-driver = e2e.testDriver;
        }
      );

      devShells = forAllSystems (
        system:
        let
          pkgs = pkgsFor system;
        in
        {
          default = pkgs.mkShell {
            packages =
              with pkgs;
              [
                # Go toolchain
                go
                gopls
                gotools
                go-tools # staticcheck
                golangci-lint

                # Nix tooling
                nixfmt-tree
                nil

                # Documentation
                mdbook

                # Testing dependencies
                git
                jujutsu

                # Task runner
                just
              ]
              ++ (with toolbox.packages.${system}; [
                beads-rust-default
                beads-viewer-default
              ]);
          };

          # Minimal shell for CI — avoids pulling in toolbox (jj, beadwork, rust)
          ci = pkgs.mkShell {
            packages = with pkgs; [
              go
              golangci-lint
              git
            ];
          };
        }
      );

      formatter = forAllSystems (system: (pkgsFor system).nixfmt-tree);

      # Integration tests (VM tests only work on Linux)
      checks = forAllSystems (
        system:
        let
          pkgs = pkgsFor system;
          isLinux = pkgs.stdenv.isLinux;
        in
        {
          # NixOS VM integration test using the actual module
          # Only available on Linux systems
          vm-integration =
            if isLinux then
              import ./tests/vm-integration.nix { inherit pkgs self; }
            else
              # Placeholder for non-Linux systems
              pkgs.runCommand "vm-integration-unsupported" { } ''
                echo "VM integration tests are only supported on Linux" > $out
              '';
        }
      );
    };
}

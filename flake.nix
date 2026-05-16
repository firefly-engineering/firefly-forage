{
  description = "Firefly Forage - Isolated sandboxes for AI coding agents";

  inputs = {
    nix-pins.url = "github:firefly-engineering/nix-pins";
    nixpkgs.follows = "nix-pins/nixpkgs";
    toolbox.url = "github:firefly-engineering/toolbox";
    toolbox.inputs.nix-pins.follows = "nix-pins";
    toolbox.inputs.devenv.follows = "";
  };

  outputs =
    {
      self,
      nixpkgs,
      toolbox,
      ...
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

      darwinModules = {
        default = self.darwinModules.host;
        host = import ./modules/darwin.nix { inherit self; };
      };

      lib = import ./lib { inherit (nixpkgs) lib; };

      packages = forAllSystems (
        system:
        let
          pkgs = pkgsFor system;
          isLinux = pkgs.stdenv.isLinux;

          # Shared Go module source — used by forage-ctl and e2e tests.
          # The Go workspace has a `replace` directive pointing to ../../images/forage-base,
          # so the source must include both directories rooted at the repo root.
          goSrc = pkgs.lib.fileset.toSource {
            root = ./.;
            fileset = pkgs.lib.fileset.unions [
              ./packages/forage-ctl
              ./images/forage-base
            ];
          };
          goModRoot = "packages/forage-ctl";

          e2e = import ./tests/e2e/vm.nix {
            inherit
              pkgs
              self
              goSrc
              goModRoot
              ;
          };
        in
        {
          forage-ctl = pkgs.callPackage ./packages/forage-ctl { inherit goSrc goModRoot; };
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
                # Nix tooling
                nixfmt-tree
                nil

                # Testing dependencies
                git
              ]
              ++ (with toolbox.packages.${system}; [
                beadwork
                go-toolchain
                just
                mdbook-toolchain
                vcs-toolchain
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

          # Darwin module evaluation test — verifies darwin.nix produces correct config.
          # Runs on all platforms (pure Nix evaluation, no VM needed).
          darwin-eval = import ./tests/darwin-eval.nix { inherit pkgs self; };
        }
      );
    };
}

{
  description = "Firefly Forage - Isolated sandboxes for AI coding agents";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    extra-container.url = "github:erikarvstedt/extra-container";
    extra-container.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs =
    {
      self,
      nixpkgs,
      extra-container,
    }:
    let
      supportedSystems = [
        "x86_64-linux"
        "aarch64-linux"
      ];

      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;

      pkgsFor = system: nixpkgs.legacyPackages.${system};
    in
    {
      nixosModules = {
        default = self.nixosModules.host;
        host = import ./modules/host.nix { inherit self extra-container nixpkgs; };
      };

      lib = import ./lib { inherit (nixpkgs) lib; };

      packages = forAllSystems (
        system:
        let
          pkgs = pkgsFor system;
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
      );

      devShells = forAllSystems (
        system:
        let
          pkgs = pkgsFor system;
        in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
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

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
              nixfmt
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

      formatter = forAllSystems (system: (pkgsFor system).nixfmt);

      # Integration tests
      checks.x86_64-linux = let
        pkgs = pkgsFor "x86_64-linux";
      in {
        # NixOS VM integration test using the actual module
        vm-integration = pkgs.testers.runNixOSTest {
          name = "firefly-forage-integration";

          nodes.machine = { config, pkgs, ... }: {
            imports = [ self.nixosModules.host ];

            # Test user for the sandbox
            users.users.testuser = {
              isNormalUser = true;
              uid = 1000;
            };

            # Create a dummy secret file for testing
            environment.etc."forage-test-secret".text = "test-api-key";

            # Enable the firefly-forage module with a test template
            services.firefly-forage = {
              enable = true;
              user = "testuser";
              authorizedKeys = [
                "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestKeyForIntegrationTests test@forage"
              ];
              secrets.test-secret = "/etc/forage-test-secret";
              templates.test = {
                description = "Test template for integration tests";
                network = "none";
                agents.test-agent = {
                  package = pkgs.hello;
                  secretName = "test-secret";
                  authEnvVar = "TEST_KEY";
                };
              };
            };

            virtualisation = {
              memorySize = 2048;
              cores = 2;
            };
          };

          testScript = ''
            machine.wait_for_unit("multi-user.target")

            # Verify forage-ctl is installed and runs
            machine.succeed("forage-ctl --help")

            # Verify the module created the expected directories
            machine.succeed("test -d /var/lib/firefly-forage")
            machine.succeed("test -d /etc/firefly-forage/templates")

            # Verify the test template is available
            machine.succeed("forage-ctl templates | grep -q test")

            # machinectl should work (triggers systemd-machined via socket)
            machine.succeed("machinectl list")

            print("All integration tests passed!")
          '';
        };
      };
    };
}

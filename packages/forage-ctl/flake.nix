{
  description = "Firefly Forage sandbox management CLI";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};

        # The forage-ctl package
        forage-ctl = pkgs.buildGoModule {
          pname = "forage-ctl";
          version = "0.1.0";
          src = ./.;
          vendorHash = "sha256-hoXlgAidu3eEhY5GBx/YDxzThdBIKfQtmA9bQ21eIzU=";
          env.CGO_ENABLED = "0";
        };

        # NixOS VM integration test
        vmTest = pkgs.testers.runNixOSTest {
          name = "forage-ctl-integration";

          nodes.machine = { config, pkgs, ... }: {
            boot.enableContainers = true;

            # Ensure machined socket is enabled (service is socket-activated)
            systemd.sockets.systemd-machined.wantedBy = [ "sockets.target" ];

            systemd.tmpfiles.rules = [
              "d /var/lib/firefly-forage 0755 root root -"
              "d /var/lib/firefly-forage/sandboxes 0755 root root -"
              "d /etc/firefly-forage 0755 root root -"
              "d /etc/firefly-forage/templates 0755 root root -"
            ];

            environment.systemPackages = [ forage-ctl pkgs.jq ];

            virtualisation = {
              memorySize = 2048;
              cores = 2;
            };

            environment.etc."firefly-forage/templates/test.json".text = builtins.toJSON {
              name = "test";
              description = "Test template";
              network = "full";
              agents.test-agent = {
                packagePath = "pkgs.hello";
                secretName = "test-secret";
                authEnvVar = "TEST_KEY";
              };
            };

            environment.etc."firefly-forage/config.json".text = builtins.toJSON {
              runtime = "nspawn";
              hostAddress = "10.250.0.1";
              containerSubnet = "10.250.0.0/16";
              portRange = { from = 2200; to = 2299; };
            };
          };

          testScript = ''
            machine.wait_for_unit("multi-user.target")

            # Verify forage-ctl runs
            machine.succeed("forage-ctl --version")

            # Verify config files exist
            machine.succeed("test -f /etc/firefly-forage/config.json")
            machine.succeed("test -f /etc/firefly-forage/templates/test.json")

            # machinectl triggers systemd-machined via socket activation
            machine.succeed("machinectl list")

            # Verify template is loadable
            machine.succeed("forage-ctl templates 2>&1 | grep -q test")

            print("All VM integration tests passed!")
          '';
        };
      in
      {
        packages = {
          default = forage-ctl;
          forage-ctl = forage-ctl;
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go gopls golangci-lint delve
            docker-client openssh go-junit-report
          ];

          shellHook = ''
            echo "Firefly Forage development shell"
            echo ""
            echo "Commands:"
            echo "  go test ./...                                    - Run unit tests"
            echo "  FORAGE_INTEGRATION_TESTS=1 go test ./...         - Run all tests"
            echo "  FORAGE_RUNTIME=docker go test ./...              - Use docker runtime"
            echo "  nix build .#checks.x86_64-linux.vm-integration   - Run VM tests"
            echo ""
          '';

          FORAGE_RUNTIME = "docker";
        };

        checks = {
          unit-tests = pkgs.runCommand "unit-tests" {
            nativeBuildInputs = [ pkgs.go ];
            src = ./.;
          } ''
            export HOME=$(mktemp -d)
            export GOPATH=$HOME/go
            export GOCACHE=$HOME/.cache/go-build
            cd $src
            go test ./... -short 2>&1 | tee $out
          '';
        } // (if system == "x86_64-linux" then {
          vm-integration = vmTest;
        } else {});
      }
    );
}

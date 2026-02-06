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
      in
      {
        packages = {
          default = forage-ctl;
          forage-ctl = forage-ctl;
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            # Go development
            go
            gopls
            golangci-lint
            delve

            # Container runtimes for testing
            docker-client

            # SSH tools for gateway testing
            openssh

            # Testing tools
            go-junit-report
          ];

          shellHook = ''
            echo "Firefly Forage development shell"
            echo ""
            echo "Commands:"
            echo "  go test ./...                          - Run unit tests"
            echo "  FORAGE_INTEGRATION_TESTS=1 go test ./... - Run all tests including integration"
            echo "  FORAGE_RUNTIME=docker go test ./...    - Use docker runtime for tests"
            echo ""
          '';

          # Make docker runtime the default for integration tests
          FORAGE_RUNTIME = "docker";
        };

        # Docker-based integration test
        checks.docker-integration = pkgs.runCommand "docker-integration-test" {
          buildInputs = [ pkgs.go pkgs.docker-client ];
          src = ./.;
        } ''
          cd $src
          export HOME=$(mktemp -d)
          export GOPATH=$HOME/go
          export GOCACHE=$HOME/.cache/go-build

          # Run tests with docker runtime
          export FORAGE_RUNTIME=docker
          export FORAGE_INTEGRATION_TESTS=1

          go test -v ./internal/integration/... 2>&1 | tee $out
        '';
      }
    ) // {
      # NixOS VM test for full container testing with actual nspawn
      # This tests the real container infrastructure
      nixosConfigurations.integration-test-vm = nixpkgs.lib.nixosSystem {
        system = "x86_64-linux";
        modules = [
          ({ pkgs, ... }: {
            # VM configuration for testing
            virtualisation = {
              memorySize = 2048;
              cores = 2;
              diskSize = 4096;
            };

            # Enable container support
            boot.enableContainers = true;

            # Install forage-ctl and dependencies
            environment.systemPackages = with pkgs; [
              self.packages.x86_64-linux.forage-ctl
              go
              openssh
            ];

            # Enable SSH for testing gateway
            services.openssh.enable = true;

            # Test user
            users.users.test = {
              isNormalUser = true;
              extraGroups = [ "systemd-journal" ];
              password = "test";
            };

            # Create test directories
            systemd.tmpfiles.rules = [
              "d /var/lib/firefly-forage 0755 root root -"
              "d /var/lib/firefly-forage/sandboxes 0755 root root -"
              "d /etc/firefly-forage 0755 root root -"
              "d /etc/firefly-forage/templates 0755 root root -"
            ];
          })
        ];
      };
    };
}

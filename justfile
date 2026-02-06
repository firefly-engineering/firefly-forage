# Firefly Forage development tasks

# Default recipe - show available commands
default:
    @just --list

# Build forage-ctl
build:
    nix build .#forage-ctl

# Run all Go tests (unit + workflow)
test:
    cd packages/forage-ctl && go test ./...

# Run Go tests with verbose output
test-verbose:
    cd packages/forage-ctl && go test -v ./...

# Run a specific test package
test-pkg pkg:
    cd packages/forage-ctl && go test -v ./internal/{{pkg}}/...

# Run docker integration tests (requires docker daemon)
test-docker:
    cd packages/forage-ctl && FORAGE_INTEGRATION_TESTS=1 FORAGE_RUNTIME=docker go test -v ./internal/integration/...

# Run all tests including docker integration
test-all: test test-docker

# Run NixOS VM integration test (full nspawn environment)
test-vm:
    cd packages/forage-ctl && nix build .#checks.x86_64-linux.vm-integration

# Format all code
fmt:
    nix fmt
    cd packages/forage-ctl && go fmt ./...

# Run Go linter (golangci-lint)
lint:
    cd packages/forage-ctl && golangci-lint run

# Run Go linter with auto-fix
lint-fix:
    cd packages/forage-ctl && golangci-lint run --fix

# Build documentation
docs:
    nix build .#docs

# Serve documentation locally
docs-serve:
    cd docs && mdbook serve

# Check everything (fmt, lint, test, build)
check: fmt lint test build

# Clean build artifacts
clean:
    rm -rf result
    cd packages/forage-ctl && go clean

# Update Go dependencies
update-deps:
    cd packages/forage-ctl && go get -u ./... && go mod tidy

# Show flake outputs
outputs:
    nix flake show

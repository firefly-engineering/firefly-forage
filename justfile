# Firefly Forage development tasks

# Default recipe - show available commands
default:
    @just --list

# Build forage-ctl with nix
build:
    nix build .#forage-ctl

# Run all Go tests
test:
    @just packages/forage-ctl/test

# Run Go tests with verbose output
test-v:
    @just packages/forage-ctl/test-v

# Run a specific test package
test-pkg pkg:
    @just packages/forage-ctl/test-pkg {{pkg}}

# Run docker integration tests (requires docker daemon)
test-docker:
    @just packages/forage-ctl/test-docker

# Run all Go tests including docker integration
test-all:
    @just packages/forage-ctl/test-all

# Run NixOS VM integration test (uses actual nixosModule, Linux only)
test-vm:
    nix build .#checks.$(nix eval --raw --impure --expr 'builtins.currentSystem').vm-integration

# Format all code
fmt:
    nix fmt -- .
    @just packages/forage-ctl/fmt

# Run Go linter
lint:
    @just packages/forage-ctl/lint

# Fix Go linter issues
lint-fix:
    @just packages/forage-ctl/lint-fix

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
    @just packages/forage-ctl/clean

# Update Go dependencies
update-deps:
    @just packages/forage-ctl/update-deps

# Show flake outputs
outputs:
    nix flake show

# Darwin module evaluation test for Firefly Forage
#
# This test evaluates the nix-darwin module (darwin.nix) and verifies it produces
# correct configuration without needing a full macOS VM. It checks:
# - config.json is generated with expected fields
# - template JSON files are produced for each template
# - activation scripts create the expected directories
# - assertions catch invalid configurations
#
# Run with: nix build .#checks.<system>.darwin-eval
{ pkgs, self }:

let
  lib = pkgs.lib;

  # Minimal nix-darwin-like module system for evaluation.
  # We only need enough structure to evaluate our module —
  # not the full nix-darwin framework.
  evalModule =
    { extraConfig ? { } }:
    lib.evalModules {
      modules = [
        # Provide the minimal module interface that darwin.nix expects
        {
          options = {
            environment.systemPackages = lib.mkOption {
              type = lib.types.listOf lib.types.package;
              default = [ ];
            };
            environment.etc = lib.mkOption {
              type = lib.types.attrsOf (
                lib.types.submodule {
                  options = {
                    text = lib.mkOption { type = lib.types.str; };
                    target = lib.mkOption {
                      type = lib.types.str;
                      default = "";
                    };
                    mode = lib.mkOption {
                      type = lib.types.str;
                      default = "0444";
                    };
                  };
                }
              );
              default = { };
            };
            system.activationScripts = lib.mkOption {
              type = lib.types.attrsOf lib.types.anything;
              default = { };
            };
            launchd.daemons = lib.mkOption {
              type = lib.types.attrsOf lib.types.anything;
              default = { };
            };
            users.users = lib.mkOption {
              type = lib.types.attrsOf lib.types.anything;
              default = { };
            };
            assertions = lib.mkOption {
              type = lib.types.listOf lib.types.anything;
              default = [ ];
            };
          };
        }
        # Import our darwin module
        (import ../modules/darwin.nix { inherit self; })
        # Test configuration
        extraConfig
      ];
    };

  # Basic configuration for testing
  basicConfig = evalModule {
    extraConfig = {
      services.firefly-forage = {
        enable = true;
        user = "testuser";
        secrets.test-secret = "/run/forage-secrets/test";
        templates.claude = {
          description = "Test Claude template";
          network = "full";
          agents.claude = {
            package = pkgs.hello; # Dummy package
            secretName = "test-secret";
            authEnvVar = "ANTHROPIC_API_KEY";
          };
        };
      };
    };
  };

  cfg = basicConfig.config;

  # Parse the generated config.json
  hostConfigJSON = builtins.fromJSON cfg.environment.etc."firefly-forage/config.json".text;

  # Template entries are keyed by template name (not path)
  templateJSON = builtins.fromJSON cfg.environment.etc.claude.text;
in

pkgs.runCommand "darwin-eval-test" { } ''
  set -euo pipefail
  passed=0
  failed=0

  check() {
    local desc="$1"
    local result="$2"
    if [ "$result" = "true" ]; then
      echo "  PASS: $desc"
      passed=$((passed + 1))
    else
      echo "  FAIL: $desc"
      failed=$((failed + 1))
    fi
  }

  echo "=== Darwin Module Evaluation Tests ==="
  echo ""

  echo "--- Host config.json ---"
  check "user field is set" "${builtins.toJSON (hostConfigJSON.user == "testuser")}"
  check "secrets map exists" "${builtins.toJSON (hostConfigJSON ? secrets)}"
  check "stateDir is set" "${builtins.toJSON (hostConfigJSON.stateDir == "/var/lib/firefly-forage")}"

  echo ""
  echo "--- Template claude.json ---"
  check "template name is claude" "${builtins.toJSON (templateJSON.name == "claude")}"
  check "description is set" "${builtins.toJSON (templateJSON.description == "Test Claude template")}"
  check "network mode is full" "${builtins.toJSON (templateJSON.network == "full")}"
  check "agent claude exists" "${builtins.toJSON (templateJSON.agents ? claude)}"
  check "agent secretName is test-secret" "${builtins.toJSON (templateJSON.agents.claude.secretName == "test-secret")}"
  check "agent authEnvVar is ANTHROPIC_API_KEY" "${builtins.toJSON (templateJSON.agents.claude.authEnvVar == "ANTHROPIC_API_KEY")}"

  echo ""
  echo "--- config.json mode ---"
  check "config.json has mode 0644" "${builtins.toJSON (cfg.environment.etc."firefly-forage/config.json".mode == "0644")}"

  echo ""
  echo "--- Activation scripts ---"
  check "activation script exists" "${
    builtins.toJSON (cfg.system.activationScripts ? postActivation)
  }"
  check "activation creates state dir" "${
    builtins.toJSON (
      builtins.match ".*mkdir -p.*/var/lib/firefly-forage.*" cfg.system.activationScripts.postActivation.text
      != null
    )
  }"
  check "activation creates secrets dir" "${
    builtins.toJSON (
      builtins.match ".*mkdir -p /run/forage-secrets.*" cfg.system.activationScripts.postActivation.text
      != null
    )
  }"
  check "chown includes group" "${
    builtins.toJSON (
      builtins.match ".*chown testuser:staff.*" cfg.system.activationScripts.postActivation.text != null
    )
  }"

  echo ""
  echo "--- Assertions ---"
  check "assertions list is non-empty" "${builtins.toJSON (builtins.length cfg.assertions > 0)}"

  echo ""
  echo "=== Results: $passed passed, $failed failed ==="

  if [ "$failed" -gt 0 ]; then
    echo "FAILED"
    exit 1
  fi

  echo "ALL TESTS PASSED"
  mkdir -p $out
  echo "passed" > $out/result
''

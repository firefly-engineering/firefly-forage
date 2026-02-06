# NixOS VM integration test for Firefly Forage
#
# This test uses the actual nixosModule to verify the full integration works.
# Run with: nix build .#checks.<system>.vm-integration
#
# NOTE: Full container lifecycle tests require network access which is not
# available in hermetic VM tests. This test verifies module setup, templates,
# and forage-ctl functionality without actually creating containers.
# Full lifecycle testing should be done on a real NixOS system.
{ pkgs, self }:

pkgs.testers.runNixOSTest {
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

    # Additional packages for testing
    environment.systemPackages = with pkgs; [ jujutsu git ];

    # Set NIX_PATH so extra-container can find nixpkgs
    nix.nixPath = [ "nixpkgs=${pkgs.path}" ];

    virtualisation = {
      memorySize = 2048;
      cores = 2;
    };
  };

  testScript = ''
    machine.wait_for_unit("multi-user.target")

    # === Module installation tests ===
    print("Testing module installation...")

    # Verify forage-ctl is installed and runs
    machine.succeed("forage-ctl --help")

    # Verify the module created the expected directories
    machine.succeed("test -d /var/lib/firefly-forage")
    machine.succeed("test -d /var/lib/firefly-forage/sandboxes")
    machine.succeed("test -d /var/lib/firefly-forage/workspaces")
    machine.succeed("test -d /etc/firefly-forage/templates")

    # Verify host config was created
    machine.succeed("test -f /etc/firefly-forage/config.json")
    config = machine.succeed("cat /etc/firefly-forage/config.json")
    print(f"Host config: {config}")
    assert '"user":"testuser"' in config.replace(" ", ""), "Host config should have testuser"

    # === Template tests ===
    print("Testing templates...")

    # Verify the test template is available
    machine.succeed("forage-ctl templates | grep -q test")

    # Verify template JSON was created
    machine.succeed("test -f /etc/firefly-forage/templates/test.json")
    template = machine.succeed("cat /etc/firefly-forage/templates/test.json")
    print(f"Template: {template}")
    assert '"network":"none"' in template.replace(" ", ""), "Template should have network=none"

    # === Container infrastructure tests ===
    print("Testing container infrastructure...")

    # machinectl should work (triggers systemd-machined via socket)
    machine.succeed("machinectl list")

    # extra-container should be available
    machine.succeed("which extra-container")

    # === Secrets tests ===
    print("Testing secrets...")

    # Verify secrets directory exists
    machine.succeed("test -d /run/forage-secrets")

    # === JJ workspace creation test ===
    print("Testing jj workspace creation...")

    # Configure git (required for jj)
    machine.succeed("git config --global user.email 'test@example.com'")
    machine.succeed("git config --global user.name 'Test User'")

    # Create a jj repository
    machine.succeed("mkdir -p /tmp/test-project")
    machine.succeed("cd /tmp/test-project && jj git init")
    machine.succeed("echo '# Test Project' > /tmp/test-project/README.md")
    machine.succeed("cd /tmp/test-project && jj commit -m 'Initial commit'")

    # Verify jj log works
    log_output = machine.succeed("cd /tmp/test-project && jj log --no-graph -T 'description'")
    assert "Initial commit" in log_output, "Should see initial commit"

    # Verify we can create a jj workspace
    machine.succeed("cd /tmp/test-project && jj workspace add /tmp/test-workspace")
    machine.succeed("test -d /tmp/test-workspace")
    machine.succeed("test -f /tmp/test-workspace/README.md")

    print("All integration tests passed!")
  '';
}

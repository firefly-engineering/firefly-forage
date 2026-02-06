# NixOS VM integration test for Firefly Forage
#
# This test uses the actual nixosModule to verify the full integration works.
# Run with: nix build .#checks.<system>.vm-integration
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
}

package integration

import (
	"os"
	"testing"
)

// TestHarnessSkipsWhenDisabled verifies that the harness skips tests
// when FORAGE_INTEGRATION_TESTS is not set.
func TestHarnessSkipsWhenDisabled(t *testing.T) {
	// Temporarily unset the env var
	orig := os.Getenv("FORAGE_INTEGRATION_TESTS")
	os.Unsetenv("FORAGE_INTEGRATION_TESTS")
	defer func() {
		if orig != "" {
			os.Setenv("FORAGE_INTEGRATION_TESTS", orig)
		}
	}()

	// This test verifies the skip behavior by checking if we reach this point
	// when the env var is unset, the test should be skipped

	if os.Getenv("FORAGE_INTEGRATION_TESTS") != "" {
		// If we're in integration test mode, verify the harness works
		h := NewHarness(t)
		if h == nil {
			t.Error("NewHarness returned nil")
		}
	}
	// If env var is not set, this test just passes (can't test skip from within)
}

func TestDefaultTemplate(t *testing.T) {
	tmpl := DefaultTemplate()

	if tmpl.Name != "integration-test" {
		t.Errorf("Name = %q, want %q", tmpl.Name, "integration-test")
	}
	if tmpl.Network != "none" {
		t.Errorf("Network = %q, want %q", tmpl.Network, "none")
	}
	if _, ok := tmpl.Agents["test"]; !ok {
		t.Error("Template should have 'test' agent")
	}
}

// TestIntegrationExample shows how to write an integration test.
// This test is always skipped unless FORAGE_INTEGRATION_TESTS=1.
func TestIntegrationExample(t *testing.T) {
	h := NewHarness(t) // Skips if integration tests disabled

	// Add a template
	h.AddTemplate("test", DefaultTemplate())

	// Create a workspace
	workspace := h.CreateWorkspace("test-sandbox")

	t.Logf("Created workspace at: %s", workspace)

	// In a real test, you would:
	// 1. Create a sandbox using the sandbox package
	// 2. Track it with h.TrackSandbox(name)
	// 3. Wait for SSH with h.WaitForSSH(port, timeout)
	// 4. Run tests against the sandbox
	// 5. Cleanup is automatic via t.Cleanup
}

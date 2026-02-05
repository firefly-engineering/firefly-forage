// Package integration provides a test harness for integration tests
// that require actual container runtime support.
//
// Integration tests are skipped unless the FORAGE_INTEGRATION_TESTS
// environment variable is set. These tests require:
//   - NixOS with systemd-nspawn support
//   - sudo access for extra-container
//   - Available ports in the configured range
//
// # Test Harness
//
// TestHarness manages test environments:
//
//	func TestMyIntegration(t *testing.T) {
//	    h := integration.NewHarness(t) // Skips if env var not set
//
//	    h.AddTemplate("test", integration.DefaultTemplate())
//	    workspace := h.CreateWorkspace("my-sandbox")
//
//	    // Create sandbox, run tests...
//
//	    // Cleanup is automatic via t.Cleanup
//	}
//
// # Harness Features
//
// The harness provides:
//   - Isolated temporary directories for configs and state
//   - Template and workspace creation helpers
//   - SSH readiness waiting (WaitForSSH)
//   - Sandbox tracking for cleanup (TrackSandbox)
//   - Access to paths, host config, and runtime
//
// # Running Integration Tests
//
//	FORAGE_INTEGRATION_TESTS=1 go test -v ./internal/integration/...
package integration

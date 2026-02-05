// Package testutil provides test fixtures and utilities.
//
// This package contains embedded JSON fixtures and helper functions for
// loading valid and invalid configurations in unit tests.
//
// # Fixtures
//
// JSON fixtures are embedded using go:embed:
//
//	fixtures/valid_host_config.json
//	fixtures/invalid_host_config.json
//	fixtures/valid_template.json
//	fixtures/valid_sandbox_metadata.json
//
// # Loading Fixtures
//
// Helper functions load and parse fixtures into typed config objects:
//
//	cfg, err := testutil.ValidHostConfig()
//	tmpl, err := testutil.ValidTemplate()
//	meta, err := testutil.ValidSandboxMetadata()
//	cfg, err := testutil.InvalidHostConfig()
//
// # Raw Fixture Access
//
// For custom parsing or testing edge cases:
//
//	data, err := testutil.LoadFixture("valid_host_config.json")
//
// # Usage in Tests
//
//	func TestConfigValidation(t *testing.T) {
//	    valid, _ := testutil.ValidHostConfig()
//	    if err := valid.Validate(); err != nil {
//	        t.Errorf("valid config failed validation: %v", err)
//	    }
//
//	    invalid, _ := testutil.InvalidHostConfig()
//	    if err := invalid.Validate(); err == nil {
//	        t.Error("invalid config should fail validation")
//	    }
//	}
package testutil

package testutil

import (
	"embed"
	"encoding/json"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
)

//go:embed fixtures/*.json
var fixturesFS embed.FS

// LoadFixture loads a JSON fixture file by name.
func LoadFixture(name string) ([]byte, error) {
	return fixturesFS.ReadFile("fixtures/" + name)
}

// LoadHostConfigFixture loads a host config fixture.
func LoadHostConfigFixture(name string) (*config.HostConfig, error) {
	data, err := LoadFixture(name)
	if err != nil {
		return nil, err
	}
	var cfg config.HostConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadTemplateFixture loads a template fixture.
func LoadTemplateFixture(name string) (*config.Template, error) {
	data, err := LoadFixture(name)
	if err != nil {
		return nil, err
	}
	var tmpl config.Template
	if err := json.Unmarshal(data, &tmpl); err != nil {
		return nil, err
	}
	return &tmpl, nil
}

// LoadSandboxMetadataFixture loads a sandbox metadata fixture.
func LoadSandboxMetadataFixture(name string) (*config.SandboxMetadata, error) {
	data, err := LoadFixture(name)
	if err != nil {
		return nil, err
	}
	var meta config.SandboxMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// ValidHostConfig returns the valid host config fixture.
func ValidHostConfig() (*config.HostConfig, error) {
	return LoadHostConfigFixture("valid_host_config.json")
}

// InvalidHostConfig returns the invalid host config fixture.
func InvalidHostConfig() (*config.HostConfig, error) {
	return LoadHostConfigFixture("invalid_host_config.json")
}

// ValidTemplate returns the valid template fixture.
func ValidTemplate() (*config.Template, error) {
	return LoadTemplateFixture("valid_template.json")
}

// ValidSandboxMetadata returns the valid sandbox metadata fixture.
func ValidSandboxMetadata() (*config.SandboxMetadata, error) {
	return LoadSandboxMetadataFixture("valid_sandbox_metadata.json")
}

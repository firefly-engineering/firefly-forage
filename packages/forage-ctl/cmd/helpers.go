package cmd

import (
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/errors"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
)

// paths returns the default paths configuration.
// This is a helper to reduce repetition in commands.
func paths() *config.Paths {
	return config.DefaultPaths()
}

// loadSandbox loads sandbox metadata or returns a SandboxNotFound error.
func loadSandbox(name string) (*config.SandboxMetadata, error) {
	p := paths()
	metadata, err := config.LoadSandboxMetadata(p.SandboxesDir, name)
	if err != nil {
		return nil, errors.SandboxNotFound(name)
	}
	return metadata, nil
}

// loadRunningSandbox loads sandbox metadata and verifies it's running.
// Returns SandboxNotFound if the sandbox doesn't exist,
// or SandboxNotRunning if it exists but isn't running.
func loadRunningSandbox(name string) (*config.SandboxMetadata, error) {
	metadata, err := loadSandbox(name)
	if err != nil {
		return nil, err
	}

	if !runtime.IsRunning(name) {
		return nil, errors.SandboxNotRunning(name)
	}

	return metadata, nil
}

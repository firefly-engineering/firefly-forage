package cmd

import (
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/app"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/errors"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
)

// paths returns the default paths configuration.
// This is a helper to reduce repetition in commands.
func paths() *config.Paths {
	return app.Default.Paths
}

// getRuntime returns the application runtime.
func getRuntime() runtime.Runtime {
	return app.Default.Runtime
}

// isRunning checks if a container is running using the app's runtime.
func isRunning(name string) bool {
	return app.Default.IsRunning(name)
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

	if !isRunning(name) {
		return nil, errors.SandboxNotRunning(name)
	}

	return metadata, nil
}

// listSandboxes lists all sandbox metadata.
func listSandboxes() ([]*config.SandboxMetadata, error) {
	return config.ListSandboxes(paths().SandboxesDir)
}

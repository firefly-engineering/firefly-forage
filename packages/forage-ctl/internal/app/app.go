// Package app provides the application context for forage-ctl.
// It allows dependency injection for testing.
package app

import (
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
)

// App holds the application dependencies
type App struct {
	// Paths holds the configured paths
	Paths *config.Paths

	// Runtime is the container runtime
	Runtime runtime.Runtime

	// HostConfig is the loaded host configuration
	HostConfig *config.HostConfig
}

// Option is a function that configures the App
type Option func(*App)

// WithPaths sets custom paths
func WithPaths(paths *config.Paths) Option {
	return func(a *App) {
		a.Paths = paths
	}
}

// WithRuntime sets a custom runtime
func WithRuntime(r runtime.Runtime) Option {
	return func(a *App) {
		a.Runtime = r
	}
}

// WithHostConfig sets a custom host config
func WithHostConfig(cfg *config.HostConfig) Option {
	return func(a *App) {
		a.HostConfig = cfg
	}
}

// New creates a new App with the given options
func New(opts ...Option) *App {
	app := &App{
		Paths: config.DefaultPaths(),
	}

	for _, opt := range opts {
		opt(app)
	}

	return app
}

// Default is the default application instance
var Default = New()

// SetDefault sets the default application instance (used for testing)
func SetDefault(app *App) {
	Default = app
}

// ResetDefault resets to the default application instance
func ResetDefault() {
	Default = New()
}

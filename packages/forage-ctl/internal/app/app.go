// Package app provides the application context for forage-ctl.
// It allows dependency injection for testing.
package app

import (
	"context"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
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

// New creates a new App with the given options.
// If runtime is not provided via WithRuntime, it will be auto-detected.
func New(opts ...Option) *App {
	app := &App{
		Paths: config.DefaultPaths(),
	}

	for _, opt := range opts {
		opt(app)
	}

	// Initialize runtime if not provided
	if app.Runtime == nil {
		cfg := &runtime.Config{
			Type:            runtime.RuntimeAuto,
			ContainerPrefix: config.ContainerPrefix,
			SandboxesDir:    app.Paths.SandboxesDir,
		}
		rt, err := runtime.New(cfg)
		if err != nil {
			logging.Debug("failed to initialize runtime", "error", err)
		} else {
			app.Runtime = rt
		}
	}

	return app
}

// IsRunning checks if a container is running using the app's runtime
func (a *App) IsRunning(name string) bool {
	if a.Runtime == nil {
		return false
	}
	running, _ := a.Runtime.IsRunning(context.Background(), name)
	return running
}

// Start starts a container using the app's runtime
func (a *App) Start(name string) error {
	if a.Runtime == nil {
		return nil
	}
	return a.Runtime.Start(context.Background(), name)
}

// Stop stops a container using the app's runtime
func (a *App) Stop(name string) error {
	if a.Runtime == nil {
		return nil
	}
	return a.Runtime.Stop(context.Background(), name)
}

// Destroy destroys a container using the app's runtime
func (a *App) Destroy(name string) error {
	if a.Runtime == nil {
		return nil
	}
	return a.Runtime.Destroy(context.Background(), name)
}

// Create creates a container using the app's runtime
func (a *App) Create(opts runtime.CreateOptions) error {
	if a.Runtime == nil {
		return nil
	}
	return a.Runtime.Create(context.Background(), opts)
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

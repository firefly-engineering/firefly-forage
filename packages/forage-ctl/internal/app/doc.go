// Package app provides the application context for forage-ctl.
//
// This package manages application-wide dependencies using the functional
// options pattern, enabling easy testing through dependency injection.
//
// # App Context
//
// The App struct holds core dependencies:
//
//	type App struct {
//	    Paths      *config.Paths      // File system paths
//	    Runtime    runtime.Runtime    // Container runtime
//	    HostConfig *config.HostConfig // Host configuration
//	}
//
// # Creating an App
//
// Use New with functional options:
//
//	// Production usage
//	app, err := app.New()
//
//	// Testing with custom dependencies
//	app, err := app.New(
//	    app.WithPaths(testPaths),
//	    app.WithRuntime(mockRuntime),
//	    app.WithHostConfig(testConfig),
//	)
//
// # Available Options
//
//	WithPaths(paths)        // Custom path configuration
//	WithRuntime(runtime)    // Custom container runtime
//	WithHostConfig(config)  // Custom host configuration
package app

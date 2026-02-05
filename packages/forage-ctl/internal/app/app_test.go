package app

import (
	"testing"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
)

func TestNew(t *testing.T) {
	app := New()

	if app == nil {
		t.Fatal("New() returned nil")
	}

	// Should have default paths
	if app.Paths == nil {
		t.Error("Paths should not be nil")
	}

	// Runtime and HostConfig may be nil by default
}

func TestNew_WithPaths(t *testing.T) {
	customPaths := &config.Paths{
		ConfigDir:     "/custom/config",
		StateDir:      "/custom/state",
		SecretsDir:    "/custom/secrets",
		SandboxesDir:  "/custom/sandboxes",
		WorkspacesDir: "/custom/workspaces",
		TemplatesDir:  "/custom/templates",
	}

	app := New(WithPaths(customPaths))

	if app.Paths != customPaths {
		t.Error("WithPaths did not set custom paths")
	}
}

func TestNew_WithRuntime(t *testing.T) {
	mockRuntime := runtime.NewMockRuntime()

	app := New(WithRuntime(mockRuntime))

	if app.Runtime != mockRuntime {
		t.Error("WithRuntime did not set runtime")
	}
}

func TestNew_WithHostConfig(t *testing.T) {
	customConfig := &config.HostConfig{
		User: "testuser",
		PortRange: config.PortRange{
			From: 3000,
			To:   3100,
		},
	}

	app := New(WithHostConfig(customConfig))

	if app.HostConfig != customConfig {
		t.Error("WithHostConfig did not set host config")
	}
}

func TestNew_MultipleOptions(t *testing.T) {
	customPaths := &config.Paths{ConfigDir: "/custom"}
	mockRuntime := runtime.NewMockRuntime()
	customConfig := &config.HostConfig{User: "test"}

	app := New(
		WithPaths(customPaths),
		WithRuntime(mockRuntime),
		WithHostConfig(customConfig),
	)

	if app.Paths != customPaths {
		t.Error("Paths not set correctly")
	}
	if app.Runtime != mockRuntime {
		t.Error("Runtime not set correctly")
	}
	if app.HostConfig != customConfig {
		t.Error("HostConfig not set correctly")
	}
}

func TestSetDefault(t *testing.T) {
	// Save original default
	original := Default
	defer func() { Default = original }()

	customApp := New(WithHostConfig(&config.HostConfig{User: "custom"}))
	SetDefault(customApp)

	if Default != customApp {
		t.Error("SetDefault did not update Default")
	}
}

func TestResetDefault(t *testing.T) {
	// Save original default
	original := Default
	defer func() { Default = original }()

	// Set a custom default
	customApp := New(WithHostConfig(&config.HostConfig{User: "custom"}))
	SetDefault(customApp)

	// Reset to default
	ResetDefault()

	// Should have a new default app with default paths
	if Default == customApp {
		t.Error("ResetDefault did not create new Default")
	}
	if Default.Paths == nil {
		t.Error("ResetDefault should create app with default paths")
	}
}

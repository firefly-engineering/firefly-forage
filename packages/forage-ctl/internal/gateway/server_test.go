package gateway

import (
	"testing"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/testutil"
)

func TestServer_HandleConnection_InvalidName(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	server := NewServer(env.Paths)

	// Test invalid sandbox names - should fail validation
	invalidNames := []string{
		"../escape",             // path traversal
		"My-Project",            // uppercase
		"has spaces",            // spaces
		"-starts-with-dash",     // starts with dash
		"has;semicolon",         // special characters
	}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			err := server.HandleConnection([]string{name})
			if err == nil {
				t.Errorf("HandleConnection(%q) should have failed with invalid name", name)
			}
		})
	}
}

func TestServer_HandleConnection_ValidNameNotFound(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	server := NewServer(env.Paths)

	// Valid name but sandbox doesn't exist
	err := server.HandleConnection([]string{"nonexistent"})
	if err == nil {
		t.Error("HandleConnection should fail for nonexistent sandbox")
	}
}

func TestServer_ConnectToSandbox_InvalidName(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	server := NewServer(env.Paths)

	// Test path traversal attack
	err := server.ConnectToSandbox("../../../etc/passwd")
	if err == nil {
		t.Error("ConnectToSandbox should fail for path traversal")
	}
}

func TestServer_ConnectToSandbox_NotRunning(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	// Set up mock runtime
	runtime.SetGlobal(env.Runtime)
	defer runtime.SetGlobal(nil)

	// Add sandbox metadata but don't mark it as running
	env.AddSandbox(&config.SandboxMetadata{
		Name:     "stopped-sandbox",
		Template: "test",
		Port:     2200,
	})

	// Mark as stopped in runtime
	env.Runtime.Containers["stopped-sandbox"].Status = runtime.StatusStopped

	server := NewServer(env.Paths)

	err := server.ConnectToSandbox("stopped-sandbox")
	if err == nil {
		t.Error("ConnectToSandbox should fail for stopped sandbox")
	}
}

func TestServer_ShowPicker_NoSandboxes(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	server := NewServer(env.Paths)

	// ShowPicker should not error when there are no sandboxes
	err := server.ShowPicker()
	if err != nil {
		t.Errorf("ShowPicker() failed: %v", err)
	}
}

func TestServer_ListSandboxes_Empty(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	server := NewServer(env.Paths)

	result, err := server.ListSandboxes()
	if err != nil {
		t.Errorf("ListSandboxes() failed: %v", err)
	}

	// Should return something (even if just a header)
	if result == "" {
		t.Error("ListSandboxes() returned empty string")
	}
}

func TestServer_ListSandboxes_WithSandboxes(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	// Add some sandboxes
	env.AddSandbox(&config.SandboxMetadata{
		Name:     "sandbox1",
		Template: "test",
		Port:     2200,
	})
	env.AddSandbox(&config.SandboxMetadata{
		Name:     "sandbox2",
		Template: "test",
		Port:     2201,
	})

	server := NewServer(env.Paths)

	result, err := server.ListSandboxes()
	if err != nil {
		t.Errorf("ListSandboxes() failed: %v", err)
	}

	// Should contain sandbox names
	if result == "" {
		t.Error("ListSandboxes() returned empty string")
	}
}

func TestNewServer(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	server := NewServer(env.Paths)

	if server == nil {
		t.Error("NewServer() returned nil")
	}

	if server.Paths != env.Paths {
		t.Error("Server.Paths not set correctly")
	}
}

package runtime

import (
	"context"
	"testing"
)

func TestNspawnRuntime_Name(t *testing.T) {
	rt := NewNspawnRuntime("/nix/store/.../extra-container", "forage-")

	if rt.Name() != "nspawn" {
		t.Errorf("Name() = %q, want %q", rt.Name(), "nspawn")
	}
}

func TestNspawnRuntime_containerName(t *testing.T) {
	rt := NewNspawnRuntime("/nix/store/.../extra-container", "forage-")

	tests := []struct {
		sandboxName string
		want        string
	}{
		{"myproject", "forage-myproject"},
		{"test-123", "forage-test-123"},
		{"", "forage-"},
	}

	for _, tt := range tests {
		t.Run(tt.sandboxName, func(t *testing.T) {
			got := rt.containerName(tt.sandboxName)
			if got != tt.want {
				t.Errorf("containerName(%q) = %q, want %q", tt.sandboxName, got, tt.want)
			}
		})
	}
}

func TestNspawnRuntime_containerName_CustomPrefix(t *testing.T) {
	rt := NewNspawnRuntime("/path/to/extra-container", "custom-prefix-")

	got := rt.containerName("sandbox")
	want := "custom-prefix-sandbox"
	if got != want {
		t.Errorf("containerName with custom prefix = %q, want %q", got, want)
	}
}

func TestNewNspawnRuntime(t *testing.T) {
	rt := NewNspawnRuntime("/nix/store/abc/extra-container", "test-")

	if rt == nil {
		t.Fatal("NewNspawnRuntime returned nil")
	}

	if rt.ExtraContainerPath != "/nix/store/abc/extra-container" {
		t.Errorf("ExtraContainerPath = %q, want %q",
			rt.ExtraContainerPath, "/nix/store/abc/extra-container")
	}

	if rt.ContainerPrefix != "test-" {
		t.Errorf("ContainerPrefix = %q, want %q", rt.ContainerPrefix, "test-")
	}

	if rt.sandboxPorts == nil {
		t.Error("sandboxPorts should be initialized")
	}
}

func TestNspawnRuntime_Interface(t *testing.T) {
	// Ensure NspawnRuntime implements Runtime interface
	var _ Runtime = (*NspawnRuntime)(nil)
}

func TestNspawnRuntime_SSHRuntime_Interface(t *testing.T) {
	// Ensure NspawnRuntime implements SSHRuntime interface
	var _ SSHRuntime = (*NspawnRuntime)(nil)
}

func TestNspawnRuntime_PortTracking(t *testing.T) {
	rt := NewNspawnRuntime("/path/to/extra-container", "forage-")
	ctx := context.Background()

	// Verify port map is empty initially
	if len(rt.sandboxPorts) != 0 {
		t.Errorf("sandboxPorts should be empty initially, got %d entries", len(rt.sandboxPorts))
	}

	// Manually add a port (simulating what Create would do)
	rt.sandboxPorts["test-sandbox"] = 2200

	// Verify port was stored
	if port, ok := rt.sandboxPorts["test-sandbox"]; !ok {
		t.Error("Port not stored for test-sandbox")
	} else if port != 2200 {
		t.Errorf("Port = %d, want %d", port, 2200)
	}

	// Verify SSHPort method returns the stored port
	if port, err := rt.SSHPort(ctx, "test-sandbox"); err != nil {
		t.Errorf("SSHPort() error: %v", err)
	} else if port != 2200 {
		t.Errorf("SSHPort() = %d, want %d", port, 2200)
	}

	// Verify SSHPort returns error for unknown sandbox
	if _, err := rt.SSHPort(ctx, "unknown"); err == nil {
		t.Error("SSHPort(unknown) should return error")
	}
}

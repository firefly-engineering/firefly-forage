package runtime

import (
	"context"
	"os"
	"testing"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
)

func TestNspawnRuntime_Name(t *testing.T) {
	rt := NewNspawnRuntime("/nix/store/.../extra-container", "forage-", "", "")

	if rt.Name() != "nspawn" {
		t.Errorf("Name() = %q, want %q", rt.Name(), "nspawn")
	}
}

func TestNspawnRuntime_containerName(t *testing.T) {
	rt := NewNspawnRuntime("/nix/store/.../extra-container", "forage-", "", "")

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
	rt := NewNspawnRuntime("/path/to/extra-container", "custom-prefix-", "", "")

	got := rt.containerName("sandbox")
	want := "custom-prefix-sandbox"
	if got != want {
		t.Errorf("containerName with custom prefix = %q, want %q", got, want)
	}
}

func TestNewNspawnRuntime(t *testing.T) {
	rt := NewNspawnRuntime("/nix/store/abc/extra-container", "test-", "/var/lib/forage/sandboxes", "")

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

	if rt.SandboxesDir != "/var/lib/forage/sandboxes" {
		t.Errorf("SandboxesDir = %q, want %q", rt.SandboxesDir, "/var/lib/forage/sandboxes")
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

func TestNspawnRuntime_SSHHost_FromMetadata(t *testing.T) {
	// Create temp directory for sandbox metadata
	tmpDir, err := os.MkdirTemp("", "nspawn-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create sandbox metadata with network slot
	metadata := &config.SandboxMetadata{
		Name:        "test-sandbox",
		NetworkSlot: 5,
		Template:    "test",
	}
	if err = config.SaveSandboxMetadata(tmpDir, metadata); err != nil {
		t.Fatalf("Failed to save sandbox metadata: %v", err)
	}

	rt := NewNspawnRuntime("/path/to/extra-container", "forage-", tmpDir, "")
	ctx := context.Background()

	// Verify SSHHost loads from metadata and returns container IP
	host, err := rt.SSHHost(ctx, "test-sandbox")
	if err != nil {
		t.Errorf("SSHHost() error: %v", err)
	} else if host != "10.100.5.2" {
		t.Errorf("SSHHost() = %q, want %q", host, "10.100.5.2")
	}
}

func TestNspawnRuntime_SSHHost_NotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nspawn-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	rt := NewNspawnRuntime("/path/to/extra-container", "forage-", tmpDir, "")
	ctx := context.Background()

	// Verify SSHHost returns error for unknown sandbox
	if _, err := rt.SSHHost(ctx, "unknown"); err == nil {
		t.Error("SSHHost(unknown) should return error")
	}
}

func TestNspawnRuntime_SSHHost_NoSandboxesDir(t *testing.T) {
	rt := NewNspawnRuntime("/path/to/extra-container", "forage-", "", "")
	ctx := context.Background()

	// Verify SSHHost returns error when sandboxes dir not configured
	_, err := rt.SSHHost(ctx, "test")
	if err == nil {
		t.Error("SSHHost should return error when sandboxes dir not configured")
	}
}

func TestNspawnRuntime_SSHExec(t *testing.T) {
	// Create temp directory for sandbox metadata
	tmpDir, err := os.MkdirTemp("", "nspawn-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create sandbox metadata
	metadata := &config.SandboxMetadata{
		Name:        "test-sandbox",
		NetworkSlot: 1,
		Template:    "test",
	}
	if err = config.SaveSandboxMetadata(tmpDir, metadata); err != nil {
		t.Fatalf("Failed to save sandbox metadata: %v", err)
	}

	rt := NewNspawnRuntime("/path/to/extra-container", "forage-", tmpDir, "")
	ctx := context.Background()

	// SSHExec will fail because SSH isn't actually running,
	// but it should get the host correctly from metadata
	_, err = rt.SSHExec(ctx, "test-sandbox", []string{"echo", "test"}, ExecOptions{})
	// We expect an error since SSH isn't running, but it shouldn't be about metadata lookup
	if err != nil && err.Error() == "failed to load sandbox metadata: no sandbox named \"test-sandbox\" found" {
		t.Errorf("SSHExec failed to load metadata: %v", err)
	}
}

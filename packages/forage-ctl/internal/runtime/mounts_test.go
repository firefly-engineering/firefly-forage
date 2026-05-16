package runtime

import (
	"strings"
	"testing"
)

func TestNewStandardMounts(t *testing.T) {
	mounts := NewStandardMounts("/workspace/test", "/run/secrets/test", "")

	if mounts.NixStore == nil {
		t.Error("NixStore mount should not be nil")
	}
	if mounts.NixStore.Target != "/nix/store" {
		t.Errorf("NixStore target = %s, want /nix/store", mounts.NixStore.Target)
	}
	if !mounts.NixStore.ReadOnly {
		t.Error("NixStore should be read-only")
	}

	if mounts.Workspace == nil {
		t.Error("Workspace mount should not be nil")
	}
	if mounts.Workspace.Source != "/workspace/test" {
		t.Errorf("Workspace source = %s, want /workspace/test", mounts.Workspace.Source)
	}
	if mounts.Workspace.Target != "/workspace" {
		t.Errorf("Workspace target = %s, want /workspace", mounts.Workspace.Target)
	}

	if mounts.Secrets == nil {
		t.Error("Secrets mount should not be nil")
	}
	if !mounts.Secrets.ReadOnly {
		t.Error("Secrets should be read-only")
	}

	// No source repo provided
	if mounts.SourceRepo != nil {
		t.Error("SourceRepo should be nil when no source repo provided")
	}
}

func TestStandardMountsToDockerArgs(t *testing.T) {
	mounts := NewStandardMounts("/workspace/test", "/run/secrets/test", "")
	args := mounts.ToDockerArgs()

	// Should have at least 4 mounts (nix store, daemon socket, workspace, secrets)
	if len(args) < 8 { // Each mount is -v + value
		t.Errorf("Expected at least 8 args, got %d", len(args))
	}

	// Check for nix store mount with :ro
	found := false
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-v" && strings.Contains(args[i+1], "/nix/store") {
			if !strings.HasSuffix(args[i+1], ":ro") {
				t.Error("Nix store mount should be read-only")
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("Nix store mount not found in Docker args")
	}
}

func TestStandardMountsToAppleArgs(t *testing.T) {
	mounts := NewStandardMounts("/workspace/test", "/run/secrets/test", "")
	args := mounts.ToAppleArgs()

	// Should have at least 4 mounts
	if len(args) < 8 { // Each mount is --mount + value
		t.Errorf("Expected at least 8 args, got %d", len(args))
	}

	// Check for nix store mount with readonly
	found := false
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--mount" && strings.Contains(args[i+1], "/nix/store") {
			if !strings.Contains(args[i+1], "readonly") {
				t.Error("Nix store mount should be read-only")
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("Nix store mount not found in Apple args")
	}
}

func TestDetectPlatformMountConfig(t *testing.T) {
	config := DetectPlatformMountConfig()

	if config.NixStorePath != "/nix/store" {
		t.Errorf("NixStorePath = %s, want /nix/store", config.NixStorePath)
	}

	if config.NixDaemonSocketPath != "/nix/var/nix/daemon-socket" {
		t.Errorf("NixDaemonSocketPath = %s, want /nix/var/nix/daemon-socket", config.NixDaemonSocketPath)
	}
}

package runtime

import (
	"context"
	"testing"
	"time"
)

func TestNspawnCapabilities(t *testing.T) {
	rt := &NspawnRuntime{}
	caps := rt.Capabilities()

	if !caps.NixOSConfig {
		t.Error("nspawn should support NixOSConfig")
	}
	if !caps.NetworkIsolation {
		t.Error("nspawn should support NetworkIsolation")
	}
	if !caps.EphemeralRoot {
		t.Error("nspawn should support EphemeralRoot")
	}
	if !caps.SSHAccess {
		t.Error("nspawn should support SSHAccess")
	}
	if !caps.GeneratedFiles {
		t.Error("nspawn should support GeneratedFiles")
	}
	if !caps.ResourceLimits {
		t.Error("nspawn should support ResourceLimits")
	}
	if !caps.GracefulShutdown {
		t.Error("nspawn should support GracefulShutdown")
	}
}

func TestDockerCapabilities(t *testing.T) {
	rt := &DockerRuntime{Command: "docker"}
	caps := rt.Capabilities()

	if caps.NixOSConfig {
		t.Error("docker should not support NixOSConfig")
	}
	if caps.NetworkIsolation {
		t.Error("docker should not support NetworkIsolation")
	}
	if !caps.EphemeralRoot {
		t.Error("docker should support EphemeralRoot")
	}
	if caps.SSHAccess {
		t.Error("docker should not support SSHAccess")
	}
	if !caps.GeneratedFiles {
		t.Error("docker should support GeneratedFiles")
	}
	if !caps.ResourceLimits {
		t.Error("docker should support ResourceLimits")
	}
	if !caps.GracefulShutdown {
		t.Error("docker should support GracefulShutdown")
	}
}

func TestGetCapabilities_WithCapableRuntime(t *testing.T) {
	rt := &NspawnRuntime{}
	caps := GetCapabilities(rt)

	if !caps.NixOSConfig {
		t.Error("GetCapabilities should return nspawn capabilities")
	}
}

func TestGracefulStopper_MockRuntime(t *testing.T) {
	mock := NewMockRuntime()
	ctx := context.Background()

	mock.AddContainer("test", StatusRunning)

	err := mock.GracefulStop(ctx, "test", 5*time.Second)
	if err != nil {
		t.Fatalf("GracefulStop failed: %v", err)
	}

	running, _ := mock.IsRunning(ctx, "test")
	if running {
		t.Error("Container should be stopped after GracefulStop")
	}

	calls := mock.GetCallsFor("GracefulStop")
	if len(calls) != 1 {
		t.Errorf("Expected 1 GracefulStop call, got %d", len(calls))
	}
}

func TestGracefulStopper_ViaInterface(t *testing.T) {
	// Test the pattern: check if runtime implements GracefulStopper via interface
	mock := NewMockRuntime()
	ctx := context.Background()
	mock.AddContainer("test", StatusRunning)

	var rt Runtime = mock
	if gs, ok := rt.(GracefulStopper); ok {
		err := gs.GracefulStop(ctx, "test", 10*time.Second)
		if err != nil {
			t.Fatalf("GracefulStop failed: %v", err)
		}
	} else {
		t.Fatal("MockRuntime should implement GracefulStopper")
	}
}

func TestGetCapabilities_WithNonCapableRuntime(t *testing.T) {
	// MockRuntime doesn't implement CapableRuntime
	rt := NewMockRuntime()
	caps := GetCapabilities(rt)

	// All should default to true
	if !caps.NixOSConfig {
		t.Error("default capabilities should have NixOSConfig true")
	}
	if !caps.NetworkIsolation {
		t.Error("default capabilities should have NetworkIsolation true")
	}
	if !caps.EphemeralRoot {
		t.Error("default capabilities should have EphemeralRoot true")
	}
	if !caps.SSHAccess {
		t.Error("default capabilities should have SSHAccess true")
	}
	if !caps.GeneratedFiles {
		t.Error("default capabilities should have GeneratedFiles true")
	}
	if !caps.ResourceLimits {
		t.Error("default capabilities should have ResourceLimits true")
	}
	if !caps.GracefulShutdown {
		t.Error("default capabilities should have GracefulShutdown true")
	}
}

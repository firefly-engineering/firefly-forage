package monitor

import (
	"context"
	"testing"
	"time"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/audit"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
)

func TestMonitor_New(t *testing.T) {
	rt := runtime.NewMockRuntime()
	paths := &config.Paths{
		SandboxesDir: t.TempDir(),
		StateDir:     t.TempDir(),
	}

	m := New(30*time.Second, rt, paths)
	if m.interval != 30*time.Second {
		t.Errorf("interval = %v, want %v", m.interval, 30*time.Second)
	}
	if m.autoRestart {
		t.Error("autoRestart should default to false")
	}
	if m.auditLog != nil {
		t.Error("auditLog should default to nil")
	}
}

func TestMonitor_Options(t *testing.T) {
	rt := runtime.NewMockRuntime()
	paths := &config.Paths{
		SandboxesDir: t.TempDir(),
		StateDir:     t.TempDir(),
	}
	auditLogger := audit.NewLogger(paths.StateDir)

	m := New(60*time.Second, rt, paths,
		WithAutoRestart(true),
		WithAuditLogger(auditLogger),
	)

	if !m.autoRestart {
		t.Error("autoRestart should be true")
	}
	if m.auditLog == nil {
		t.Error("auditLog should be set")
	}
}

func TestMonitor_CheckAllEmpty(t *testing.T) {
	rt := runtime.NewMockRuntime()
	paths := &config.Paths{
		SandboxesDir: t.TempDir(),
		StateDir:     t.TempDir(),
	}

	m := New(time.Second, rt, paths)
	ctx := context.Background()

	results := m.checkAll(ctx)
	if len(results) != 0 {
		t.Errorf("got %d results, want 0 for empty sandboxes dir", len(results))
	}
}

func TestMonitor_CheckAllWithSandbox(t *testing.T) {
	rt := runtime.NewMockRuntime()
	sandboxesDir := t.TempDir()
	stateDir := t.TempDir()
	paths := &config.Paths{
		SandboxesDir: sandboxesDir,
		StateDir:     stateDir,
	}

	// Create a sandbox metadata file
	metadata := &config.SandboxMetadata{
		Name:        "test-sandbox",
		Template:    "test",
		NetworkSlot: 1,
		Multiplexer: "tmux",
	}
	if err := config.SaveSandboxMetadata(sandboxesDir, metadata); err != nil {
		t.Fatalf("failed to save sandbox metadata: %v", err)
	}

	auditLogger := audit.NewLogger(stateDir)
	m := New(time.Second, rt, paths, WithAuditLogger(auditLogger))
	ctx := context.Background()

	results := m.checkAll(ctx)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Sandbox != "test-sandbox" {
		t.Errorf("sandbox = %q, want %q", results[0].Sandbox, "test-sandbox")
	}

	// Verify audit event was logged
	events, err := auditLogger.Events("test-sandbox")
	if err != nil {
		t.Fatalf("Events failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d audit events, want 1", len(events))
	}
	if events[0].Type != audit.EventHealth {
		t.Errorf("event type = %q, want %q", events[0].Type, audit.EventHealth)
	}
}

func TestMonitor_RunCancellation(t *testing.T) {
	rt := runtime.NewMockRuntime()
	paths := &config.Paths{
		SandboxesDir: t.TempDir(),
		StateDir:     t.TempDir(),
	}

	m := New(100*time.Millisecond, rt, paths)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- m.Run(ctx)
	}()

	// Let it run briefly then cancel
	time.Sleep(250 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not stop after context cancellation")
	}
}

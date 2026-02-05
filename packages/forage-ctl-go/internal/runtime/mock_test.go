package runtime

import (
	"context"
	"fmt"
	"testing"
)

func TestMockRuntime_Create(t *testing.T) {
	mock := NewMockRuntime()
	ctx := context.Background()

	opts := CreateOptions{
		Name:       "test-container",
		ConfigPath: "/path/to/config.nix",
		Start:      true,
	}

	err := mock.Create(ctx, opts)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify container was created
	info, _ := mock.Status(ctx, "test-container")
	if info.Status != StatusRunning {
		t.Errorf("Status = %v, want %v", info.Status, StatusRunning)
	}

	// Verify call was logged
	calls := mock.GetCallsFor("Create")
	if len(calls) != 1 {
		t.Errorf("len(calls) = %d, want 1", len(calls))
	}
}

func TestMockRuntime_CreateWithoutStart(t *testing.T) {
	mock := NewMockRuntime()
	ctx := context.Background()

	opts := CreateOptions{
		Name:  "test-container",
		Start: false,
	}

	err := mock.Create(ctx, opts)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	info, _ := mock.Status(ctx, "test-container")
	if info.Status != StatusStopped {
		t.Errorf("Status = %v, want %v", info.Status, StatusStopped)
	}
}

func TestMockRuntime_CreateWithError(t *testing.T) {
	mock := NewMockRuntime()
	ctx := context.Background()

	expectedErr := fmt.Errorf("creation failed")
	mock.SetError("Create", expectedErr)

	err := mock.Create(ctx, CreateOptions{Name: "test"})
	if err != expectedErr {
		t.Errorf("err = %v, want %v", err, expectedErr)
	}
}

func TestMockRuntime_StartStop(t *testing.T) {
	mock := NewMockRuntime()
	ctx := context.Background()

	// Add a stopped container
	mock.AddContainer("test", StatusStopped)

	// Start it
	if err := mock.Start(ctx, "test"); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	running, _ := mock.IsRunning(ctx, "test")
	if !running {
		t.Error("Container should be running after Start")
	}

	// Stop it
	if err := mock.Stop(ctx, "test"); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	running, _ = mock.IsRunning(ctx, "test")
	if running {
		t.Error("Container should not be running after Stop")
	}
}

func TestMockRuntime_StartNonexistent(t *testing.T) {
	mock := NewMockRuntime()
	ctx := context.Background()

	err := mock.Start(ctx, "nonexistent")
	if err == nil {
		t.Error("Start should fail for nonexistent container")
	}
}

func TestMockRuntime_Destroy(t *testing.T) {
	mock := NewMockRuntime()
	ctx := context.Background()

	mock.AddContainer("test", StatusRunning)

	if err := mock.Destroy(ctx, "test"); err != nil {
		t.Fatalf("Destroy failed: %v", err)
	}

	info, _ := mock.Status(ctx, "test")
	if info.Status != StatusNotFound {
		t.Errorf("Status = %v, want %v", info.Status, StatusNotFound)
	}
}

func TestMockRuntime_IsRunning(t *testing.T) {
	mock := NewMockRuntime()
	ctx := context.Background()

	mock.AddContainer("running", StatusRunning)
	mock.AddContainer("stopped", StatusStopped)

	tests := []struct {
		name string
		want bool
	}{
		{"running", true},
		{"stopped", false},
		{"nonexistent", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			running, err := mock.IsRunning(ctx, tt.name)
			if err != nil {
				t.Fatalf("IsRunning failed: %v", err)
			}
			if running != tt.want {
				t.Errorf("IsRunning(%q) = %v, want %v", tt.name, running, tt.want)
			}
		})
	}
}

func TestMockRuntime_Status(t *testing.T) {
	mock := NewMockRuntime()
	ctx := context.Background()

	mock.AddContainer("test", StatusRunning)

	info, err := mock.Status(ctx, "test")
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}

	if info.Name != "test" {
		t.Errorf("Name = %q, want %q", info.Name, "test")
	}
	if info.Status != StatusRunning {
		t.Errorf("Status = %v, want %v", info.Status, StatusRunning)
	}
}

func TestMockRuntime_Exec(t *testing.T) {
	mock := NewMockRuntime()
	ctx := context.Background()

	mock.AddContainer("test", StatusRunning)
	mock.SetExecResult("test", &ExecResult{
		ExitCode: 0,
		Stdout:   "hello world",
		Stderr:   "",
	})

	result, err := mock.Exec(ctx, "test", []string{"echo", "hello"}, ExecOptions{})
	if err != nil {
		t.Fatalf("Exec failed: %v", err)
	}

	if result.Stdout != "hello world" {
		t.Errorf("Stdout = %q, want %q", result.Stdout, "hello world")
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestMockRuntime_ExecDefault(t *testing.T) {
	mock := NewMockRuntime()
	ctx := context.Background()

	mock.AddContainer("test", StatusRunning)

	// Without setting a result, should return default
	result, err := mock.Exec(ctx, "test", []string{"command"}, ExecOptions{})
	if err != nil {
		t.Fatalf("Exec failed: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if result.Stdout != "" {
		t.Errorf("Stdout = %q, want empty", result.Stdout)
	}
}

func TestMockRuntime_List(t *testing.T) {
	mock := NewMockRuntime()
	ctx := context.Background()

	mock.AddContainer("container-1", StatusRunning)
	mock.AddContainer("container-2", StatusStopped)
	mock.AddContainer("container-3", StatusRunning)

	containers, err := mock.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(containers) != 3 {
		t.Errorf("len(containers) = %d, want 3", len(containers))
	}
}

func TestMockRuntime_GetCalls(t *testing.T) {
	mock := NewMockRuntime()
	ctx := context.Background()

	mock.Create(ctx, CreateOptions{Name: "test1"})
	mock.Create(ctx, CreateOptions{Name: "test2"})
	mock.Start(ctx, "test1")

	calls := mock.GetCalls()
	if len(calls) != 3 {
		t.Errorf("len(calls) = %d, want 3", len(calls))
	}

	createCalls := mock.GetCallsFor("Create")
	if len(createCalls) != 2 {
		t.Errorf("len(createCalls) = %d, want 2", len(createCalls))
	}

	startCalls := mock.GetCallsFor("Start")
	if len(startCalls) != 1 {
		t.Errorf("len(startCalls) = %d, want 1", len(startCalls))
	}
}

func TestMockRuntime_Reset(t *testing.T) {
	mock := NewMockRuntime()
	ctx := context.Background()

	mock.AddContainer("test", StatusRunning)
	mock.SetError("Create", fmt.Errorf("error"))
	mock.Create(ctx, CreateOptions{Name: "another"})

	// Reset
	mock.Reset()

	// Verify everything is cleared
	containers, _ := mock.List(ctx)
	if len(containers) != 0 {
		t.Errorf("len(containers) = %d, want 0 after reset", len(containers))
	}

	// Note: List() call from above should be recorded after Reset
	if len(mock.CallLog) != 1 { // Just the List call after Reset
		t.Errorf("len(CallLog) = %d after reset + List", len(mock.CallLog))
	}

	// Error should be cleared
	err := mock.Create(ctx, CreateOptions{Name: "test"})
	if err != nil {
		t.Errorf("Create should succeed after reset: %v", err)
	}
}

func TestMockRuntime_Name(t *testing.T) {
	mock := NewMockRuntime()
	if mock.Name() != "mock" {
		t.Errorf("Name() = %q, want %q", mock.Name(), "mock")
	}
}

func TestMockRuntime_ExecInteractive(t *testing.T) {
	mock := NewMockRuntime()
	ctx := context.Background()

	err := mock.ExecInteractive(ctx, "test", []string{"bash"}, ExecOptions{})
	if err != nil {
		t.Errorf("ExecInteractive should not error: %v", err)
	}

	// With error
	mock.SetError("ExecInteractive", fmt.Errorf("tty error"))
	err = mock.ExecInteractive(ctx, "test", []string{"bash"}, ExecOptions{})
	if err == nil {
		t.Error("ExecInteractive should return injected error")
	}
}

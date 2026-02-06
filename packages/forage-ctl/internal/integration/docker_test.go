// Package integration provides integration tests that exercise complete code paths.
//
// Docker integration tests require:
// - Docker daemon running
// - User in docker group (or docker accessible)
// - FORAGE_INTEGRATION_TESTS=1 and FORAGE_RUNTIME=docker environment variables
//
// Run with: FORAGE_INTEGRATION_TESTS=1 FORAGE_RUNTIME=docker go test -v ./internal/integration/...
package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
)

// skipUnlessDockerEnabled skips the test unless docker integration is enabled
func skipUnlessDockerEnabled(t *testing.T) {
	t.Helper()

	if os.Getenv("FORAGE_INTEGRATION_TESTS") != "1" {
		t.Skip("integration tests disabled (set FORAGE_INTEGRATION_TESTS=1)")
	}

	if os.Getenv("FORAGE_RUNTIME") != "docker" {
		t.Skip("docker runtime not selected (set FORAGE_RUNTIME=docker)")
	}

	// Verify docker is available
	rt, err := runtime.NewDockerRuntime("forage-test-")
	if err != nil {
		t.Skipf("docker runtime not available: %v", err)
	}

	// Quick check that docker is responsive
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = rt.List(ctx)
	if err != nil {
		t.Skipf("docker not responsive: %v", err)
	}
}

// TestDocker_ContainerLifecycle tests the complete container lifecycle with Docker
func TestDocker_ContainerLifecycle(t *testing.T) {
	skipUnlessDockerEnabled(t)

	rt, err := runtime.NewDockerRuntime("forage-test-")
	if err != nil {
		t.Fatalf("failed to create docker runtime: %v", err)
	}

	ctx := context.Background()
	sandboxName := "lifecycle-test"

	// Cleanup any leftover containers from previous runs
	_ = rt.Destroy(ctx, sandboxName)

	// Create container
	t.Log("Creating container...")
	err = rt.Create(ctx, runtime.CreateOptions{
		Name:  sandboxName,
		Start: true,
	})
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}

	// Verify it's running
	t.Log("Verifying container is running...")
	running, err := rt.IsRunning(ctx, sandboxName)
	if err != nil {
		t.Errorf("IsRunning failed: %v", err)
	}
	if !running {
		t.Error("container should be running after Create with Start=true")
	}

	// Get status
	t.Log("Getting container status...")
	status, err := rt.Status(ctx, sandboxName)
	if err != nil {
		t.Errorf("Status failed: %v", err)
	}
	if status.Status != runtime.StatusRunning {
		t.Errorf("expected StatusRunning, got %v", status.Status)
	}

	// Execute a command
	t.Log("Executing command in container...")
	result, err := rt.Exec(ctx, sandboxName, []string{"echo", "hello"}, runtime.ExecOptions{})
	if err != nil {
		t.Errorf("Exec failed: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.Stdout != "hello\n" {
		t.Errorf("expected 'hello\\n', got %q", result.Stdout)
	}

	// Stop container
	t.Log("Stopping container...")
	err = rt.Stop(ctx, sandboxName)
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}

	// Verify it's stopped
	running, _ = rt.IsRunning(ctx, sandboxName)
	if running {
		t.Error("container should not be running after Stop")
	}

	// Start again
	t.Log("Starting container...")
	err = rt.Start(ctx, sandboxName)
	if err != nil {
		t.Errorf("Start failed: %v", err)
	}

	running, _ = rt.IsRunning(ctx, sandboxName)
	if !running {
		t.Error("container should be running after Start")
	}

	// Destroy container
	t.Log("Destroying container...")
	err = rt.Destroy(ctx, sandboxName)
	if err != nil {
		t.Errorf("Destroy failed: %v", err)
	}

	// Verify it's gone
	running, _ = rt.IsRunning(ctx, sandboxName)
	if running {
		t.Error("container should not exist after Destroy")
	}

	t.Log("Container lifecycle test passed!")
}

// TestDocker_List tests listing containers
func TestDocker_List(t *testing.T) {
	skipUnlessDockerEnabled(t)

	rt, err := runtime.NewDockerRuntime("forage-test-")
	if err != nil {
		t.Fatalf("failed to create docker runtime: %v", err)
	}

	ctx := context.Background()

	// Cleanup any leftover containers
	_ = rt.Destroy(ctx, "list-test-1")
	_ = rt.Destroy(ctx, "list-test-2")

	// Create two containers
	for _, name := range []string{"list-test-1", "list-test-2"} {
		err = rt.Create(ctx, runtime.CreateOptions{
			Name:  name,
			Start: true,
		})
		if err != nil {
			t.Fatalf("failed to create container %s: %v", name, err)
		}
	}

	// List containers
	containers, err := rt.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	// Should have at least 2 containers
	found := 0
	for _, c := range containers {
		if c.Name == "list-test-1" || c.Name == "list-test-2" {
			found++
		}
	}

	if found != 2 {
		t.Errorf("expected to find 2 containers, found %d", found)
	}

	// Cleanup
	_ = rt.Destroy(ctx, "list-test-1")
	_ = rt.Destroy(ctx, "list-test-2")
}

// TestDocker_BindMounts tests container bind mounts
func TestDocker_BindMounts(t *testing.T) {
	skipUnlessDockerEnabled(t)

	rt, err := runtime.NewDockerRuntime("forage-test-")
	if err != nil {
		t.Fatalf("failed to create docker runtime: %v", err)
	}

	ctx := context.Background()
	sandboxName := "bindmount-test"

	// Cleanup
	_ = rt.Destroy(ctx, sandboxName)

	// Create a temp file to mount
	tmpDir := t.TempDir()
	testFile := tmpDir + "/test.txt"
	if err := os.WriteFile(testFile, []byte("bind mount test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create container with bind mount
	err = rt.Create(ctx, runtime.CreateOptions{
		Name:  sandboxName,
		Start: true,
		BindMounts: map[string]string{
			tmpDir: "/workspace",
		},
	})
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}

	// Verify we can read the mounted file
	result, err := rt.Exec(ctx, sandboxName, []string{"cat", "/workspace/test.txt"}, runtime.ExecOptions{})
	if err != nil {
		t.Errorf("Exec failed: %v", err)
	}
	if result.Stdout != "bind mount test" {
		t.Errorf("expected 'bind mount test', got %q", result.Stdout)
	}

	// Cleanup
	_ = rt.Destroy(ctx, sandboxName)
}

// TestDocker_ExecWithOptions tests exec with various options
func TestDocker_ExecWithOptions(t *testing.T) {
	skipUnlessDockerEnabled(t)

	rt, err := runtime.NewDockerRuntime("forage-test-")
	if err != nil {
		t.Fatalf("failed to create docker runtime: %v", err)
	}

	ctx := context.Background()
	sandboxName := "exec-options-test"

	// Cleanup and create
	_ = rt.Destroy(ctx, sandboxName)
	err = rt.Create(ctx, runtime.CreateOptions{
		Name:  sandboxName,
		Start: true,
	})
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}

	t.Run("working directory", func(t *testing.T) {
		result, err := rt.Exec(ctx, sandboxName, []string{"pwd"}, runtime.ExecOptions{
			WorkingDir: "/tmp",
		})
		if err != nil {
			t.Errorf("Exec failed: %v", err)
		}
		if result.Stdout != "/tmp\n" {
			t.Errorf("expected '/tmp\\n', got %q", result.Stdout)
		}
	})

	t.Run("environment variables", func(t *testing.T) {
		result, err := rt.Exec(ctx, sandboxName, []string{"sh", "-c", "echo $MY_VAR"}, runtime.ExecOptions{
			Env: []string{"MY_VAR=test_value"},
		})
		if err != nil {
			t.Errorf("Exec failed: %v", err)
		}
		if result.Stdout != "test_value\n" {
			t.Errorf("expected 'test_value\\n', got %q", result.Stdout)
		}
	})

	// Cleanup
	_ = rt.Destroy(ctx, sandboxName)
}

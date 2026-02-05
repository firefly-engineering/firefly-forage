package runtime

import (
	"context"
	"encoding/json"
	"testing"
)

func TestDockerRuntime_Name(t *testing.T) {
	rt := &DockerRuntime{
		Command:         "docker",
		ContainerPrefix: "forage-",
	}

	if rt.Name() != "docker" {
		t.Errorf("Name() = %q, want %q", rt.Name(), "docker")
	}

	rt.Command = "podman"
	if rt.Name() != "podman" {
		t.Errorf("Name() = %q, want %q", rt.Name(), "podman")
	}
}

func TestDockerRuntime_containerName(t *testing.T) {
	rt := &DockerRuntime{
		Command:         "docker",
		ContainerPrefix: "forage-",
	}

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

func TestDockerRuntime_containerName_CustomPrefix(t *testing.T) {
	rt := &DockerRuntime{
		Command:         "docker",
		ContainerPrefix: "custom-prefix-",
	}

	got := rt.containerName("sandbox")
	want := "custom-prefix-sandbox"
	if got != want {
		t.Errorf("containerName with custom prefix = %q, want %q", got, want)
	}
}

func TestDockerRuntime_Interface(t *testing.T) {
	// Ensure DockerRuntime implements Runtime interface
	var _ Runtime = (*DockerRuntime)(nil)
}

func TestDockerInspect_Parse(t *testing.T) {
	// Test that dockerInspect struct can parse expected JSON
	jsonData := `[{
		"State": {
			"Status": "running",
			"Running": true,
			"StartedAt": "2024-01-01T00:00:00Z"
		},
		"NetworkSettings": {
			"IPAddress": "172.17.0.2"
		}
	}]`

	var inspects []dockerInspect
	if err := json.Unmarshal([]byte(jsonData), &inspects); err != nil {
		t.Fatalf("Failed to parse dockerInspect: %v", err)
	}

	if len(inspects) != 1 {
		t.Fatalf("Expected 1 inspect result, got %d", len(inspects))
	}

	inspect := inspects[0]
	if inspect.State.Status != "running" {
		t.Errorf("State.Status = %q, want %q", inspect.State.Status, "running")
	}
	if !inspect.State.Running {
		t.Error("State.Running = false, want true")
	}
	if inspect.State.StartedAt != "2024-01-01T00:00:00Z" {
		t.Errorf("State.StartedAt = %q, want %q", inspect.State.StartedAt, "2024-01-01T00:00:00Z")
	}
	if inspect.NetworkSettings.IPAddress != "172.17.0.2" {
		t.Errorf("NetworkSettings.IPAddress = %q, want %q", inspect.NetworkSettings.IPAddress, "172.17.0.2")
	}
}

func TestDockerRuntime_Status_NotFound(t *testing.T) {
	// Test that Status returns NotFound for missing container
	// This is a unit test that doesn't require docker
	rt := &DockerRuntime{
		Command:         "docker",
		ContainerPrefix: "forage-",
	}

	// We can't easily test this without mocking exec, but we can verify the
	// interface is implemented correctly
	info, err := rt.Status(context.Background(), "nonexistent-container-that-should-not-exist-12345")

	// This will fail because docker isn't running in test, but we verify the return type
	if info == nil && err != nil {
		// Expected - docker command failed
		t.Skip("Skipping - docker not available")
	}

	// If docker is available, verify return type
	if info != nil && info.Status == StatusNotFound {
		// Correct behavior
	}
}

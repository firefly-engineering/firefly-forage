package injection

import (
	"context"
	"testing"
)

func TestWorkspaceMountsContributor_ContributeMounts(t *testing.T) {
	mounts := []ResolvedMount{
		{Name: "main", HostPath: "/var/lib/ws/main", ContainerPath: "/workspace"},
		{Name: "beads", HostPath: "/var/lib/ws/beads", ContainerPath: "/workspace/.beads", ReadOnly: true},
	}

	contrib := NewWorkspaceMountsContributor(mounts)
	req := &MountRequest{ReadOnlyWorkspace: false}

	result, err := contrib.ContributeMounts(context.Background(), req)
	if err != nil {
		t.Fatalf("ContributeMounts() failed: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("got %d mounts, want 2", len(result))
	}

	// First mount: not read-only
	if result[0].HostPath != "/var/lib/ws/main" {
		t.Errorf("mount[0].HostPath = %q, want %q", result[0].HostPath, "/var/lib/ws/main")
	}
	if result[0].ContainerPath != "/workspace" {
		t.Errorf("mount[0].ContainerPath = %q, want %q", result[0].ContainerPath, "/workspace")
	}
	if result[0].ReadOnly {
		t.Error("mount[0] should not be read-only")
	}

	// Second mount: inherently read-only
	if !result[1].ReadOnly {
		t.Error("mount[1] should be read-only")
	}
}

func TestWorkspaceMountsContributor_ReadOnlyWorkspace(t *testing.T) {
	mounts := []ResolvedMount{
		{Name: "main", HostPath: "/var/lib/ws/main", ContainerPath: "/workspace"},
	}

	contrib := NewWorkspaceMountsContributor(mounts)
	req := &MountRequest{ReadOnlyWorkspace: true}

	result, err := contrib.ContributeMounts(context.Background(), req)
	if err != nil {
		t.Fatalf("ContributeMounts() failed: %v", err)
	}

	if !result[0].ReadOnly {
		t.Error("mount should be read-only when ReadOnlyWorkspace is true")
	}
}

func TestWorkspaceMountsContributor_Empty(t *testing.T) {
	contrib := NewWorkspaceMountsContributor(nil)
	result, err := contrib.ContributeMounts(context.Background(), &MountRequest{})
	if err != nil {
		t.Fatalf("ContributeMounts() failed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("got %d mounts, want 0", len(result))
	}
}

func TestWorkspaceMountContributor_BackwardCompat(t *testing.T) {
	// Ensure the legacy contributor still works
	contrib := NewWorkspaceMountContributor("/tmp/ws", "/workspace")
	result, err := contrib.ContributeMounts(context.Background(), &MountRequest{})
	if err != nil {
		t.Fatalf("ContributeMounts() failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d mounts, want 1", len(result))
	}
	if result[0].HostPath != "/tmp/ws" || result[0].ContainerPath != "/workspace" {
		t.Errorf("unexpected mount: %+v", result[0])
	}
}

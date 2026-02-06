package port

import (
	"testing"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
)

func TestAllocateSlot_Empty(t *testing.T) {
	slot, err := AllocateSlot(nil)
	if err != nil {
		t.Fatalf("AllocateSlot failed: %v", err)
	}

	if slot != 1 {
		t.Errorf("slot = %d, want 1", slot)
	}
}

func TestAllocateSlot_WithExisting(t *testing.T) {
	existing := []*config.SandboxMetadata{
		{NetworkSlot: 1},
		{NetworkSlot: 2},
	}

	slot, err := AllocateSlot(existing)
	if err != nil {
		t.Fatalf("AllocateSlot failed: %v", err)
	}

	if slot != 3 {
		t.Errorf("slot = %d, want 3", slot)
	}
}

func TestAllocateSlot_GapInSlots(t *testing.T) {
	// Slot 2 is free (gap)
	existing := []*config.SandboxMetadata{
		{NetworkSlot: 1},
		{NetworkSlot: 3},
	}

	slot, err := AllocateSlot(existing)
	if err != nil {
		t.Fatalf("AllocateSlot failed: %v", err)
	}

	if slot != 2 {
		t.Errorf("slot = %d, want 2 (first gap)", slot)
	}
}

func TestAllocateSlot_Exhausted(t *testing.T) {
	// Create 254 existing sandboxes (exhausting all network slots)
	existing := make([]*config.SandboxMetadata, 254)
	for i := 0; i < 254; i++ {
		existing[i] = &config.SandboxMetadata{
			NetworkSlot: i + 1,
		}
	}

	_, err := AllocateSlot(existing)
	if err == nil {
		t.Error("Expected error when network slots exhausted, got nil")
	}
}

func TestAllocateSlot_PreservesOrder(t *testing.T) {
	// Allocate multiple times and verify order
	var existing []*config.SandboxMetadata

	for i := 0; i < 5; i++ {
		slot, err := AllocateSlot(existing)
		if err != nil {
			t.Fatalf("AllocateSlot %d failed: %v", i, err)
		}

		expectedSlot := i + 1

		if slot != expectedSlot {
			t.Errorf("iteration %d: slot = %d, want %d", i, slot, expectedSlot)
		}

		existing = append(existing, &config.SandboxMetadata{
			NetworkSlot: slot,
		})
	}
}

func TestContainerIP(t *testing.T) {
	tests := []struct {
		slot int
		want string
	}{
		{1, "10.100.1.2"},
		{2, "10.100.2.2"},
		{100, "10.100.100.2"},
		{254, "10.100.254.2"},
	}

	for _, tt := range tests {
		got := ContainerIP(tt.slot)
		if got != tt.want {
			t.Errorf("ContainerIP(%d) = %q, want %q", tt.slot, got, tt.want)
		}
	}
}

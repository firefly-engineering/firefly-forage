package port

import (
	"testing"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
)

func TestAllocate_Empty(t *testing.T) {
	hostConfig := &config.HostConfig{
		PortRange: config.PortRange{
			From: 2200,
			To:   2299,
		},
	}

	port, slot, err := Allocate(hostConfig, nil)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}

	if port != 2200 {
		t.Errorf("port = %d, want 2200", port)
	}
	if slot != 1 {
		t.Errorf("slot = %d, want 1", slot)
	}
}

func TestAllocate_WithExisting(t *testing.T) {
	hostConfig := &config.HostConfig{
		PortRange: config.PortRange{
			From: 2200,
			To:   2299,
		},
	}

	existing := []*config.SandboxMetadata{
		{Port: 2200, NetworkSlot: 1},
		{Port: 2201, NetworkSlot: 2},
	}

	port, slot, err := Allocate(hostConfig, existing)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}

	if port != 2202 {
		t.Errorf("port = %d, want 2202", port)
	}
	if slot != 3 {
		t.Errorf("slot = %d, want 3", slot)
	}
}

func TestAllocate_GapInPorts(t *testing.T) {
	hostConfig := &config.HostConfig{
		PortRange: config.PortRange{
			From: 2200,
			To:   2299,
		},
	}

	// Port 2201 is free (gap)
	existing := []*config.SandboxMetadata{
		{Port: 2200, NetworkSlot: 1},
		{Port: 2202, NetworkSlot: 3},
	}

	port, slot, err := Allocate(hostConfig, existing)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}

	if port != 2201 {
		t.Errorf("port = %d, want 2201 (first gap)", port)
	}
	if slot != 2 {
		t.Errorf("slot = %d, want 2 (first gap)", slot)
	}
}

func TestAllocate_PortRangeExhausted(t *testing.T) {
	hostConfig := &config.HostConfig{
		PortRange: config.PortRange{
			From: 2200,
			To:   2202, // Only 3 ports available
		},
	}

	existing := []*config.SandboxMetadata{
		{Port: 2200, NetworkSlot: 1},
		{Port: 2201, NetworkSlot: 2},
		{Port: 2202, NetworkSlot: 3},
	}

	_, _, err := Allocate(hostConfig, existing)
	if err == nil {
		t.Error("Expected error when port range exhausted, got nil")
	}
}

func TestAllocate_SinglePort(t *testing.T) {
	hostConfig := &config.HostConfig{
		PortRange: config.PortRange{
			From: 2200,
			To:   2200, // Only 1 port available
		},
	}

	port, slot, err := Allocate(hostConfig, nil)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}

	if port != 2200 {
		t.Errorf("port = %d, want 2200", port)
	}
	if slot != 1 {
		t.Errorf("slot = %d, want 1", slot)
	}

	// Now it should be exhausted
	existing := []*config.SandboxMetadata{{Port: 2200, NetworkSlot: 1}}
	_, _, err = Allocate(hostConfig, existing)
	if err == nil {
		t.Error("Expected error when single port exhausted, got nil")
	}
}

func TestAllocate_NetworkSlotExhausted(t *testing.T) {
	hostConfig := &config.HostConfig{
		PortRange: config.PortRange{
			From: 2200,
			To:   2999, // Plenty of ports
		},
	}

	// Create 254 existing sandboxes (exhausting all network slots)
	existing := make([]*config.SandboxMetadata, 254)
	for i := 0; i < 254; i++ {
		existing[i] = &config.SandboxMetadata{
			Port:        2200 + i,
			NetworkSlot: i + 1,
		}
	}

	_, _, err := Allocate(hostConfig, existing)
	if err == nil {
		t.Error("Expected error when network slots exhausted, got nil")
	}
}

func TestAllocate_PreservesOrder(t *testing.T) {
	hostConfig := &config.HostConfig{
		PortRange: config.PortRange{
			From: 2200,
			To:   2299,
		},
	}

	// Allocate multiple times and verify order
	var existing []*config.SandboxMetadata

	for i := 0; i < 5; i++ {
		port, slot, err := Allocate(hostConfig, existing)
		if err != nil {
			t.Fatalf("Allocate %d failed: %v", i, err)
		}

		expectedPort := 2200 + i
		expectedSlot := i + 1

		if port != expectedPort {
			t.Errorf("iteration %d: port = %d, want %d", i, port, expectedPort)
		}
		if slot != expectedSlot {
			t.Errorf("iteration %d: slot = %d, want %d", i, slot, expectedSlot)
		}

		existing = append(existing, &config.SandboxMetadata{
			Port:        port,
			NetworkSlot: slot,
		})
	}
}

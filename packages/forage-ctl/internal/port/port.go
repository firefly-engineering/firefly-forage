package port

import (
	"fmt"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
)

// Network slot range for container IP allocation (10.100.X.0/24).
const (
	NetworkSlotMin = 1
	NetworkSlotMax = 254 // 255 is broadcast, 0 is network address
)

// AllocateSlot finds the next available network slot.
// Each sandbox gets a unique network slot for its private network (10.100.X.0/24).
func AllocateSlot(sandboxes []*config.SandboxMetadata) (slot int, err error) {
	usedSlots := make(map[int]bool)

	for _, sb := range sandboxes {
		usedSlots[sb.NetworkSlot] = true
	}

	// Find available network slot
	for s := NetworkSlotMin; s <= NetworkSlotMax; s++ {
		if !usedSlots[s] {
			return s, nil
		}
	}

	return 0, fmt.Errorf("no available network slots (max %d sandboxes)", NetworkSlotMax)
}

// ContainerIP returns the container IP address for a given network slot.
// Containers use the 10.100.X.0/24 network where X is the slot.
// The container gets .2 (host gets .1).
func ContainerIP(slot int) string {
	return fmt.Sprintf("10.100.%d.2", slot)
}

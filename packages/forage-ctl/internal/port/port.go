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

// Allocate finds the next available port and network slot
func Allocate(hostConfig *config.HostConfig, sandboxes []*config.SandboxMetadata) (port int, slot int, err error) {
	usedPorts := make(map[int]bool)
	usedSlots := make(map[int]bool)

	for _, sb := range sandboxes {
		usedPorts[sb.Port] = true
		usedSlots[sb.NetworkSlot] = true
	}

	// Find available port
	for p := hostConfig.PortRange.From; p <= hostConfig.PortRange.To; p++ {
		if !usedPorts[p] {
			port = p
			break
		}
	}

	if port == 0 {
		return 0, 0, fmt.Errorf("no available ports in range %d-%d",
			hostConfig.PortRange.From, hostConfig.PortRange.To)
	}

	// Find available network slot
	for s := NetworkSlotMin; s <= NetworkSlotMax; s++ {
		if !usedSlots[s] {
			slot = s
			break
		}
	}

	if slot == 0 {
		return 0, 0, fmt.Errorf("no available network slots")
	}

	return port, slot, nil
}

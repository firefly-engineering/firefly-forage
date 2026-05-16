// Package port provides port and network slot allocation for sandboxes.
//
// Each sandbox requires a unique SSH port and network slot. This package
// manages allocation by scanning existing sandbox metadata to find unused
// resources.
//
// # Port Allocation
//
// Ports are allocated from the range configured in HostConfig:
//
//	port, slot, err := port.Allocate(hostConfig, existingSandboxes)
//
// # Network Slots
//
// Network slots determine container IP addresses in the 10.100.X.0/24 range.
// Slot 1 gives 10.100.1.0/24, slot 2 gives 10.100.2.0/24, etc.
//
// Constants:
//
//	NetworkSlotMin = 1   // First usable slot
//	NetworkSlotMax = 254 // Last usable slot (255 is broadcast)
//
// # Allocation Strategy
//
// Both ports and slots are allocated using first-fit: the lowest available
// value is chosen. This maximizes the chance of finding available resources
// when sandboxes are created and destroyed over time.
package port

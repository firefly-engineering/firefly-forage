// Package network provides network isolation configuration for sandboxes.
//
// This package generates NixOS firewall rules for different network isolation
// modes, enabling fine-grained control over sandbox network access.
//
// # Network Modes
//
// Three modes are supported:
//
//   - ModeFull: Unrestricted outbound access (default)
//   - ModeRestricted: Only allowed hosts are accessible
//   - ModeNone: No external network access
//
// # Restricted Mode
//
// In restricted mode, the package:
//  1. Resolves allowed hostnames to IP addresses
//  2. Generates nftables rules permitting only those destinations
//  3. Blocks all other outbound traffic
//
// Usage:
//
//	cfg := &network.Config{
//	    Mode:         network.ModeRestricted,
//	    AllowedHosts: []string{"api.anthropic.com", "api.openai.com"},
//	    NetworkSlot:  1,
//	}
//	nixConfig := network.GenerateNixNetworkConfig(cfg)
//
// # Host Resolution
//
// ResolveHosts resolves hostnames to IP addresses for firewall rules:
//
//	resolved, err := network.ResolveHosts([]string{"api.anthropic.com"})
//	// Returns hostname with IPv4 and IPv6 addresses
package network

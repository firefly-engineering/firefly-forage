// Package health provides health check utilities for sandbox monitoring.
//
// Health checks verify that a sandbox is fully operational by checking
// container status, SSH connectivity, and multiplexer session availability.
//
// # Health Status
//
// Sandbox health is represented by Status:
//
//	StatusHealthy   - Container running, SSH reachable, mux active
//	StatusUnhealthy - Container running but SSH unreachable
//	StatusNoMux     - SSH reachable but multiplexer session not found
//	StatusStopped   - Container not running
//
// # Check Functions
//
// Individual checks:
//
//	health.CheckSSH(host)           // SSH connectivity
//	health.CheckMux(host, mux)      // multiplexer session exists
//	health.GetUptime(name, rt)      // Container uptime
//
// Combined checks:
//
//	result := health.Check(sandboxName, host, rt, mux)
//	// result.ContainerRunning, .SSHReachable, .MuxActive, .Uptime
//
//	status := health.GetSummary(sandboxName, host, rt, mux)
//	// Returns StatusHealthy, StatusUnhealthy, etc.
//
// # Constants
//
// SSHReadyTimeoutSeconds defines the default timeout when waiting for
// SSH to become available after container creation.
package health

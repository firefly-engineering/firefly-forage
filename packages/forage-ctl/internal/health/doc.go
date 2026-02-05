// Package health provides health check utilities for sandbox monitoring.
//
// Health checks verify that a sandbox is fully operational by checking
// container status, SSH connectivity, and tmux session availability.
//
// # Health Status
//
// Sandbox health is represented by Status:
//
//	StatusHealthy   - Container running, SSH reachable, tmux active
//	StatusUnhealthy - Container running but SSH unreachable
//	StatusNoTmux    - SSH reachable but tmux session not found
//	StatusStopped   - Container not running
//
// # Check Functions
//
// Individual checks:
//
//	health.CheckSSH(port)      // SSH connectivity
//	health.CheckTmux(port)     // tmux session exists
//	health.GetUptime(name, rt) // Container uptime
//
// Combined checks:
//
//	result := health.Check(sandboxName, port, rt)
//	// result.ContainerRunning, .SSHReachable, .TmuxActive, .Uptime
//
//	status := health.GetSummary(sandboxName, port, rt)
//	// Returns StatusHealthy, StatusUnhealthy, etc.
//
// # Constants
//
// SSHReadyTimeoutSeconds defines the default timeout when waiting for
// SSH to become available after container creation.
package health

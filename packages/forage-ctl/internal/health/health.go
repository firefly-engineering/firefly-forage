package health

import (
	"context"
	"fmt"
	"time"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/multiplexer"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/ssh"
)

// CheckOptions holds options for health checking.
type CheckOptions struct {
	Runtime runtime.Runtime
}

// Status represents the health status of a sandbox
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
	StatusNoMux     Status = "no-mux"
	StatusStopped   Status = "stopped"

	// SSHReadyTimeoutSeconds is the default timeout waiting for SSH to become ready.
	SSHReadyTimeoutSeconds = 30
)

// CheckResult contains the results of health checks
type CheckResult struct {
	ContainerRunning bool
	SSHReachable     bool
	MuxActive        bool
	Uptime           string
	MuxWindows       []string
}

// CheckSSH checks if SSH is reachable
func CheckSSH(host string) bool {
	return ssh.CheckConnection(host)
}

// CheckMux checks if the multiplexer session exists
func CheckMux(host string, mux multiplexer.Multiplexer) bool {
	args := mux.CheckSessionArgs()
	_, err := ssh.ExecWithOutput(host, args...)
	return err == nil
}

// GetMuxWindows returns the list of multiplexer windows
func GetMuxWindows(host string, mux multiplexer.Multiplexer) []string {
	args := mux.ListWindowsArgs()
	output, err := ssh.ExecWithOutput(host, args...)
	if err != nil {
		return nil
	}
	return mux.ParseWindowList(output)
}

// GetUptime returns the container uptime in human-readable format.
// Uses the runtime-agnostic Status method to get container start time.
func GetUptime(sandboxName string, rt runtime.Runtime) string {
	if rt == nil {
		return "unknown"
	}

	info, err := rt.Status(context.Background(), sandboxName)
	if err != nil || info == nil {
		return "unknown"
	}

	since := info.StartedAt
	if since == "" || since == "n/a" {
		return "unknown"
	}

	// Try common timestamp formats
	var t time.Time
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"Mon 2006-01-02 15:04:05 MST",
		"2006-01-02T15:04:05.000000000Z",
	}

	for _, format := range formats {
		if parsed, err := time.Parse(format, since); err == nil {
			t = parsed
			break
		}
	}

	if t.IsZero() {
		return since // Return raw value if can't parse
	}

	duration := time.Since(t)
	return formatDuration(duration)
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	} else if d < 24*time.Hour {
		hours := int(d.Hours())
		mins := int(d.Minutes()) % 60
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	return fmt.Sprintf("%dd %dh", days, hours)
}

// Check performs all health checks for a sandbox.
// The rt parameter is optional; if nil, container running check returns false.
func Check(sandboxName string, host string, rt runtime.Runtime, mux multiplexer.Multiplexer) *CheckResult {
	result := &CheckResult{}

	// Check container
	if rt != nil {
		result.ContainerRunning, _ = rt.IsRunning(context.Background(), sandboxName)
	}
	if !result.ContainerRunning {
		return result
	}

	// Check uptime
	result.Uptime = GetUptime(sandboxName, rt)

	// Check SSH
	result.SSHReachable = CheckSSH(host)
	if !result.SSHReachable {
		return result
	}

	// Check multiplexer
	result.MuxActive = CheckMux(host, mux)
	if result.MuxActive {
		result.MuxWindows = GetMuxWindows(host, mux)
	}

	return result
}

// GetSummary returns a summary health status.
// The rt parameter is optional; if nil, returns StatusStopped.
func GetSummary(sandboxName string, host string, rt runtime.Runtime, mux multiplexer.Multiplexer) Status {
	if rt == nil {
		return StatusStopped
	}
	running, _ := rt.IsRunning(context.Background(), sandboxName)
	if !running {
		return StatusStopped
	}
	if !CheckSSH(host) {
		return StatusUnhealthy
	}
	if !CheckMux(host, mux) {
		return StatusNoMux
	}
	return StatusHealthy
}

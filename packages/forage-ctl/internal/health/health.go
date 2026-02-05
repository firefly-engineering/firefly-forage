package health

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/ssh"
)

// Status represents the health status of a sandbox
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
	StatusNoTmux    Status = "no-tmux"
	StatusStopped   Status = "stopped"

	// SSHReadyTimeoutSeconds is the default timeout waiting for SSH to become ready.
	SSHReadyTimeoutSeconds = 30
)

// CheckResult contains the results of health checks
type CheckResult struct {
	ContainerRunning bool
	SSHReachable     bool
	TmuxActive       bool
	Uptime           string
	TmuxWindows      []string
}

// CheckSSH checks if SSH is reachable
func CheckSSH(port int) bool {
	return ssh.CheckConnection(port)
}

// CheckTmux checks if the tmux session exists
func CheckTmux(port int) bool {
	_, err := ssh.ExecWithOutput(port, "tmux", "has-session", "-t", config.TmuxSessionName)
	return err == nil
}

// GetTmuxWindows returns the list of tmux windows
func GetTmuxWindows(port int) []string {
	output, err := ssh.ExecWithOutput(port, "tmux", "list-windows", "-t", config.TmuxSessionName, "-F", "#{window_index}:#{window_name}")
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var windows []string
	for _, line := range lines {
		if line != "" {
			windows = append(windows, line)
		}
	}
	return windows
}

// GetUptime returns the container uptime in human-readable format.
// Uses the runtime-agnostic Status method to get container start time.
func GetUptime(sandboxName string) string {
	rt := runtime.Global()
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

// Check performs all health checks for a sandbox
func Check(sandboxName string, port int, sandboxesDir string) *CheckResult {
	result := &CheckResult{}

	// Check container
	result.ContainerRunning = runtime.IsRunning(sandboxName)
	if !result.ContainerRunning {
		return result
	}

	// Check uptime
	result.Uptime = GetUptime(sandboxName)

	// Check SSH
	result.SSHReachable = CheckSSH(port)
	if !result.SSHReachable {
		return result
	}

	// Check tmux
	result.TmuxActive = CheckTmux(port)
	if result.TmuxActive {
		result.TmuxWindows = GetTmuxWindows(port)
	}

	return result
}

// GetSummary returns a summary health status
func GetSummary(sandboxName string, port int, sandboxesDir string) Status {
	if !runtime.IsRunning(sandboxName) {
		return StatusStopped
	}
	if !CheckSSH(port) {
		return StatusUnhealthy
	}
	if !CheckTmux(port) {
		return StatusNoTmux
	}
	return StatusHealthy
}

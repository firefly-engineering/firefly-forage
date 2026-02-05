package health

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/container"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
)

// Status represents the health status of a sandbox
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
	StatusNoTmux    Status = "no-tmux"
	StatusStopped   Status = "stopped"
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
	args := []string{
		"-p", fmt.Sprintf("%d", port),
		"-o", "ConnectTimeout=2",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "BatchMode=yes",
		"agent@localhost", "true",
	}
	cmd := exec.Command("ssh", args...)
	return cmd.Run() == nil
}

// CheckTmux checks if the tmux session exists
func CheckTmux(port int) bool {
	output, err := container.ExecSSHWithOutput(port, "tmux", "has-session", "-t", "forage")
	_ = output
	return err == nil
}

// GetTmuxWindows returns the list of tmux windows
func GetTmuxWindows(port int) []string {
	output, err := container.ExecSSHWithOutput(port, "tmux", "list-windows", "-t", "forage", "-F", "#{window_index}:#{window_name}")
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

// GetUptime returns the container uptime in human-readable format
func GetUptime(sandboxName string) string {
	containerName := config.ContainerName(sandboxName)
	cmd := exec.Command("machinectl", "show", containerName, "-p", "Since", "--value")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}

	since := strings.TrimSpace(string(output))
	if since == "" || since == "n/a" {
		return "unknown"
	}

	// Try to parse the timestamp
	t, err := time.Parse("Mon 2006-01-02 15:04:05 MST", since)
	if err != nil {
		// Try alternate format
		t, err = time.Parse(time.RFC3339, since)
		if err != nil {
			return since // Return raw value if can't parse
		}
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

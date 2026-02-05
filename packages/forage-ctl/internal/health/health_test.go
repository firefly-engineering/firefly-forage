package health

import (
	"testing"
	"time"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
)

func TestStatusConstants(t *testing.T) {
	// Verify status constants are defined correctly
	tests := []struct {
		status Status
		want   string
	}{
		{StatusHealthy, "healthy"},
		{StatusUnhealthy, "unhealthy"},
		{StatusNoTmux, "no-tmux"},
		{StatusStopped, "stopped"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.want {
			t.Errorf("Status %v = %q, want %q", tt.status, tt.status, tt.want)
		}
	}
}

func TestConstants(t *testing.T) {
	// Verify important constants are set
	if config.TmuxSessionName == "" {
		t.Error("config.TmuxSessionName should not be empty")
	}
	if SSHReadyTimeoutSeconds <= 0 {
		t.Errorf("SSHReadyTimeoutSeconds = %d, should be positive", SSHReadyTimeoutSeconds)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{"seconds", 30 * time.Second, "30s"},
		{"one minute", 1 * time.Minute, "1m"},
		{"minutes", 45 * time.Minute, "45m"},
		{"one hour", 1 * time.Hour, "1h 0m"},
		{"hours and minutes", 2*time.Hour + 30*time.Minute, "2h 30m"},
		{"one day", 24 * time.Hour, "1d 0h"},
		{"days and hours", 3*24*time.Hour + 5*time.Hour, "3d 5h"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.duration)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, got, tt.want)
			}
		})
	}
}

func TestCheckResult(t *testing.T) {
	// Test that CheckResult struct has expected fields
	result := &CheckResult{
		ContainerRunning: true,
		SSHReachable:     true,
		TmuxActive:       true,
		Uptime:           "1h 30m",
		TmuxWindows:      []string{"0:bash", "1:nvim"},
	}

	if !result.ContainerRunning {
		t.Error("ContainerRunning should be true")
	}
	if !result.SSHReachable {
		t.Error("SSHReachable should be true")
	}
	if !result.TmuxActive {
		t.Error("TmuxActive should be true")
	}
	if result.Uptime != "1h 30m" {
		t.Errorf("Uptime = %q, want %q", result.Uptime, "1h 30m")
	}
	if len(result.TmuxWindows) != 2 {
		t.Errorf("TmuxWindows length = %d, want 2", len(result.TmuxWindows))
	}
}

func TestCheckSSH_NoConnection(t *testing.T) {
	// Test with a port that definitely won't have SSH running
	result := CheckSSH(65432)
	if result {
		t.Error("CheckSSH should return false for non-existent SSH server")
	}
}

func TestCheckTmux_NoConnection(t *testing.T) {
	// Test with a port that definitely won't have SSH running
	result := CheckTmux(65432)
	if result {
		t.Error("CheckTmux should return false for non-existent SSH server")
	}
}

func TestGetTmuxWindows_NoConnection(t *testing.T) {
	// Test with a port that definitely won't have SSH running
	windows := GetTmuxWindows(65432)
	if windows != nil {
		t.Error("GetTmuxWindows should return nil for non-existent SSH server")
	}
}

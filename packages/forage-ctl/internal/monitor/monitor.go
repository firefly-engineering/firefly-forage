// Package monitor provides background health monitoring for sandboxes.
package monitor

import (
	"context"
	"time"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/audit"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/health"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/multiplexer"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
)

// CheckResult holds the result of a single sandbox health check.
type CheckResult struct {
	Sandbox string
	Status  health.Status
	Health  *health.CheckResult
}

// Monitor periodically checks the health of all sandboxes.
type Monitor struct {
	interval    time.Duration
	rt          runtime.Runtime
	paths       *config.Paths
	autoRestart bool
	auditLog    *audit.Logger
}

// Option configures a Monitor.
type Option func(*Monitor)

// WithAutoRestart enables automatic restart of unhealthy sandboxes.
func WithAutoRestart(enabled bool) Option {
	return func(m *Monitor) {
		m.autoRestart = enabled
	}
}

// WithAuditLogger sets the audit logger for recording health events.
func WithAuditLogger(logger *audit.Logger) Option {
	return func(m *Monitor) {
		m.auditLog = logger
	}
}

// New creates a new Monitor.
func New(interval time.Duration, rt runtime.Runtime, paths *config.Paths, opts ...Option) *Monitor {
	m := &Monitor{
		interval: interval,
		rt:       rt,
		paths:    paths,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Run starts the monitoring loop. It blocks until the context is cancelled.
func (m *Monitor) Run(ctx context.Context) error {
	logging.Debug("starting health monitor", "interval", m.interval, "autoRestart", m.autoRestart)

	// Run an immediate check, then loop on interval.
	m.checkAll(ctx)

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logging.Debug("health monitor stopping")
			return ctx.Err()
		case <-ticker.C:
			m.checkAll(ctx)
		}
	}
}

// checkAll performs health checks on all known sandboxes.
func (m *Monitor) checkAll(ctx context.Context) []CheckResult {
	sandboxes, err := config.ListSandboxes(m.paths.SandboxesDir)
	if err != nil {
		logging.Warn("monitor failed to list sandboxes", "error", err)
		return nil
	}

	var results []CheckResult
	for _, sb := range sandboxes {
		if ctx.Err() != nil {
			break
		}

		mux := multiplexer.New(multiplexer.Type(sb.Multiplexer))
		status := health.GetSummary(sb.Name, sb.ContainerIP(), m.rt, mux)
		result := CheckResult{
			Sandbox: sb.Name,
			Status:  status,
		}
		results = append(results, result)

		// Log health events
		if m.auditLog != nil {
			var details string
			switch status {
			case health.StatusHealthy:
				details = "healthy"
			case health.StatusUnhealthy:
				details = "unhealthy"
			case health.StatusNoMux:
				details = "no-mux"
			case health.StatusStopped:
				details = "stopped"
			}
			_ = m.auditLog.LogEvent(audit.EventHealth, sb.Name, details)
		}

		// Auto-restart unhealthy or stopped containers
		if m.autoRestart && (status == health.StatusStopped || status == health.StatusUnhealthy) {
			logging.UserInfo("Auto-restarting sandbox %s (status: %s)", sb.Name, status)
			if err := m.rt.Start(ctx, sb.Name); err != nil {
				logging.Warn("auto-restart failed", "sandbox", sb.Name, "error", err)
				if m.auditLog != nil {
					_ = m.auditLog.LogEvent(audit.EventError, sb.Name, "auto-restart failed: "+err.Error())
				}
			} else {
				if m.auditLog != nil {
					_ = m.auditLog.LogEvent(audit.EventStart, sb.Name, "auto-restart")
				}
			}
		}
	}

	return results
}

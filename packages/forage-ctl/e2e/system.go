//go:build e2e

// Package e2e provides end-to-end testing infrastructure for Firefly Forage.
//
// Tests boot a QEMU VM with the forage NixOS module configured, run forage-ctl
// commands via SSH, and verify sandbox lifecycle behavior. The System interface
// abstracts command execution so the same test scenarios work against a VM or
// the local machine.
package e2e

import (
	"context"
	"fmt"

	"golang.org/x/crypto/ssh"

	"go.opentelemetry.io/otel/attribute"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/telemetry"
)

// System represents a host where forage-ctl and sandbox containers run.
// Tests use this interface exclusively and never reference VM/SSH details.
type System interface {
	// Run executes a shell command and returns combined stdout+stderr.
	Run(ctx context.Context, cmd string) (string, error)

	// ForageCtl runs forage-ctl with the given arguments.
	ForageCtl(ctx context.Context, args ...string) (string, error)

	// DialSandbox opens an SSH connection to a sandbox container.
	DialSandbox(ctx context.Context, ip string) (*SandboxConn, error)

	// Close shuts down the system and releases resources.
	Close() error
}

// SandboxConn wraps an SSH connection to a sandbox container.
type SandboxConn struct {
	client *ssh.Client
	ip     string
}

// Run executes a command inside the sandbox container.
func (s *SandboxConn) Run(ctx context.Context, cmd string) (string, error) {
	ctx, span := telemetry.Start(ctx, "sandbox.exec",
		telemetry.WithAttr(attribute.String("cmd", cmd)))
	defer span.End()

	session, err := s.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	done := make(chan struct{})
	var output []byte
	var runErr error

	go func() {
		output, runErr = session.CombinedOutput(cmd)
		close(done)
	}()

	select {
	case <-ctx.Done():
		session.Signal(ssh.SIGKILL)
		return "", ctx.Err()
	case <-done:
		if runErr != nil {
			return string(output), fmt.Errorf("sandbox exec %q: %w\noutput: %s", cmd, runErr, output)
		}
		return string(output), nil
	}
}

// Close terminates the SSH connection.
func (s *SandboxConn) Close() error {
	return s.client.Close()
}

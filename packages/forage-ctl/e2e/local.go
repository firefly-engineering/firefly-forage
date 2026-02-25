//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// LocalSystem runs commands on the local machine via os/exec.
// Useful for testing against a real NixOS host without the VM layer.
type LocalSystem struct {
	sshKeyPath string
}

// NewLocalSystem creates a LocalSystem.
func NewLocalSystem(sshKeyPath string) *LocalSystem {
	return &LocalSystem{sshKeyPath: sshKeyPath}
}

// Run executes a shell command locally.
func (l *LocalSystem) Run(ctx context.Context, cmd string) (string, error) {
	c := exec.CommandContext(ctx, "bash", "-c", cmd)
	output, err := c.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("local exec %q: %w\noutput: %s", cmd, err, output)
	}
	return string(output), nil
}

// ForageCtl runs forage-ctl with the given arguments locally.
func (l *LocalSystem) ForageCtl(ctx context.Context, args ...string) (string, error) {
	c := exec.CommandContext(ctx, "forage-ctl", args...)
	output, err := c.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("forage-ctl %s: %w\noutput: %s", strings.Join(args, " "), err, output)
	}
	return string(output), nil
}

// DialSandbox opens a direct SSH connection to a sandbox container.
func (l *LocalSystem) DialSandbox(ctx context.Context, ip string) (*SandboxConn, error) {
	keyData, err := os.ReadFile(l.sshKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read ssh key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		return nil, fmt.Errorf("parse ssh key: %w", err)
	}

	config := &ssh.ClientConfig{
		User: "agent",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := ip + ":22"
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}

	return &SandboxConn{client: client, ip: ip}, nil
}

// Close is a no-op for LocalSystem.
func (l *LocalSystem) Close() error {
	return nil
}

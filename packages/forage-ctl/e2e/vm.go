//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"

	"go.opentelemetry.io/otel/attribute"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/telemetry"
)

// VMConfig configures the QEMU VM for testing.
type VMConfig struct {
	// VMScript is the path to the QEMU VM run script (from nix build).
	VMScript string
	// SSHKeyPath is the path to the SSH private key for connecting to the VM.
	SSHKeyPath string
	// BootTimeout is how long to wait for the VM to boot and SSH to become ready.
	BootTimeout time.Duration
}

// VMSystem boots a QEMU VM and connects via SSH.
type VMSystem struct {
	cfg        VMConfig
	sshClient  *ssh.Client
	sshConfig  *ssh.ClientConfig
	sshPort    int
	qemuPID    int
	tmpDir     string
	consoleLog string
}

// NewVMSystem boots a QEMU VM and returns a System interface.
func NewVMSystem(ctx context.Context, cfg VMConfig) (*VMSystem, error) {
	if cfg.BootTimeout == 0 {
		cfg.BootTimeout = 5 * time.Minute
	}

	vm := &VMSystem{cfg: cfg}

	// Read SSH private key
	keyData, err := os.ReadFile(cfg.SSHKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read ssh key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		return nil, fmt.Errorf("parse ssh key: %w", err)
	}

	vm.sshConfig = &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	// Allocate a random free port
	port, err := allocatePort()
	if err != nil {
		return nil, fmt.Errorf("allocate port: %w", err)
	}
	vm.sshPort = port

	// Create a temp directory for this VM instance
	vm.tmpDir, err = os.MkdirTemp("", "forage-e2e-*")
	if err != nil {
		return nil, fmt.Errorf("create tmpdir: %w", err)
	}

	// Patch the VM run script to use our allocated port
	if err := vm.patchRunScript(); err != nil {
		os.RemoveAll(vm.tmpDir)
		return nil, fmt.Errorf("patch run script: %w", err)
	}

	// Boot the VM
	if err := vm.boot(ctx); err != nil {
		os.RemoveAll(vm.tmpDir)
		return nil, fmt.Errorf("boot vm: %w", err)
	}

	// Register in the VM registry
	if err := Register(VMInfo{
		ID:        filepath.Base(vm.tmpDir),
		PID:       vm.qemuPID,
		SSHPort:   vm.sshPort,
		StartTime: time.Now(),
		TmpDir:    vm.tmpDir,
	}); err != nil {
		log.Printf("warning: failed to register VM: %v", err)
	}

	// Wait for SSH
	if err := vm.waitSSH(ctx); err != nil {
		vm.Close()
		return nil, fmt.Errorf("wait ssh: %w", err)
	}

	// Wait for system to be fully ready
	log.Printf("waiting for multi-user.target...")
	vm.Run(ctx, "systemctl is-system-running --wait")

	return vm, nil
}

// Run executes a shell command in the VM via SSH.
func (vm *VMSystem) Run(ctx context.Context, cmd string) (string, error) {
	ctx, span := telemetry.Start(ctx, "vm.exec",
		telemetry.WithAttr(attribute.String("cmd", cmd)))
	defer span.End()

	session, err := vm.sshClient.NewSession()
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
			return string(output), fmt.Errorf("vm exec %q: %w\noutput: %s", cmd, runErr, output)
		}
		return string(output), nil
	}
}

// ForageCtl runs forage-ctl with the given arguments in the VM.
func (vm *VMSystem) ForageCtl(ctx context.Context, args ...string) (string, error) {
	ctx, span := telemetry.Start(ctx, "vm.forage-ctl",
		telemetry.WithAttr(attribute.String("args", strings.Join(args, " "))))
	defer span.End()

	cmd := "forage-ctl " + strings.Join(args, " ")
	return vm.Run(ctx, cmd)
}

// DialSandbox opens an SSH connection to a sandbox container via SSH tunneling.
// The connection is tunneled through the VM: Host -> VM -> Container.
func (vm *VMSystem) DialSandbox(ctx context.Context, ip string) (*SandboxConn, error) {
	// Read the SSH key for container auth (same key, "agent" user)
	keyData, err := os.ReadFile(vm.cfg.SSHKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read ssh key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		return nil, fmt.Errorf("parse ssh key: %w", err)
	}

	containerConfig := &ssh.ClientConfig{
		User: "agent",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	// Tunnel through the VM to the container
	addr := ip + ":22"
	conn, err := vm.sshClient.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("tunnel to %s: %w", addr, err)
	}

	nconn, chans, reqs, err := ssh.NewClientConn(conn, addr, containerConfig)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("ssh handshake with container %s: %w", addr, err)
	}

	client := ssh.NewClient(nconn, chans, reqs)
	return &SandboxConn{client: client, ip: ip}, nil
}

// Close shuts down the VM and cleans up resources.
func (vm *VMSystem) Close() error {
	// Close SSH connection
	if vm.sshClient != nil {
		vm.sshClient.Close()
	}

	// Try graceful shutdown
	if vm.qemuPID > 0 {
		log.Printf("shutting down VM (PID %d)...", vm.qemuPID)

		// Send SIGTERM
		proc, err := os.FindProcess(vm.qemuPID)
		if err == nil {
			proc.Signal(syscall.SIGTERM)

			// Wait up to 30s for graceful shutdown
			deadline := time.Now().Add(30 * time.Second)
			for time.Now().Before(deadline) {
				if err := proc.Signal(syscall.Signal(0)); err != nil {
					break // Process exited
				}
				time.Sleep(500 * time.Millisecond)
			}

			// Force kill if still alive
			if err := proc.Signal(syscall.Signal(0)); err == nil {
				log.Printf("VM did not exit gracefully, sending SIGKILL")
				proc.Signal(syscall.SIGKILL)
				time.Sleep(time.Second)
			}
		}
	}

	// Deregister from registry
	Deregister(filepath.Base(vm.tmpDir))

	// Clean up temp directory
	if vm.tmpDir != "" {
		os.RemoveAll(vm.tmpDir)
	}

	return nil
}

// SSHPort returns the SSH port allocated for this VM.
func (vm *VMSystem) SSHPort() int {
	return vm.sshPort
}

// allocatePort finds a random free TCP port.
func allocatePort() (int, error) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port, nil
}

// patchRunScript copies the VM run script and replaces the hardcoded SSH port.
func (vm *VMSystem) patchRunScript() error {
	data, err := os.ReadFile(vm.cfg.VMScript)
	if err != nil {
		return fmt.Errorf("read vm script: %w", err)
	}

	// Replace the hardcoded port forwarding
	patched := strings.ReplaceAll(
		string(data),
		"hostfwd=tcp::2222-:22",
		fmt.Sprintf("hostfwd=tcp::%d-:22", vm.sshPort),
	)

	patchedScript := filepath.Join(vm.tmpDir, "run-vm")
	if err := os.WriteFile(patchedScript, []byte(patched), 0755); err != nil {
		return fmt.Errorf("write patched script: %w", err)
	}

	return nil
}

// boot starts the QEMU VM process.
func (vm *VMSystem) boot(ctx context.Context) error {
	ctx, span := telemetry.Start(ctx, "vm.boot",
		telemetry.WithAttr(attribute.Int("ssh.port", vm.sshPort)))
	defer span.End()

	log.Printf("booting VM on SSH port %d...", vm.sshPort)

	// Clean up any stale disk images in our tmpdir
	matches, _ := filepath.Glob(filepath.Join(vm.tmpDir, "*.qcow2"))
	for _, m := range matches {
		os.Remove(m)
	}

	vm.consoleLog = filepath.Join(vm.tmpDir, "console.log")
	pidFile := filepath.Join(vm.tmpDir, "vm.pid")

	script := filepath.Join(vm.tmpDir, "run-vm")
	cmd := exec.CommandContext(ctx, script,
		"-daemonize",
		"-pidfile", pidFile,
		"-display", "none",
		"-serial", "file:"+vm.consoleLog,
	)
	// Run QEMU from the tmpdir so qcow2 files land there
	cmd.Dir = vm.tmpDir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("start qemu: %w", err)
	}

	// Wait for PID file (QEMU creates nix store image first, can take a while)
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(pidFile)
		if err == nil && len(strings.TrimSpace(string(data))) > 0 {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				return fmt.Errorf("parse pid: %w", err)
			}
			vm.qemuPID = pid
			log.Printf("VM started with PID %d", pid)
			return nil
		}
		time.Sleep(time.Second)
	}

	return fmt.Errorf("VM failed to start (no PID file after timeout)")
}

// waitSSH waits for the VM to become reachable via SSH.
func (vm *VMSystem) waitSSH(ctx context.Context) error {
	deadline := time.Now().Add(vm.cfg.BootTimeout)
	addr := fmt.Sprintf("localhost:%d", vm.sshPort)

	log.Printf("waiting for SSH on %s (timeout: %v)...", addr, vm.cfg.BootTimeout)

	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		client, err := ssh.Dial("tcp", addr, vm.sshConfig)
		if err == nil {
			vm.sshClient = client
			log.Printf("SSH ready")
			return nil
		}

		time.Sleep(2 * time.Second)
	}

	// Dump console log on timeout
	if data, err := os.ReadFile(vm.consoleLog); err == nil {
		lines := strings.Split(string(data), "\n")
		start := 0
		if len(lines) > 50 {
			start = len(lines) - 50
		}
		log.Printf("VM console log (last 50 lines):\n%s", strings.Join(lines[start:], "\n"))
	}

	return fmt.Errorf("SSH timeout after %v", vm.cfg.BootTimeout)
}

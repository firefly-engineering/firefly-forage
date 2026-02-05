// Package gateway provides the gateway service for single-port sandbox access
package gateway

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/tui"
)

// Server represents the gateway server
type Server struct {
	Paths *config.Paths
}

// NewServer creates a new gateway server
func NewServer(paths *config.Paths) *Server {
	return &Server{Paths: paths}
}

// HandleConnection handles an incoming connection
// This is designed to be called from SSH ForceCommand
func (s *Server) HandleConnection(args []string) error {
	logging.Debug("gateway connection", "args", args)

	// If a sandbox name is provided as argument, connect directly
	if len(args) > 0 && args[0] != "" {
		sandboxName := args[0]
		return s.ConnectToSandbox(sandboxName)
	}

	// Otherwise, show the interactive picker
	return s.ShowPicker()
}

// HandleSSHOriginalCommand handles SSH_ORIGINAL_COMMAND environment variable
// This is used when the gateway is set up as SSH ForceCommand
func (s *Server) HandleSSHOriginalCommand() error {
	originalCmd := os.Getenv("SSH_ORIGINAL_COMMAND")
	logging.Debug("SSH_ORIGINAL_COMMAND", "value", originalCmd)

	if originalCmd != "" {
		// Parse the command - first word is sandbox name
		parts := strings.Fields(originalCmd)
		if len(parts) > 0 {
			return s.ConnectToSandbox(parts[0])
		}
	}

	// No command specified, show picker
	return s.ShowPicker()
}

// ShowPicker displays the interactive sandbox picker
func (s *Server) ShowPicker() error {
	sandboxes, err := config.ListSandboxes(s.Paths.SandboxesDir)
	if err != nil {
		return fmt.Errorf("failed to list sandboxes: %w", err)
	}

	if len(sandboxes) == 0 {
		fmt.Println("No sandboxes available.")
		fmt.Println("\nCreate a sandbox on the host with:")
		fmt.Println("  forage-ctl up <name> -t <template> -w <workspace>")
		return nil
	}

	result, err := tui.RunPicker(sandboxes, s.Paths)
	if err != nil {
		return fmt.Errorf("picker error: %w", err)
	}

	switch result.Action {
	case tui.ActionAttach:
		if result.Sandbox != nil {
			return s.ConnectToSandbox(result.Sandbox.Name)
		}

	case tui.ActionNew:
		fmt.Println("\nCreate a sandbox on the host with:")
		fmt.Println("  forage-ctl up <name> -t <template> -w <workspace>")

	case tui.ActionDown:
		if result.Sandbox != nil {
			fmt.Printf("\nRemove sandbox on the host with:\n")
			fmt.Printf("  forage-ctl down %s\n", result.Sandbox.Name)
		}
	}

	return nil
}

// ConnectToSandbox connects to a specific sandbox
func (s *Server) ConnectToSandbox(name string) error {
	metadata, err := config.LoadSandboxMetadata(s.Paths.SandboxesDir, name)
	if err != nil {
		return fmt.Errorf("sandbox not found: %s", name)
	}

	if !runtime.IsRunning(name) {
		return fmt.Errorf("sandbox %s is not running", name)
	}

	logging.Debug("connecting to sandbox", "name", name, "port", metadata.Port)

	// Find ssh binary
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("ssh not found: %w", err)
	}

	// Build SSH command to connect to sandbox
	sshArgs := []string{
		"ssh",
		"-p", fmt.Sprintf("%d", metadata.Port),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-t", "agent@localhost",
		"tmux attach-session -t forage || tmux new-session -s forage",
	}

	// Replace current process with SSH
	return syscall.Exec(sshPath, sshArgs, os.Environ())
}

// ListSandboxes returns a formatted list of sandboxes
func (s *Server) ListSandboxes() (string, error) {
	sandboxes, err := config.ListSandboxes(s.Paths.SandboxesDir)
	if err != nil {
		return "", err
	}

	return tui.SimplePicker(sandboxes, s.Paths), nil
}

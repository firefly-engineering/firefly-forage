// Package gateway provides the gateway service for single-port sandbox access
package gateway

import (
	"fmt"
	"os"
	"strings"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/tui"
)

// Server represents the gateway server
type Server struct {
	Paths   *config.Paths
	Runtime runtime.Runtime
}

// NewServer creates a new gateway server
func NewServer(paths *config.Paths, rt runtime.Runtime) *Server {
	return &Server{Paths: paths, Runtime: rt}
}

// HandleConnection handles an incoming connection
// This is designed to be called from SSH ForceCommand
func (s *Server) HandleConnection(args []string) error {
	logging.Debug("gateway connection", "args", args)

	// If a sandbox name is provided as argument, connect directly
	if len(args) > 0 && args[0] != "" {
		sandboxName := args[0]
		// Validate before attempting connection (defense in depth - ConnectToSandbox also validates)
		if err := config.ValidateSandboxName(sandboxName); err != nil {
			return fmt.Errorf("invalid sandbox name: %w", err)
		}
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
			sandboxName := parts[0]
			// Validate before attempting connection (defense in depth)
			if err := config.ValidateSandboxName(sandboxName); err != nil {
				return fmt.Errorf("invalid sandbox name in SSH command: %w", err)
			}
			return s.ConnectToSandbox(sandboxName)
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

	result, err := tui.RunPicker(sandboxes, s.Paths, s.Runtime, tui.PickerOptions{})
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
	// Validate name to prevent path traversal or injection
	if err := config.ValidateSandboxName(name); err != nil {
		return fmt.Errorf("invalid sandbox name: %w", err)
	}

	return Connect(name, s.Paths.SandboxesDir, s.Runtime)
}

// ListSandboxes returns a formatted list of sandboxes
func (s *Server) ListSandboxes() (string, error) {
	sandboxes, err := config.ListSandboxes(s.Paths.SandboxesDir)
	if err != nil {
		return "", err
	}

	return tui.SimplePicker(sandboxes, s.Paths, s.Runtime), nil
}

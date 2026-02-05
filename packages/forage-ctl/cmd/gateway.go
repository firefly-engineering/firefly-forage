package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/tui"
	"github.com/spf13/cobra"
)

var gatewayCmd = &cobra.Command{
	Use:   "gateway [sandbox-name]",
	Short: "Interactive sandbox selector (gateway mode)",
	Long: `Opens an interactive TUI for selecting and connecting to sandboxes.

If a sandbox name is provided, connects directly to that sandbox.
Otherwise, presents an interactive picker to choose from available sandboxes.

This command is designed to be used as a login shell for SSH access,
providing a single entry point to all sandboxes.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runGateway,
}

func init() {
	rootCmd.AddCommand(gatewayCmd)
}

func runGateway(cmd *cobra.Command, args []string) error {
	paths := config.DefaultPaths()

	logging.Debug("gateway mode started")

	// If sandbox name provided, connect directly
	if len(args) == 1 {
		return connectToSandbox(args[0], paths)
	}

	// List sandboxes
	sandboxes, err := config.ListSandboxes(paths.SandboxesDir)
	if err != nil {
		return fmt.Errorf("failed to list sandboxes: %w", err)
	}

	// Run interactive picker
	result, err := tui.RunPicker(sandboxes, paths)
	if err != nil {
		return fmt.Errorf("picker error: %w", err)
	}

	logging.Debug("picker result", "action", result.Action)

	switch result.Action {
	case tui.ActionAttach:
		if result.Sandbox != nil {
			return connectToSandbox(result.Sandbox.Name, paths)
		}

	case tui.ActionNew:
		fmt.Println("\nTo create a new sandbox, run:")
		fmt.Println("  forage-ctl up <name> -t <template> -w <workspace>")
		fmt.Println("\nAvailable templates:")
		templates, _ := config.ListTemplates(paths.TemplatesDir)
		for _, t := range templates {
			fmt.Printf("  - %s: %s\n", t.Name, t.Description)
		}

	case tui.ActionDown:
		if result.Sandbox != nil {
			fmt.Printf("\nTo remove sandbox '%s', run:\n", result.Sandbox.Name)
			fmt.Printf("  forage-ctl down %s\n", result.Sandbox.Name)
		}

	case tui.ActionQuit:
		// Just exit cleanly
	}

	return nil
}

func connectToSandbox(name string, paths *config.Paths) error {
	metadata, err := config.LoadSandboxMetadata(paths.SandboxesDir, name)
	if err != nil {
		return fmt.Errorf("sandbox not found: %s", name)
	}

	if !runtime.IsRunning(name) {
		return fmt.Errorf("sandbox %s is not running", name)
	}

	logging.Debug("connecting to sandbox", "name", name, "port", metadata.Port)

	// Use exec to replace the current process with ssh
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("ssh not found: %w", err)
	}

	sshArgs := []string{
		"ssh",
		"-p", fmt.Sprintf("%d", metadata.Port),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-t", "agent@localhost",
		"tmux attach-session -t forage || tmux new-session -s forage",
	}

	return syscall.Exec(sshPath, sshArgs, os.Environ())
}

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/gateway"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/tui"
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
	p := paths()
	rt := getRuntime()

	logging.Debug("gateway mode started")

	// If sandbox name provided, connect directly
	if len(args) == 1 {
		return connectToSandbox(args[0], p)
	}

	// List sandboxes
	sandboxes, err := listSandboxes()
	if err != nil {
		return fmt.Errorf("failed to list sandboxes: %w", err)
	}

	// Run interactive picker (no creation wizard in gateway mode)
	result, err := tui.RunPicker(sandboxes, p, rt, tui.PickerOptions{})
	if err != nil {
		return fmt.Errorf("picker error: %w", err)
	}

	logging.Debug("picker result", "action", result.Action)

	switch result.Action {
	case tui.ActionAttach:
		if result.Sandbox != nil {
			return connectToSandbox(result.Sandbox.Name, p)
		}

	case tui.ActionNew:
		// TODO: implement creation wizard for gateway mode
		fmt.Println("\nTo create a new sandbox, run:")
		fmt.Println("  forage-ctl up <name> -t <template> -w <workspace>")
		fmt.Println("\nAvailable templates:")
		templates, _ := config.ListTemplates(p.TemplatesDir)
		for _, t := range templates {
			fmt.Printf("  - %s: %s\n", t.Name, t.Description)
		}

	case tui.ActionDown:
		// TODO: implement sandbox teardown in gateway mode
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
	return gateway.Connect(name, paths.SandboxesDir, getRuntime())
}

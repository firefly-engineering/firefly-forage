package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/sandbox"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/tui"
)

var pickCmd = &cobra.Command{
	Use:   "pick",
	Short: "Interactive sandbox picker",
	Long: `Opens an interactive TUI for selecting and connecting to sandboxes.

Use arrow keys or j/k to navigate, / to filter, Enter to connect.

Actions:
  Enter  - Attach to selected sandbox
  n      - Create new sandbox (opens wizard)
  d      - Show instructions for removing selected sandbox
  q/Esc  - Quit`,
	RunE: runPick,
}

func init() {
	rootCmd.AddCommand(pickCmd)
}

func runPick(cmd *cobra.Command, args []string) error {
	p := paths()
	rt := getRuntime()

	logging.Debug("picker mode started")

	// List sandboxes (nil is fine -- RunPicker handles empty list)
	sandboxes, err := listSandboxes()
	if err != nil {
		return fmt.Errorf("failed to list sandboxes: %w", err)
	}

	opts := tui.PickerOptions{
		AllowCreate:  true,
		TemplatesDir: p.TemplatesDir,
	}

	// Run interactive picker
	result, err := tui.RunPicker(sandboxes, p, rt, opts)
	if err != nil {
		return fmt.Errorf("picker error: %w", err)
	}

	logging.Debug("picker result", "action", result.Action)

	switch result.Action {
	case tui.ActionAttach:
		if result.Sandbox != nil {
			return attachToSandbox(result.Sandbox, p)
		}

	case tui.ActionNew:
		if result.CreateOptions != nil {
			return createSandboxFromWizard(result.CreateOptions)
		}
		fmt.Println("\nTo create a new sandbox, run:")
		fmt.Println("  forage-ctl up <name> -t <template> -w <workspace>")
		fmt.Println("\nAvailable templates:")
		templates, _ := config.ListTemplates(p.TemplatesDir)
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

func createSandboxFromWizard(opts *tui.CreateOptions) error {
	ctx := context.Background()

	creator, err := sandbox.NewCreator()
	if err != nil {
		return fmt.Errorf("failed to initialize: %w", err)
	}

	logInfo("Creating sandbox %s...", opts.Name)

	result, err := creator.Create(ctx, sandbox.CreateOptions{
		Name:         opts.Name,
		Template:     opts.Template,
		RepoPath:     opts.RepoPath,
		Direct:       opts.Direct,
		NoTmuxConfig: opts.NoTmuxConfig,
		GitUser:      opts.GitUser,
		GitEmail:     opts.GitEmail,
		SSHKeyPath:   opts.SSHKeyPath,
	})
	if err != nil {
		return fmt.Errorf("sandbox creation failed: %w", err)
	}

	logSuccess("Sandbox %s created", opts.Name)
	fmt.Printf("  IP: %s\n", result.ContainerIP)
	fmt.Printf("  Workspace: %s\n", result.Workspace)
	fmt.Printf("  Connect: forage-ctl ssh %s\n", opts.Name)

	return nil
}

func attachToSandbox(metadata *config.SandboxMetadata, paths *config.Paths) error {
	if !isRunning(metadata.Name) {
		return fmt.Errorf("sandbox %s is not running. Start it with: forage-ctl start %s",
			metadata.Name, metadata.Name)
	}

	logging.Debug("attaching to sandbox", "name", metadata.Name, "ip", metadata.ContainerIP())

	// Use the ssh command logic
	return connectToSandbox(metadata.Name, paths)
}

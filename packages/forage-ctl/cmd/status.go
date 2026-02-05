package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/health"
)

var statusCmd = &cobra.Command{
	Use:   "status <name>",
	Short: "Show detailed status of a sandbox",
	Args:  cobra.ExactArgs(1),
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	name := args[0]

	metadata, err := loadSandbox(name)
	if err != nil {
		return err
	}

	result := health.Check(name, metadata.Port, getRuntime())

	fmt.Printf("Sandbox: %s\n", metadata.Name)
	fmt.Printf("Template: %s\n", metadata.Template)
	fmt.Printf("Port: %d\n", metadata.Port)
	fmt.Printf("Workspace: %s\n", metadata.Workspace)

	mode := metadata.WorkspaceMode
	if mode == "" {
		mode = "direct"
	}
	fmt.Printf("Mode: %s\n", mode)

	if metadata.SourceRepo != "" {
		fmt.Printf("Source Repo: %s\n", metadata.SourceRepo)
	}
	if metadata.JJWorkspaceName != "" {
		fmt.Printf("JJ Workspace: %s\n", metadata.JJWorkspaceName)
	}

	fmt.Printf("Created: %s\n", metadata.CreatedAt)
	fmt.Println()

	// Health status
	fmt.Println("Health Checks:")
	fmt.Printf("  Container: %s\n", boolStatus(result.ContainerRunning))
	if result.ContainerRunning {
		fmt.Printf("  Uptime: %s\n", result.Uptime)
		fmt.Printf("  SSH: %s\n", boolStatus(result.SSHReachable))
		fmt.Printf("  Tmux: %s\n", boolStatus(result.TmuxActive))
		if len(result.TmuxWindows) > 0 {
			fmt.Printf("  Windows: %s\n", strings.Join(result.TmuxWindows, ", "))
		}
	}

	return nil
}

func boolStatus(b bool) string {
	if b {
		return "✓"
	}
	return "✗"
}

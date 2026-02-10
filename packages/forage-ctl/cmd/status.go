package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/health"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/multiplexer"
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

	mux := multiplexer.New(multiplexer.Type(metadata.Multiplexer))
	result := health.Check(name, metadata.ContainerIP(), getRuntime(), mux)

	fmt.Printf("Sandbox: %s\n", metadata.Name)
	fmt.Printf("Template: %s\n", metadata.Template)
	fmt.Printf("IP: %s\n", metadata.ContainerIP())
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

	if metadata.AgentIdentity != nil {
		id := metadata.AgentIdentity
		if id.GitUser != "" {
			fmt.Printf("Git User: %s\n", id.GitUser)
		}
		if id.GitEmail != "" {
			fmt.Printf("Git Email: %s\n", id.GitEmail)
		}
		if id.SSHKeyPath != "" {
			fmt.Printf("SSH Key: %s\n", id.SSHKeyPath)
		}
	}

	fmt.Printf("Created: %s\n", metadata.CreatedAt)
	fmt.Println()

	// Health status
	fmt.Println("Health Checks:")
	fmt.Printf("  Container: %s\n", boolStatus(result.ContainerRunning))
	if result.ContainerRunning {
		fmt.Printf("  Uptime: %s\n", result.Uptime)
		fmt.Printf("  SSH: %s\n", boolStatus(result.SSHReachable))
		fmt.Printf("  Mux: %s\n", boolStatus(result.MuxActive))
		if len(result.MuxWindows) > 0 {
			fmt.Printf("  Windows: %s\n", strings.Join(result.MuxWindows, ", "))
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

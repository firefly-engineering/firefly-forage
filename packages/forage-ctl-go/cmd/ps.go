package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/health"
	"github.com/spf13/cobra"
)

var psCmd = &cobra.Command{
	Use:   "ps",
	Short: "List all sandboxes",
	RunE:  runPs,
}

func init() {
	rootCmd.AddCommand(psCmd)
}

func runPs(cmd *cobra.Command, args []string) error {
	paths := config.DefaultPaths()

	sandboxes, err := config.ListSandboxes(paths.SandboxesDir)
	if err != nil {
		return fmt.Errorf("failed to list sandboxes: %w", err)
	}

	if len(sandboxes) == 0 {
		logInfo("No sandboxes found. Create one with: forage-ctl up <name> -t <template>")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTEMPLATE\tPORT\tMODE\tWORKSPACE\tSTATUS")
	fmt.Fprintln(w, "----\t--------\t----\t----\t---------\t------")

	for _, sb := range sandboxes {
		mode := sb.WorkspaceMode
		if mode == "" {
			mode = "dir"
		}

		status := health.GetSummary(sb.Name, sb.Port, paths.SandboxesDir)
		statusStr := formatStatus(status)

		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\n",
			sb.Name, sb.Template, sb.Port, mode, sb.Workspace, statusStr)
	}

	return w.Flush()
}

func formatStatus(status health.Status) string {
	switch status {
	case health.StatusHealthy:
		return "✓ healthy"
	case health.StatusUnhealthy:
		return "⚠ unhealthy"
	case health.StatusNoTmux:
		return "○ no-tmux"
	case health.StatusStopped:
		return "● stopped"
	default:
		return string(status)
	}
}

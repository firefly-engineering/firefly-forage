package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/health"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/multiplexer"
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
	rt := getRuntime()

	sandboxes, err := listSandboxes()
	if err != nil {
		return fmt.Errorf("failed to list sandboxes: %w", err)
	}

	if len(sandboxes) == 0 {
		logInfo("No sandboxes found. Create one with: forage-ctl up <name> -t <template>")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTEMPLATE\tIP\tMODE\tWORKSPACE\tSTATUS")
	fmt.Fprintln(w, "----\t--------\t--\t----\t---------\t------")

	for _, sb := range sandboxes {
		mode := sb.WorkspaceMode
		mux := multiplexer.New(multiplexer.Type(sb.Multiplexer))
		status := health.GetSummary(sb.Name, sb.ContainerIP(), rt, mux)
		statusStr := formatStatus(status)

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			sb.Name, sb.Template, sb.ContainerIP(), mode, sb.Workspace, statusStr)
	}

	return w.Flush()
}

func formatStatus(status health.Status) string {
	switch status {
	case health.StatusHealthy:
		return "✓ healthy"
	case health.StatusUnhealthy:
		return "⚠ unhealthy"
	case health.StatusNoMux:
		return "○ no-mux"
	case health.StatusStopped:
		return "● stopped"
	default:
		return string(status)
	}
}

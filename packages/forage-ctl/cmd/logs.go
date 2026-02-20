package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
)

var logsCmd = &cobra.Command{
	Use:   "logs <name>",
	Short: "View container logs",
	Args:  cobra.ExactArgs(1),
	RunE:  runLogs,
}

var logsFollow bool
var logsLines int

func init() {
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
	logsCmd.Flags().IntVarP(&logsLines, "lines", "n", 50, "Number of lines to show")
	rootCmd.AddCommand(logsCmd)
}

func runLogs(cmd *cobra.Command, args []string) error {
	name := args[0]
	if _, err := loadSandbox(name); err != nil {
		return err
	}

	rt := getRuntime()
	lv, ok := rt.(runtime.LogViewer)
	if !ok {
		return fmt.Errorf("log viewing is not supported by the %s runtime", rt.Name())
	}

	return lv.ViewLogs(context.Background(), name, logsFollow, logsLines)
}

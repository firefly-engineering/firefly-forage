package cmd

import (
	"fmt"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/system"
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
	paths := config.DefaultPaths()
	metadata, err := config.LoadSandboxMetadata(paths.SandboxesDir, name)
	if err != nil {
		return fmt.Errorf("sandbox not found: %s", name)
	}
	containerName := metadata.ResolvedContainerName()

	journalctlPath, err := exec.LookPath("journalctl")
	if err != nil {
		return fmt.Errorf("journalctl not found: %w", err)
	}

	journalArgs := []string{
		"journalctl",
		"-M", containerName,
		"-n", fmt.Sprintf("%d", logsLines),
	}

	if logsFollow {
		journalArgs = append(journalArgs, "-f")
	}

	return syscall.Exec(journalctlPath, journalArgs, system.SafeEnviron())
}

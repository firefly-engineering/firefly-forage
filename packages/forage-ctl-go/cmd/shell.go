package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/container"
	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:   "shell <name>",
	Short: "Open root shell in container via machinectl",
	Args:  cobra.ExactArgs(1),
	RunE:  runShell,
}

func init() {
	rootCmd.AddCommand(shellCmd)
}

func runShell(cmd *cobra.Command, args []string) error {
	name := args[0]
	paths := config.DefaultPaths()

	_, err := config.LoadSandboxMetadata(paths.SandboxesDir, name)
	if err != nil {
		return fmt.Errorf("sandbox not found: %s", name)
	}

	if !container.IsRunning(name) {
		return fmt.Errorf("sandbox %s is not running", name)
	}

	containerName := config.ContainerName(name)

	machinectlPath, err := exec.LookPath("machinectl")
	if err != nil {
		return fmt.Errorf("machinectl not found: %w", err)
	}

	shellArgs := []string{
		"machinectl",
		"shell",
		containerName,
	}

	return syscall.Exec(machinectlPath, shellArgs, os.Environ())
}

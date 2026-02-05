package cmd

import (
	"context"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/errors"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
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
		return errors.SandboxNotFound(name)
	}

	if !runtime.IsRunning(name) {
		return errors.SandboxNotRunning(name)
	}

	rt := runtime.Global()
	if rt == nil {
		return errors.New(errors.ExitGeneralError, "no container runtime available")
	}

	// Use runtime's interactive exec to get a shell
	return rt.ExecInteractive(context.Background(), name, []string{"/bin/bash"}, runtime.ExecOptions{})
}

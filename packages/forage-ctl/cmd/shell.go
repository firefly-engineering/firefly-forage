package cmd

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/errors"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
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

	_, err := loadRunningSandbox(name)
	if err != nil {
		return err
	}

	rt := runtime.Global()
	if rt == nil {
		return errors.New(errors.ExitGeneralError, "no container runtime available")
	}

	// Use runtime's interactive exec to get a shell
	return rt.ExecInteractive(context.Background(), name, []string{"/bin/bash"}, runtime.ExecOptions{})
}

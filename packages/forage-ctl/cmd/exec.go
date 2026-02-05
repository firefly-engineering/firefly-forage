package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/container"
	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec <name> -- <command>",
	Short: "Execute command in sandbox",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runExec,
}

func init() {
	rootCmd.AddCommand(execCmd)
}

func runExec(cmd *cobra.Command, args []string) error {
	name := args[0]
	paths := config.DefaultPaths()

	metadata, err := config.LoadSandboxMetadata(paths.SandboxesDir, name)
	if err != nil {
		return fmt.Errorf("sandbox not found: %s", name)
	}

	if !container.IsRunning(name) {
		return fmt.Errorf("sandbox %s is not running", name)
	}

	// Find the command to execute (everything after --)
	var execArgs []string
	foundSeparator := false
	for i, arg := range args {
		if arg == "--" {
			execArgs = args[i+1:]
			foundSeparator = true
			break
		}
	}

	if !foundSeparator || len(execArgs) == 0 {
		return fmt.Errorf("usage: forage-ctl exec <name> -- <command>")
	}

	// Build SSH command
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("ssh not found: %w", err)
	}

	// Construct the command string
	cmdStr := execArgs[0]
	for _, arg := range execArgs[1:] {
		cmdStr += " " + shellQuote(arg)
	}

	sshArgs := []string{
		"ssh",
		"-p", fmt.Sprintf("%d", metadata.Port),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"agent@localhost",
		cmdStr,
	}

	return syscall.Exec(sshPath, sshArgs, os.Environ())
}

func shellQuote(s string) string {
	// Simple shell quoting - wrap in single quotes, escape existing single quotes
	result := "'"
	for _, c := range s {
		if c == '\'' {
			result += "'\\''"
		} else {
			result += string(c)
		}
	}
	result += "'"
	return result
}

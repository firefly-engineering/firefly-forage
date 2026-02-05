package cmd

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/errors"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/ssh"
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
		return errors.SandboxNotFound(name)
	}

	if !runtime.IsRunning(name) {
		return errors.SandboxNotRunning(name)
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
		return errors.ValidationError("usage: forage-ctl exec <name> -- <command>")
	}

	// Build SSH command using the builder
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return errors.SSHError("ssh not found", err)
	}

	// Construct the command string
	cmdStr := execArgs[0]
	for _, arg := range execArgs[1:] {
		cmdStr += " " + shellQuote(arg)
	}

	// Use SSH options builder
	opts := ssh.DefaultOptions(metadata.Port)
	sshArgs := opts.BuildArgsWithArgv(cmdStr)

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

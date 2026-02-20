package cmd

import (
	"os/exec"
	"syscall"

	shellquote "github.com/kballard/go-shellquote"
	"github.com/spf13/cobra"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/errors"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/ssh"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/system"
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

	metadata, err := loadRunningSandbox(name)
	if err != nil {
		return err
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

	// Construct the command string with all arguments quoted
	cmdStr := shellquote.Join(execArgs...)

	// Use SSH options builder
	opts := ssh.DefaultOptions(metadata.ContainerIP())
	sshArgs := opts.BuildArgsWithArgv(cmdStr)

	return syscall.Exec(sshPath, sshArgs, system.SafeEnviron())
}


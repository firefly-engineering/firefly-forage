package cmd

import (
	"fmt"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/ssh"
	"github.com/spf13/cobra"
)

var sshCmd = &cobra.Command{
	Use:   "ssh <name>",
	Short: "SSH into a sandbox and attach to tmux session",
	Args:  cobra.ExactArgs(1),
	RunE:  runSSH,
}

func init() {
	rootCmd.AddCommand(sshCmd)
}

func runSSH(cmd *cobra.Command, args []string) error {
	name := args[0]
	paths := config.DefaultPaths()

	metadata, err := config.LoadSandboxMetadata(paths.SandboxesDir, name)
	if err != nil {
		return fmt.Errorf("sandbox not found: %s", name)
	}

	if !runtime.IsRunning(name) {
		return fmt.Errorf("sandbox %s is not running", name)
	}

	// Replace current process with ssh session attached to tmux
	tmuxCmd := "tmux attach-session -t forage || tmux new-session -s forage"
	return ssh.ReplaceWithSession(metadata.Port, tmuxCmd)
}

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/ssh"
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

	metadata, err := loadRunningSandbox(name)
	if err != nil {
		return err
	}

	// Replace current process with ssh session attached to tmux
	tmuxCmd := fmt.Sprintf("tmux attach-session -t %s || tmux new-session -s %s -c /workspace",
		config.TmuxSessionName, config.TmuxSessionName)
	return ssh.ReplaceWithSession(metadata.ContainerIP(), tmuxCmd)
}

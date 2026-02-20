package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/multiplexer"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/ssh"
)

var sshCmd = &cobra.Command{
	Use:   "ssh <name>",
	Short: "SSH into a sandbox and attach to multiplexer session",
	Args:  cobra.ExactArgs(1),
	RunE:  runSSH,
}

func init() {
	sshCmd.Flags().Bool("no-tmux-cc", false, "Disable tmux control mode (-CC) even when WezTerm is detected")
	rootCmd.AddCommand(sshCmd)
}

func runSSH(cmd *cobra.Command, args []string) error {
	name := args[0]

	metadata, err := loadRunningSandbox(name)
	if err != nil {
		return err
	}

	noCC, _ := cmd.Flags().GetBool("no-tmux-cc")
	mux := multiplexer.New(multiplexer.Type(metadata.Multiplexer), multiplexer.WithControlMode(!noCC))

	if attachCmd := mux.AttachCommand(); attachCmd != "" {
		return ssh.ReplaceWithSession(metadata.ContainerIP(), attachCmd)
	}

	// Check if multiplexer supports native connect (e.g., wezterm)
	containerName := metadata.ResolvedContainerName()
	if nc, ok := mux.(multiplexer.NativeConnector); ok {
		if os.Getenv("TERM_PROGRAM") == "WezTerm" {
			return nc.NativeConnect(containerName)
		}
		return fmt.Errorf("sandbox %q uses wezterm multiplexing\n"+
			"  Connect with: wezterm connect %s\n"+
			"  Or configure an SSH domain in ~/.wezterm.lua", name, containerName)
	}

	return fmt.Errorf("multiplexer %q has no attach command and no native connect", mux.Type())
}

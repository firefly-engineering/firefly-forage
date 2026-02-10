package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/multiplexer"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/ssh"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/terminal"
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

	mux := multiplexer.New(multiplexer.Type(metadata.Multiplexer))

	if attachCmd := mux.AttachCommand(); attachCmd != "" {
		noCC, _ := cmd.Flags().GetBool("no-tmux-cc")
		if !noCC && terminal.SupportsControlMode() {
			if tmux, ok := mux.(*multiplexer.Tmux); ok {
				attachCmd = tmux.AttachCommandCC()
			}
		}
		return ssh.ReplaceWithSession(metadata.ContainerIP(), attachCmd)
	}

	// wezterm path: detect terminal, use native connect or error
	if os.Getenv("TERM_PROGRAM") == "WezTerm" {
		return weztermConnect(name)
	}

	return fmt.Errorf("sandbox %q uses wezterm multiplexing\n"+
		"  Connect with: wezterm connect forage-%s\n"+
		"  Or configure an SSH domain in ~/.wezterm.lua", name, name)
}

// weztermConnect execs wezterm connect for the named sandbox.
func weztermConnect(name string) error {
	binary, err := exec.LookPath("wezterm")
	if err != nil {
		return fmt.Errorf("wezterm not found in PATH: %w", err)
	}
	argv := []string{"wezterm", "connect", "forage-" + name}
	return syscall.Exec(binary, argv, os.Environ())
}

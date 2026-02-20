package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/sandbox"
)

var (
	verbose    bool
	jsonOutput bool
)

var rootCmd = &cobra.Command{
	Use:   "forage-ctl",
	Short: "Firefly Forage sandbox management CLI",
	Long: `forage-ctl manages isolated, ephemeral sandboxes for AI coding agents.

Each sandbox is a lightweight container with:
  - Shared nix store (read-only)
  - Ephemeral root filesystem
  - Persistent workspace via bind mount
  - SSH access with tmux session`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		logging.Setup(verbose, jsonOutput, os.Stderr)
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output logs in JSON format")
	rootCmd.CompletionOptions.DisableDefaultCmd = true
}

// Helper aliases for user-facing output (delegates to logging package)
var (
	logInfo    = logging.UserInfo
	logSuccess = logging.UserSuccess
	logWarning = logging.UserWarning
	_          = logging.UserError // reserved for future use
)

// displayInitResult shows init command results to the user.
func displayInitResult(r *sandbox.InitCommandResult) {
	if r == nil {
		return
	}

	if r.TemplateCommandsRun > 0 {
		if len(r.TemplateWarnings) > 0 {
			logWarning("  %d of %d init commands had warnings", len(r.TemplateWarnings), r.TemplateCommandsRun)
			for _, w := range r.TemplateWarnings {
				logWarning("    %s", w)
			}
		} else {
			fmt.Printf("  Init commands: %d completed\n", r.TemplateCommandsRun)
		}
	}

	if r.ProjectInitRun {
		if r.ProjectInitWarning != "" {
			logWarning("  %s", r.ProjectInitWarning)
		} else {
			fmt.Printf("  Project init: completed\n")
		}
	}
}

package cmd

import (
	"os"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
	"github.com/spf13/cobra"
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
	logError   = logging.UserError
)

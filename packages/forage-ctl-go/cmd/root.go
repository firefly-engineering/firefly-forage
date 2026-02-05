package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
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
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global flags can be added here
	rootCmd.CompletionOptions.DisableDefaultCmd = true
}

// Helper functions for consistent output

func logInfo(format string, args ...interface{}) {
	fmt.Fprintf(os.Stdout, "ℹ "+format+"\n", args...)
}

func logSuccess(format string, args ...interface{}) {
	fmt.Fprintf(os.Stdout, "✓ "+format+"\n", args...)
}

func logWarning(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "⚠ "+format+"\n", args...)
}

func logError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "✗ "+format+"\n", args...)
}

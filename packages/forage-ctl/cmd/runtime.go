package cmd

import (
	"fmt"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
	"github.com/spf13/cobra"
)

var runtimeCmd = &cobra.Command{
	Use:   "runtime",
	Short: "Show container runtime information",
	Long: `Display information about available and active container runtimes.

Firefly Forage supports multiple container runtimes:
  - nspawn:  systemd-nspawn via extra-container (NixOS only)
  - apple:   Apple Container (macOS, uses Virtualization.framework)
  - podman:  Podman (rootless containers)
  - docker:  Docker Engine

The runtime is auto-detected based on what's available on your system.`,
	RunE: runRuntime,
}

func init() {
	rootCmd.AddCommand(runtimeCmd)
}

func runRuntime(cmd *cobra.Command, args []string) error {
	// Detect current runtime
	detected, err := runtime.Detect()
	if err != nil {
		fmt.Printf("Detection failed: %s\n", err)
	} else {
		fmt.Printf("Active runtime: %s\n", detected)
	}

	fmt.Println()

	// List available runtimes
	available := runtime.Available()
	fmt.Println("Available runtimes:")
	if len(available) == 0 {
		fmt.Println("  (none)")
	} else {
		for _, rt := range available {
			marker := "  "
			if rt == detected {
				marker = "* "
			}
			fmt.Printf("%s%s\n", marker, rt)
		}
	}

	fmt.Println()

	// Show platform info
	fmt.Println("Platform support:")
	fmt.Println("  nspawn  - NixOS (systemd-nspawn via extra-container)")
	fmt.Println("  apple   - macOS 13+ (Apple Virtualization.framework)")
	fmt.Println("  podman  - Linux, macOS (rootless preferred)")
	fmt.Println("  docker  - Linux, macOS, Windows")

	return nil
}

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/container"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/errors"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/generator"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/network"
	"github.com/spf13/cobra"
)

var networkCmd = &cobra.Command{
	Use:   "network <sandbox> <mode>",
	Short: "Change sandbox network isolation mode",
	Long: `Change the network isolation mode for a sandbox.

Available modes:
  full       - Unrestricted internet access (default)
  restricted - Only allowed hosts can be accessed (requires template config)
  none       - No network access except SSH for management

Note: Changing network mode requires restarting the sandbox.`,
	Args: cobra.ExactArgs(2),
	RunE: runNetwork,
}

var (
	networkAllowHosts []string
	networkNoRestart  bool
)

func init() {
	networkCmd.Flags().StringSliceVar(&networkAllowHosts, "allow", nil, "Additional hosts to allow (restricted mode only)")
	networkCmd.Flags().BoolVar(&networkNoRestart, "no-restart", false, "Don't restart sandbox (changes won't take effect)")
	rootCmd.AddCommand(networkCmd)
}

func runNetwork(cmd *cobra.Command, args []string) error {
	name := args[0]
	modeStr := args[1]
	paths := config.DefaultPaths()

	// Validate mode
	var mode network.Mode
	switch modeStr {
	case "full":
		mode = network.ModeFull
	case "restricted":
		mode = network.ModeRestricted
	case "none":
		mode = network.ModeNone
	default:
		return errors.New(errors.ExitGeneralError, fmt.Sprintf("invalid network mode: %s (use full, restricted, or none)", modeStr))
	}

	logging.Debug("changing network mode", "sandbox", name, "mode", mode)

	// Load sandbox metadata
	metadata, err := config.LoadSandboxMetadata(paths.SandboxesDir, name)
	if err != nil {
		return errors.SandboxNotFound(name)
	}

	// Load host config
	hostConfig, err := config.LoadHostConfig(paths.ConfigDir)
	if err != nil {
		return errors.ConfigError("failed to load host config", err)
	}

	// Load template
	template, err := config.LoadTemplate(paths.TemplatesDir, metadata.Template)
	if err != nil {
		return errors.TemplateNotFound(metadata.Template)
	}

	// Update template network mode for regeneration
	template.Network = string(mode)

	// Merge allowed hosts from template and command line
	allowedHosts := template.AllowedHosts
	if len(networkAllowHosts) > 0 {
		allowedHosts = append(allowedHosts, networkAllowHosts...)
	}
	template.AllowedHosts = allowedHosts

	// Validate restricted mode has allowed hosts
	if mode == network.ModeRestricted && len(allowedHosts) == 0 {
		logWarning("restricted mode with no allowed hosts is equivalent to 'none' mode")
	}

	// Check if sandbox is running
	wasRunning := container.IsRunning(name)
	if wasRunning && !networkNoRestart {
		logInfo("Stopping sandbox for network reconfiguration...")
		logging.Debug("stopping container", "name", name)
		if err := container.Stop(hostConfig.ExtraContainerPath, name); err != nil {
			logging.Warn("failed to stop container", "error", err)
		}
	}

	// Regenerate container configuration
	logInfo("Regenerating container configuration...")

	containerCfg := &generator.ContainerConfig{
		Name:           name,
		Port:           metadata.Port,
		NetworkSlot:    metadata.NetworkSlot,
		Workspace:      metadata.Workspace,
		SecretsPath:    filepath.Join(paths.SecretsDir, name),
		AuthorizedKeys: hostConfig.AuthorizedKeys,
		Template:       template,
		HostConfig:     hostConfig,
		WorkspaceMode:  metadata.WorkspaceMode,
		SourceRepo:     metadata.SourceRepo,
		NixpkgsRev:     hostConfig.NixpkgsRev,
	}

	nixConfig := generator.GenerateNixConfig(containerCfg)

	// Write updated container config
	configPath := filepath.Join(paths.SandboxesDir, name+".nix")
	logging.Debug("writing updated container config", "path", configPath)
	if err := os.WriteFile(configPath, []byte(nixConfig), 0644); err != nil {
		return errors.ContainerFailed("write config", err)
	}

	if networkNoRestart {
		logWarning("Container configuration updated. Restart the sandbox for changes to take effect.")
		logInfo("  forage-ctl reset %s", name)
		return nil
	}

	// Recreate container with new configuration
	logInfo("Recreating container with new network configuration...")
	logging.Debug("running extra-container", "path", hostConfig.ExtraContainerPath, "config", configPath)

	// Destroy old container
	if err := container.Destroy(hostConfig.ExtraContainerPath, name); err != nil {
		logging.Warn("failed to destroy old container", "error", err)
	}

	// Create new container
	createCmd := exec.Command("sudo", hostConfig.ExtraContainerPath, "create", "--start", configPath)
	createCmd.Stdout = os.Stdout
	createCmd.Stderr = os.Stderr
	if err := createCmd.Run(); err != nil {
		return errors.ContainerFailed("recreate container", err)
	}

	logSuccess("Network mode changed to %s", mode)

	// Show network info
	switch mode {
	case network.ModeFull:
		fmt.Println("  Full internet access enabled")
	case network.ModeRestricted:
		fmt.Println("  Restricted network enabled")
		fmt.Println("  Allowed hosts:")
		for _, host := range allowedHosts {
			fmt.Printf("    - %s\n", host)
		}
	case network.ModeNone:
		fmt.Println("  Network access disabled (SSH only for management)")
	}

	return nil
}

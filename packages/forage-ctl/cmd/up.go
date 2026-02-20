package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/errors"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/sandbox"
)

var upCmd = &cobra.Command{
	Use:   "up <name>",
	Short: "Create and start a new sandbox",
	Args:  cobra.ExactArgs(1),
	RunE:  runUp,
}

var (
	upTemplate    string
	upRepo        string
	upSSHKeys     []string
	upNoMuxConfig bool
	upDirect      bool
	upGitUser     string
	upGitEmail    string
	upSSHKeyPath  string
)

func init() {
	upCmd.Flags().StringVarP(&upTemplate, "template", "t", "", "Template to use (required)")
	upCmd.Flags().StringVarP(&upRepo, "repo", "r", "", "Repository or directory path")
	upCmd.Flags().BoolVar(&upDirect, "direct", false, "Mount directory directly (skip VCS isolation)")
	upCmd.Flags().StringArrayVar(&upSSHKeys, "ssh-key", nil, "SSH public key for sandbox access (can be repeated)")
	upCmd.Flags().BoolVar(&upNoMuxConfig, "no-mux-config", false, "Don't mount host multiplexer config into sandbox")
	upCmd.Flags().BoolVar(&upNoMuxConfig, "no-tmux-config", false, "Don't mount host multiplexer config into sandbox")
	_ = upCmd.Flags().MarkDeprecated("no-tmux-config", "use --no-mux-config instead")
	upCmd.Flags().StringVar(&upGitUser, "git-user", "", "Git user.name for agent commits")
	upCmd.Flags().StringVar(&upGitEmail, "git-email", "", "Git user.email for agent commits")
	upCmd.Flags().StringVar(&upSSHKeyPath, "ssh-key-path", "", "Path to SSH private key for agent push access")
	if err := upCmd.MarkFlagRequired("template"); err != nil {
		panic(err)
	}
	if err := upCmd.MarkFlagRequired("repo"); err != nil {
		panic(err)
	}
	rootCmd.AddCommand(upCmd)
}

func runUp(cmd *cobra.Command, args []string) error {
	name := args[0]
	ctx := context.Background()

	// Validate sandbox name early
	if err := config.ValidateSandboxName(name); err != nil {
		return errors.New(errors.ExitGeneralError, err.Error())
	}

	logging.Debug("starting sandbox creation", "name", name, "template", upTemplate)

	// Parse workspace mode from flags
	opts := parseCreateOptions(name)

	// Create the sandbox using the sandbox package
	creator, err := sandbox.NewCreator()
	if err != nil {
		return errors.ConfigError("failed to initialize", err)
	}

	logInfo("Creating sandbox %s...", name)

	result, err := creator.Create(ctx, opts)
	if err != nil {
		return errors.New(errors.ExitGeneralError, err.Error())
	}

	for _, w := range result.CapabilityWarnings {
		logWarning("  %s", w)
	}

	displayInitResult(result.InitResult)

	logSuccess("Sandbox %s created", name)
	fmt.Printf("  IP: %s\n", result.ContainerIP)
	fmt.Printf("  Workspace: %s\n", result.Workspace)
	fmt.Printf("  Connect: forage-ctl ssh %s\n", name)

	return nil
}

// parseCreateOptions parses command flags into CreateOptions.
func parseCreateOptions(name string) sandbox.CreateOptions {
	return sandbox.CreateOptions{
		Name:        name,
		Template:    upTemplate,
		RepoPath:    upRepo,
		Direct:      upDirect,
		SSHKeys:     upSSHKeys,
		NoMuxConfig: upNoMuxConfig,
		GitUser:     upGitUser,
		GitEmail:    upGitEmail,
		SSHKeyPath:  upSSHKeyPath,
	}
}

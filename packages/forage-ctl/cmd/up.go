package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

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
	upRepos       []string
	upSSHKeys     []string
	upNoMuxConfig bool
	upDirect      bool
	upGitUser     string
	upGitEmail    string
	upSSHKeyPath  string
)

func init() {
	upCmd.Flags().StringVarP(&upTemplate, "template", "t", "", "Template to use (required)")
	upCmd.Flags().StringArrayVarP(&upRepos, "repo", "r", nil, "Repository or directory path (repeatable; use name=path for named repos)")
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
	// --repo is no longer unconditionally required; templates with workspace.mounts
	// may fully specify all mount sources without needing --repo.
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
	opts, err := parseCreateOptions(name)
	if err != nil {
		return errors.New(errors.ExitGeneralError, err.Error())
	}

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

// parseRepoFlags parses --repo flags into a default repo path and named repos map.
// Formats:
//   - --repo /path/to/repo          → default repo
//   - --repo name=/path/to/repo     → named repo "name"
func parseRepoFlags(repos []string) (defaultRepo string, namedRepos map[string]string, err error) {
	namedRepos = make(map[string]string)
	for _, r := range repos {
		if idx := strings.IndexByte(r, '='); idx > 0 {
			name := r[:idx]
			path := r[idx+1:]
			absPath, absErr := filepath.Abs(path)
			if absErr != nil {
				return "", nil, fmt.Errorf("invalid repo path for %q: %w", name, absErr)
			}
			namedRepos[name] = absPath
		} else {
			if defaultRepo != "" {
				return "", nil, fmt.Errorf("multiple default repos specified; use name=path for additional repos")
			}
			defaultRepo = r
		}
	}
	return defaultRepo, namedRepos, nil
}

// parseCreateOptions parses command flags into CreateOptions.
func parseCreateOptions(name string) (sandbox.CreateOptions, error) {
	defaultRepo, namedRepos, err := parseRepoFlags(upRepos)
	if err != nil {
		return sandbox.CreateOptions{}, err
	}

	return sandbox.CreateOptions{
		Name:        name,
		Template:    upTemplate,
		RepoPath:    defaultRepo,
		Repos:       namedRepos,
		Direct:      upDirect,
		SSHKeys:     upSSHKeys,
		NoMuxConfig: upNoMuxConfig,
		GitUser:     upGitUser,
		GitEmail:    upGitEmail,
		SSHKeyPath:  upSSHKeyPath,
	}, nil
}

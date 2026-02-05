package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/container"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/errors"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/generator"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/health"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/logging"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/port"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/skills"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/workspace"
	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up <name>",
	Short: "Create and start a new sandbox",
	Args:  cobra.ExactArgs(1),
	RunE:  runUp,
}

var (
	upTemplate    string
	upWorkspace   string
	upRepo        string
	upGitWorktree string
)

func init() {
	upCmd.Flags().StringVarP(&upTemplate, "template", "t", "", "Template to use (required)")
	upCmd.Flags().StringVarP(&upWorkspace, "workspace", "w", "", "Workspace directory to mount")
	upCmd.Flags().StringVarP(&upRepo, "repo", "r", "", "JJ repository (creates isolated workspace)")
	upCmd.Flags().StringVarP(&upGitWorktree, "git-worktree", "g", "", "Git repository (creates isolated worktree)")
	upCmd.MarkFlagRequired("template")
	rootCmd.AddCommand(upCmd)
}

func runUp(cmd *cobra.Command, args []string) error {
	name := args[0]
	paths := config.DefaultPaths()

	logging.Debug("starting sandbox creation", "name", name, "template", upTemplate)

	// Validate flags - exactly one of --workspace, --repo, or --git-worktree must be specified
	flagCount := 0
	if upWorkspace != "" {
		flagCount++
	}
	if upRepo != "" {
		flagCount++
	}
	if upGitWorktree != "" {
		flagCount++
	}
	if flagCount == 0 {
		return errors.New(errors.ExitGeneralError, "one of --workspace, --repo, or --git-worktree is required")
	}
	if flagCount > 1 {
		return errors.New(errors.ExitGeneralError, "--workspace, --repo, and --git-worktree are mutually exclusive")
	}

	// Check if sandbox already exists
	if config.SandboxExists(paths.SandboxesDir, name) {
		return errors.New(errors.ExitGeneralError, fmt.Sprintf("sandbox %s already exists", name))
	}

	// Load configurations
	logging.Debug("loading host config", "configDir", paths.ConfigDir)
	hostConfig, err := config.LoadHostConfig(paths.ConfigDir)
	if err != nil {
		return errors.ConfigError("failed to load host config", err)
	}

	logging.Debug("loading template", "template", upTemplate)
	template, err := config.LoadTemplate(paths.TemplatesDir, upTemplate)
	if err != nil {
		return errors.TemplateNotFound(upTemplate)
	}

	// List existing sandboxes for port allocation
	sandboxes, err := config.ListSandboxes(paths.SandboxesDir)
	if err != nil {
		logging.Debug("no existing sandboxes found", "error", err)
		sandboxes = []*config.SandboxMetadata{}
	}

	// Allocate port and network slot
	logging.Debug("allocating port", "portRange", hostConfig.PortRange)
	allocatedPort, networkSlot, err := port.Allocate(hostConfig, sandboxes)
	if err != nil {
		return errors.PortAllocationFailed(err)
	}
	logging.Debug("port allocated", "port", allocatedPort, "slot", networkSlot)

	// Determine workspace mode and effective workspace path
	var workspaceMode string
	var effectiveWorkspace string
	var sourceRepo string
	var gitBranch string
	var backend workspace.Backend

	if upRepo != "" || upGitWorktree != "" {
		// VCS workspace mode (jj or git-worktree)
		var repoPath string
		if upRepo != "" {
			repoPath = upRepo
			backend = workspace.JJ()
		} else {
			repoPath = upGitWorktree
			backend = workspace.Git()
		}

		absRepo, err := filepath.Abs(repoPath)
		if err != nil {
			return errors.New(errors.ExitGeneralError, fmt.Sprintf("invalid repo path: %s", err))
		}

		logging.Debug("checking repository", "backend", backend.Name(), "path", absRepo)
		if !backend.IsRepo(absRepo) {
			return errors.New(errors.ExitGeneralError, fmt.Sprintf("not a %s repository: %s", backend.Name(), absRepo))
		}

		if backend.Exists(absRepo, name) {
			return errors.New(errors.ExitGeneralError, fmt.Sprintf("%s workspace %s already exists in repo", backend.Name(), name))
		}

		workspaceMode = backend.Name()
		sourceRepo = absRepo
		effectiveWorkspace = filepath.Join(paths.WorkspacesDir, name)

		// For git backend, store the branch name
		if gitBackend, ok := backend.(*workspace.GitBackend); ok {
			gitBranch = gitBackend.BranchName(name)
		}

		// Create workspace
		logInfo("Creating %s workspace...", backend.Name())
		logging.Debug("creating workspaces directory", "path", paths.WorkspacesDir)
		if err := os.MkdirAll(paths.WorkspacesDir, 0755); err != nil {
			return errors.New(errors.ExitGeneralError, fmt.Sprintf("failed to create workspaces directory: %s", err))
		}

		logging.Debug("creating workspace", "backend", backend.Name(), "repo", absRepo, "name", name, "path", effectiveWorkspace)
		if err := backend.Create(absRepo, name, effectiveWorkspace); err != nil {
			return errors.New(errors.ExitGeneralError, fmt.Sprintf("failed to create %s workspace: %s", backend.Name(), err))
		}
	} else {
		// Direct workspace mode
		absWorkspace, err := filepath.Abs(upWorkspace)
		if err != nil {
			return errors.New(errors.ExitGeneralError, fmt.Sprintf("invalid workspace path: %s", err))
		}

		if _, err := os.Stat(absWorkspace); os.IsNotExist(err) {
			return errors.New(errors.ExitGeneralError, fmt.Sprintf("workspace does not exist: %s", absWorkspace))
		}

		workspaceMode = "direct"
		effectiveWorkspace = absWorkspace
	}

	logging.Debug("workspace configured", "mode", workspaceMode, "path", effectiveWorkspace)

	// Create metadata
	metadata := &config.SandboxMetadata{
		Name:            name,
		Template:        upTemplate,
		Port:            allocatedPort,
		Workspace:       effectiveWorkspace,
		NetworkSlot:     networkSlot,
		CreatedAt:       time.Now().Format(time.RFC3339),
		WorkspaceMode:   workspaceMode,
		SourceRepo:      sourceRepo,
		JJWorkspaceName: name, // For jj mode, workspace name equals sandbox name
		GitBranch:       gitBranch,
	}

	// Set up secrets
	secretsPath := filepath.Join(paths.SecretsDir, name)
	logging.Debug("setting up secrets", "path", secretsPath)
	if err := setupSecrets(secretsPath, template, hostConfig); err != nil {
		cleanup(metadata, paths, hostConfig, backend)
		return errors.ConfigError("failed to setup secrets", err)
	}

	// Determine proxy URL
	proxyURL := ""
	if template.UseProxy && hostConfig.ProxyURL != "" {
		proxyURL = hostConfig.ProxyURL
		logging.Debug("using API proxy", "url", proxyURL)
	}

	// Generate container configuration
	containerCfg := &generator.ContainerConfig{
		Name:           name,
		Port:           allocatedPort,
		NetworkSlot:    networkSlot,
		Workspace:      effectiveWorkspace,
		SecretsPath:    secretsPath,
		AuthorizedKeys: hostConfig.AuthorizedKeys,
		Template:       template,
		HostConfig:     hostConfig,
		WorkspaceMode:  workspaceMode,
		SourceRepo:     sourceRepo,
		NixpkgsRev:     hostConfig.NixpkgsRev,
		ProxyURL:       proxyURL,
	}

	nixConfig := generator.GenerateNixConfig(containerCfg)

	// Write container config
	configPath := filepath.Join(paths.SandboxesDir, name+".nix")
	logging.Debug("writing container config", "path", configPath)
	if err := os.MkdirAll(paths.SandboxesDir, 0755); err != nil {
		cleanup(metadata, paths, hostConfig, backend)
		return errors.ContainerFailed("create sandboxes directory", err)
	}
	if err := os.WriteFile(configPath, []byte(nixConfig), 0644); err != nil {
		cleanup(metadata, paths, hostConfig, backend)
		return errors.ContainerFailed("write config", err)
	}

	// Create container with extra-container
	logInfo("Creating container...")
	logging.Debug("running extra-container", "path", hostConfig.ExtraContainerPath, "config", configPath)
	createCmd := exec.Command("sudo", hostConfig.ExtraContainerPath, "create", "--start", configPath)
	createCmd.Stdout = os.Stdout
	createCmd.Stderr = os.Stderr
	if err := createCmd.Run(); err != nil {
		cleanup(metadata, paths, hostConfig, backend)
		return errors.ContainerFailed("create", err)
	}

	// Save metadata
	logging.Debug("saving metadata")
	if err := config.SaveSandboxMetadata(paths.SandboxesDir, metadata); err != nil {
		cleanup(metadata, paths, hostConfig, backend)
		return errors.ConfigError("failed to save metadata", err)
	}

	// Wait for SSH to be ready
	logInfo("Waiting for sandbox to be ready...")
	logging.Debug("waiting for SSH", "port", allocatedPort, "timeout", 30)
	if !waitForSSH(allocatedPort, 30) {
		logWarning("SSH not ready after 30 seconds, sandbox may still be starting")
	}

	// Analyze project and inject skills file
	logging.Debug("analyzing workspace for project-aware skills", "path", effectiveWorkspace)
	analyzer := skills.NewAnalyzer(effectiveWorkspace)
	projectInfo := analyzer.Analyze()
	logging.Debug("project analysis complete",
		"type", projectInfo.Type,
		"buildSystem", projectInfo.BuildSystem,
		"frameworks", projectInfo.Frameworks)

	skillsContent := skills.GenerateSkills(metadata, template, projectInfo)
	skillsPath := filepath.Join(paths.SandboxesDir, name+".skills.md")
	if err := os.WriteFile(skillsPath, []byte(skillsContent), 0644); err != nil {
		logging.Warn("failed to save skills file", "error", err)
	}

	// Copy skills to container workspace
	logging.Debug("injecting skills into container")
	if err := copySkillsToContainer(allocatedPort, skillsContent); err != nil {
		logging.Warn("failed to inject skills", "error", err)
	}

	logSuccess("Sandbox %s created", name)
	fmt.Printf("  Port: %d\n", allocatedPort)
	fmt.Printf("  Workspace: %s\n", effectiveWorkspace)
	fmt.Printf("  Connect: forage-ctl ssh %s\n", name)

	return nil
}

func setupSecrets(secretsPath string, template *config.Template, hostConfig *config.HostConfig) error {
	if err := os.MkdirAll(secretsPath, 0700); err != nil {
		return err
	}

	for _, agent := range template.Agents {
		if agent.SecretName == "" {
			continue
		}

		secretValue, ok := hostConfig.Secrets[agent.SecretName]
		if !ok {
			logging.Debug("secret not found in host config", "secret", agent.SecretName)
			continue
		}

		secretFile := filepath.Join(secretsPath, agent.SecretName)
		if err := os.WriteFile(secretFile, []byte(secretValue), 0600); err != nil {
			return fmt.Errorf("failed to write secret %s: %w", agent.SecretName, err)
		}
		logging.Debug("secret written", "secret", agent.SecretName)
	}

	return nil
}

func waitForSSH(port int, timeoutSeconds int) bool {
	for i := 0; i < timeoutSeconds; i++ {
		if health.CheckSSH(port) {
			logging.Debug("SSH ready", "attempt", i+1)
			return true
		}
		time.Sleep(time.Second)
	}
	return false
}

func copySkillsToContainer(port int, content string) error {
	// Use SSH to write the file
	_, err := container.ExecSSHWithOutput(port, "bash", "-c",
		fmt.Sprintf("cat > /workspace/CLAUDE.md << 'SKILLS_EOF'\n%s\nSKILLS_EOF", content))
	return err
}

func cleanup(metadata *config.SandboxMetadata, paths *config.Paths, hostConfig *config.HostConfig, backend workspace.Backend) {
	logging.Debug("cleaning up failed sandbox creation", "name", metadata.Name)

	// Clean up workspace if created via backend
	if backend != nil && metadata.SourceRepo != "" && metadata.Workspace != "" {
		logging.Debug("removing workspace", "backend", backend.Name(), "name", metadata.Name)
		backend.Remove(metadata.SourceRepo, metadata.Name, metadata.Workspace)
	}

	// Clean up secrets
	secretsPath := filepath.Join(paths.SecretsDir, metadata.Name)
	os.RemoveAll(secretsPath)

	// Clean up config file
	configPath := filepath.Join(paths.SandboxesDir, metadata.Name+".nix")
	os.Remove(configPath)

	// Clean up metadata
	config.DeleteSandboxMetadata(paths.SandboxesDir, metadata.Name)

	// Try to destroy container if it was created
	container.Destroy(hostConfig.ExtraContainerPath, metadata.Name)
}

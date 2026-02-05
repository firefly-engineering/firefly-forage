package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/container"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/generator"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/health"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/jj"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/port"
	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up <name>",
	Short: "Create and start a new sandbox",
	Args:  cobra.ExactArgs(1),
	RunE:  runUp,
}

var (
	upTemplate  string
	upWorkspace string
	upRepo      string
)

func init() {
	upCmd.Flags().StringVarP(&upTemplate, "template", "t", "", "Template to use (required)")
	upCmd.Flags().StringVarP(&upWorkspace, "workspace", "w", "", "Workspace directory to mount")
	upCmd.Flags().StringVarP(&upRepo, "repo", "r", "", "JJ repository (creates isolated workspace)")
	upCmd.MarkFlagRequired("template")
	rootCmd.AddCommand(upCmd)
}

func runUp(cmd *cobra.Command, args []string) error {
	name := args[0]
	paths := config.DefaultPaths()

	// Validate flags
	if upWorkspace != "" && upRepo != "" {
		return fmt.Errorf("--workspace and --repo are mutually exclusive")
	}
	if upWorkspace == "" && upRepo == "" {
		return fmt.Errorf("either --workspace or --repo is required")
	}

	// Check if sandbox already exists
	if config.SandboxExists(paths.SandboxesDir, name) {
		return fmt.Errorf("sandbox %s already exists", name)
	}

	// Load configurations
	hostConfig, err := config.LoadHostConfig(paths.ConfigDir)
	if err != nil {
		return fmt.Errorf("failed to load host config: %w", err)
	}

	template, err := config.LoadTemplate(paths.TemplatesDir, upTemplate)
	if err != nil {
		return fmt.Errorf("template not found: %s", upTemplate)
	}

	// List existing sandboxes for port allocation
	sandboxes, err := config.ListSandboxes(paths.SandboxesDir)
	if err != nil {
		sandboxes = []*config.SandboxMetadata{}
	}

	// Allocate port and network slot
	allocatedPort, networkSlot, err := port.Allocate(hostConfig, sandboxes)
	if err != nil {
		return fmt.Errorf("failed to allocate port: %w", err)
	}

	// Determine workspace mode and effective workspace path
	var workspaceMode string
	var effectiveWorkspace string
	var sourceRepo string
	var jjWorkspaceName string

	if upRepo != "" {
		// JJ mode
		absRepo, err := filepath.Abs(upRepo)
		if err != nil {
			return fmt.Errorf("invalid repo path: %w", err)
		}

		if !jj.IsRepo(absRepo) {
			return fmt.Errorf("not a jj repository: %s", absRepo)
		}

		if jj.WorkspaceExists(absRepo, name) {
			return fmt.Errorf("jj workspace %s already exists in repo", name)
		}

		workspaceMode = "jj"
		sourceRepo = absRepo
		jjWorkspaceName = name
		effectiveWorkspace = filepath.Join(paths.WorkspacesDir, name)

		// Create jj workspace
		logInfo("Creating jj workspace...")
		if err := os.MkdirAll(paths.WorkspacesDir, 0755); err != nil {
			return fmt.Errorf("failed to create workspaces directory: %w", err)
		}

		if err := jj.CreateWorkspace(absRepo, name, effectiveWorkspace); err != nil {
			return fmt.Errorf("failed to create jj workspace: %w", err)
		}
	} else {
		// Direct workspace mode
		absWorkspace, err := filepath.Abs(upWorkspace)
		if err != nil {
			return fmt.Errorf("invalid workspace path: %w", err)
		}

		if _, err := os.Stat(absWorkspace); os.IsNotExist(err) {
			return fmt.Errorf("workspace does not exist: %s", absWorkspace)
		}

		workspaceMode = "direct"
		effectiveWorkspace = absWorkspace
	}

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
		JJWorkspaceName: jjWorkspaceName,
	}

	// Set up secrets
	secretsPath := filepath.Join(paths.SecretsDir, name)
	if err := setupSecrets(secretsPath, template, hostConfig); err != nil {
		cleanup(metadata, paths, hostConfig)
		return fmt.Errorf("failed to setup secrets: %w", err)
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
	}

	nixConfig := generator.GenerateNixConfig(containerCfg)

	// Write container config
	configPath := filepath.Join(paths.SandboxesDir, name+".nix")
	if err := os.MkdirAll(paths.SandboxesDir, 0755); err != nil {
		cleanup(metadata, paths, hostConfig)
		return fmt.Errorf("failed to create sandboxes directory: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(nixConfig), 0644); err != nil {
		cleanup(metadata, paths, hostConfig)
		return fmt.Errorf("failed to write container config: %w", err)
	}

	// Create container with extra-container
	logInfo("Creating container...")
	createCmd := exec.Command("sudo", hostConfig.ExtraContainerPath, "create", "--start", configPath)
	createCmd.Stdout = os.Stdout
	createCmd.Stderr = os.Stderr
	if err := createCmd.Run(); err != nil {
		cleanup(metadata, paths, hostConfig)
		return fmt.Errorf("failed to create container: %w", err)
	}

	// Save metadata
	if err := config.SaveSandboxMetadata(paths.SandboxesDir, metadata); err != nil {
		cleanup(metadata, paths, hostConfig)
		return fmt.Errorf("failed to save metadata: %w", err)
	}

	// Wait for SSH to be ready
	logInfo("Waiting for sandbox to be ready...")
	if !waitForSSH(allocatedPort, 30) {
		logWarning("SSH not ready after 30 seconds, sandbox may still be starting")
	}

	// Inject skills file
	skillsContent := generator.GenerateSkills(metadata, template)
	skillsPath := filepath.Join(paths.SandboxesDir, name+".skills.md")
	if err := os.WriteFile(skillsPath, []byte(skillsContent), 0644); err != nil {
		logWarning("Failed to save skills file: %v", err)
	}

	// Copy skills to container workspace
	if err := copySkillsToContainer(allocatedPort, skillsContent); err != nil {
		logWarning("Failed to inject skills: %v", err)
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
			continue
		}

		secretFile := filepath.Join(secretsPath, agent.SecretName)
		if err := os.WriteFile(secretFile, []byte(secretValue), 0600); err != nil {
			return fmt.Errorf("failed to write secret %s: %w", agent.SecretName, err)
		}
	}

	return nil
}

func waitForSSH(port int, timeoutSeconds int) bool {
	for i := 0; i < timeoutSeconds; i++ {
		if health.CheckSSH(port) {
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

func cleanup(metadata *config.SandboxMetadata, paths *config.Paths, hostConfig *config.HostConfig) {
	// Clean up jj workspace if created
	if metadata.WorkspaceMode == "jj" && metadata.SourceRepo != "" && metadata.JJWorkspaceName != "" {
		jj.ForgetWorkspace(metadata.SourceRepo, metadata.JJWorkspaceName)
		if metadata.Workspace != "" {
			os.RemoveAll(metadata.Workspace)
		}
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

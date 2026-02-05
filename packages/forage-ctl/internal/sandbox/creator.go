package sandbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/generator"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/health"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/port"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/skills"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/ssh"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/workspace"
)

// Creator handles sandbox creation with all necessary dependencies.
type Creator struct {
	paths      *config.Paths
	hostConfig *config.HostConfig
	rt         runtime.Runtime
}

// NewCreator creates a new sandbox Creator with default configuration.
func NewCreator() (*Creator, error) {
	paths := config.DefaultPaths()

	hostConfig, err := config.LoadHostConfig(paths.ConfigDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load host config: %w", err)
	}

	return &Creator{
		paths:      paths,
		hostConfig: hostConfig,
		rt:         runtime.Global(),
	}, nil
}

// Create creates a new sandbox with the given options.
func (c *Creator) Create(ctx context.Context, opts CreateOptions) (*CreateResult, error) {
	logging.Debug("starting sandbox creation", "name", opts.Name, "template", opts.Template)

	// Validate sandbox name
	if err := config.ValidateSandboxName(opts.Name); err != nil {
		return nil, fmt.Errorf("invalid sandbox name: %w", err)
	}

	// Check if sandbox already exists
	if config.SandboxExists(c.paths.SandboxesDir, opts.Name) {
		return nil, fmt.Errorf("sandbox %s already exists", opts.Name)
	}

	// Load template
	template, err := config.LoadTemplate(c.paths.TemplatesDir, opts.Template)
	if err != nil {
		return nil, fmt.Errorf("template not found: %s", opts.Template)
	}

	// List existing sandboxes for port allocation
	sandboxes, err := config.ListSandboxes(c.paths.SandboxesDir)
	if err != nil {
		logging.Debug("no existing sandboxes found", "error", err)
		sandboxes = []*config.SandboxMetadata{}
	}

	// Allocate port and network slot
	allocatedPort, networkSlot, err := port.Allocate(c.hostConfig, sandboxes)
	if err != nil {
		return nil, fmt.Errorf("port allocation failed: %w", err)
	}
	logging.Debug("port allocated", "port", allocatedPort, "slot", networkSlot)

	// Set up workspace
	ws, err := c.setupWorkspace(opts)
	if err != nil {
		return nil, err
	}

	// Create metadata
	metadata := &config.SandboxMetadata{
		Name:            opts.Name,
		Template:        opts.Template,
		Port:            allocatedPort,
		Workspace:       ws.effectivePath,
		NetworkSlot:     networkSlot,
		CreatedAt:       time.Now().Format(time.RFC3339),
		WorkspaceMode:   string(opts.WorkspaceMode),
		SourceRepo:      ws.sourceRepo,
		JJWorkspaceName: opts.Name,
		GitBranch:       ws.gitBranch,
	}

	// Set up cleanup on failure
	cleanup := func() {
		c.cleanup(metadata, ws.backend)
	}

	// Set up secrets
	secretsPath := filepath.Join(c.paths.SecretsDir, opts.Name)
	if err := c.setupSecrets(secretsPath, template); err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to setup secrets: %w", err)
	}

	// Determine proxy URL
	proxyURL := ""
	if template.UseProxy && c.hostConfig.ProxyURL != "" {
		proxyURL = c.hostConfig.ProxyURL
		logging.Debug("using API proxy", "url", proxyURL)
	}

	// Generate container configuration
	containerCfg := &generator.ContainerConfig{
		Name:           opts.Name,
		Port:           allocatedPort,
		NetworkSlot:    networkSlot,
		Workspace:      ws.effectivePath,
		SecretsPath:    secretsPath,
		AuthorizedKeys: c.hostConfig.AuthorizedKeys,
		Template:       template,
		HostConfig:     c.hostConfig,
		WorkspaceMode:  string(opts.WorkspaceMode),
		SourceRepo:     ws.sourceRepo,
		NixpkgsRev:     c.hostConfig.NixpkgsRev,
		ProxyURL:       proxyURL,
	}

	nixConfig := generator.GenerateNixConfig(containerCfg)

	// Write container config
	configPath := filepath.Join(c.paths.SandboxesDir, opts.Name+".nix")
	if err := os.MkdirAll(c.paths.SandboxesDir, 0755); err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to create sandboxes directory: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(nixConfig), 0644); err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to write config: %w", err)
	}

	// Create container via runtime
	logging.Debug("creating container via runtime", "name", opts.Name, "config", configPath)
	if err := runtime.Create(runtime.CreateOptions{
		Name:       opts.Name,
		ConfigPath: configPath,
		Start:      true,
		SSHPort:    allocatedPort,
	}); err != nil {
		cleanup()
		return nil, fmt.Errorf("container creation failed: %w", err)
	}

	// Save metadata
	if err := config.SaveSandboxMetadata(c.paths.SandboxesDir, metadata); err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to save metadata: %w", err)
	}

	// Wait for SSH to be ready
	logging.Debug("waiting for SSH", "port", allocatedPort, "timeout", health.SSHReadyTimeoutSeconds)
	c.waitForSSH(allocatedPort, health.SSHReadyTimeoutSeconds)

	// Inject skills
	c.injectSkills(opts.Name, ws.effectivePath, metadata, template)

	return &CreateResult{
		Name:      opts.Name,
		Port:      allocatedPort,
		Workspace: ws.effectivePath,
		Metadata:  metadata,
	}, nil
}

// workspaceSetup holds workspace setup results.
type workspaceSetup struct {
	effectivePath string
	sourceRepo    string
	gitBranch     string
	backend       workspace.Backend
}

// setupWorkspace sets up the workspace based on the mode.
func (c *Creator) setupWorkspace(opts CreateOptions) (*workspaceSetup, error) {
	ws := &workspaceSetup{}

	switch opts.WorkspaceMode {
	case WorkspaceModeJJ, WorkspaceModeGitWorktree:
		ws.backend = workspaceBackendFor(opts.WorkspaceMode)

		absRepo, err := filepath.Abs(opts.RepoPath)
		if err != nil {
			return nil, fmt.Errorf("invalid repo path: %w", err)
		}

		if !ws.backend.IsRepo(absRepo) {
			return nil, fmt.Errorf("not a %s repository: %s", ws.backend.Name(), absRepo)
		}

		if ws.backend.Exists(absRepo, opts.Name) {
			return nil, fmt.Errorf("%s workspace %s already exists in repo", ws.backend.Name(), opts.Name)
		}

		ws.sourceRepo = absRepo
		ws.effectivePath = filepath.Join(c.paths.WorkspacesDir, opts.Name)

		// For git backend, store the branch name
		if gitBackend, ok := ws.backend.(*workspace.GitBackend); ok {
			ws.gitBranch = gitBackend.BranchName(opts.Name)
		}

		// Create workspace directory and workspace
		if err := os.MkdirAll(c.paths.WorkspacesDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create workspaces directory: %w", err)
		}

		logging.Debug("creating workspace", "backend", ws.backend.Name(), "repo", absRepo, "name", opts.Name)
		if err := ws.backend.Create(absRepo, opts.Name, ws.effectivePath); err != nil {
			return nil, fmt.Errorf("failed to create %s workspace: %w", ws.backend.Name(), err)
		}

	case WorkspaceModeDirect, "":
		absWorkspace, err := filepath.Abs(opts.WorkspacePath)
		if err != nil {
			return nil, fmt.Errorf("invalid workspace path: %w", err)
		}

		if _, err := os.Stat(absWorkspace); os.IsNotExist(err) {
			return nil, fmt.Errorf("workspace does not exist: %s", absWorkspace)
		}

		ws.effectivePath = absWorkspace
	}

	return ws, nil
}

// setupSecrets copies secrets to the sandbox secrets directory.
func (c *Creator) setupSecrets(secretsPath string, template *config.Template) error {
	if err := os.MkdirAll(secretsPath, 0700); err != nil {
		return err
	}

	for _, agent := range template.Agents {
		if agent.SecretName == "" {
			continue
		}

		secretValue, ok := c.hostConfig.Secrets[agent.SecretName]
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

// waitForSSH waits for SSH to be ready on the given port.
func (c *Creator) waitForSSH(port int, timeoutSeconds int) bool {
	for i := 0; i < timeoutSeconds; i++ {
		if health.CheckSSH(port) {
			logging.Debug("SSH ready", "attempt", i+1)
			return true
		}
		time.Sleep(time.Second)
	}
	logging.Warn("SSH not ready after timeout", "timeout", timeoutSeconds)
	return false
}

// injectSkills analyzes the workspace and injects skills file.
func (c *Creator) injectSkills(name, workspacePath string, metadata *config.SandboxMetadata, template *config.Template) {
	logging.Debug("analyzing workspace for project-aware skills", "path", workspacePath)
	analyzer := skills.NewAnalyzer(workspacePath)
	projectInfo := analyzer.Analyze()
	logging.Debug("project analysis complete",
		"type", projectInfo.Type,
		"buildSystem", projectInfo.BuildSystem,
		"frameworks", projectInfo.Frameworks)

	skillsContent := skills.GenerateSkills(metadata, template, projectInfo)
	skillsPath := filepath.Join(c.paths.SandboxesDir, name+".skills.md")
	if err := os.WriteFile(skillsPath, []byte(skillsContent), 0644); err != nil {
		logging.Warn("failed to save skills file", "error", err)
	}

	// Copy skills to container workspace
	logging.Debug("injecting skills into container")
	if err := copySkillsToContainer(metadata.Port, skillsContent); err != nil {
		logging.Warn("failed to inject skills", "error", err)
	}
}

// cleanup removes resources created during a failed sandbox creation.
func (c *Creator) cleanup(metadata *config.SandboxMetadata, backend workspace.Backend) {
	logging.Debug("cleaning up failed sandbox creation", "name", metadata.Name)

	// Use unified cleanup function with all options enabled
	Cleanup(metadata, c.paths, DefaultCleanupOptions())
}

// copySkillsToContainer copies skills content into the container.
// Uses stdin piping to safely transfer content without shell injection risks.
// The content is passed via stdin to avoid any shell interpolation or heredoc escaping issues.
func copySkillsToContainer(port int, content string) error {
	// Pass content via stdin to sh, which writes it to the file.
	// This is safe because content never appears in the command string.
	return ssh.ExecWithStdin(port, content, "sh", "-c", "cat > /workspace/CLAUDE.md")
}

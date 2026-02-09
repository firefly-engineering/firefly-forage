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

	// Phase 1: Validate inputs
	if err := c.validateInputs(opts); err != nil {
		return nil, err
	}

	// Resolve and validate agent identity
	identity := c.resolveIdentity(opts)
	if err := config.ValidateAgentIdentity(identity); err != nil {
		return nil, fmt.Errorf("invalid agent identity: %w", err)
	}

	// Phase 2: Load resources and allocate ports
	resources, err := c.loadResources(opts)
	if err != nil {
		return nil, err
	}

	// Phase 3: Set up workspace
	ws, err := c.setupWorkspace(opts)
	if err != nil {
		return nil, err
	}

	// Phase 4: Create metadata
	metadata := c.createMetadata(opts, resources, ws, identity)

	// Set up cleanup on failure
	cleanup := func() {
		c.cleanup(metadata)
	}

	// Phase 5: Set up secrets (only if any agent uses secrets)
	var secretsPath string
	if c.templateHasSecrets(resources.template) {
		secretsPath = filepath.Join(c.paths.SecretsDir, opts.Name)
		if err = c.setupSecrets(secretsPath, resources.template); err != nil {
			cleanup()
			return nil, fmt.Errorf("failed to setup secrets: %w", err)
		}
	}

	// Phase 6: Generate permissions files
	permsMounts, err := c.generatePermissionsFiles(opts.Name, resources.template)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to generate permissions files: %w", err)
	}

	// Phase 7: Generate and write container config
	configPath, err := c.writeContainerConfig(opts, resources, ws, secretsPath, permsMounts, identity)
	if err != nil {
		cleanup()
		return nil, err
	}

	// Phase 8: Create and start container
	if err := c.startContainer(opts.Name, configPath); err != nil {
		cleanup()
		return nil, err
	}

	// Phase 9: Save metadata
	if err := config.SaveSandboxMetadata(c.paths.SandboxesDir, metadata); err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to save metadata: %w", err)
	}

	// Phase 10: Post-creation setup
	c.postCreationSetup(metadata, resources.template, ws.effectivePath)

	return &CreateResult{
		Name:        opts.Name,
		ContainerIP: metadata.ContainerIP(),
		Workspace:   ws.effectivePath,
		Metadata:    metadata,
	}, nil
}

// resourceAllocation holds loaded resources and allocated network slot.
type resourceAllocation struct {
	template    *config.Template
	networkSlot int
}

// validateInputs validates the sandbox creation inputs.
func (c *Creator) validateInputs(opts CreateOptions) error {
	if err := config.ValidateSandboxName(opts.Name); err != nil {
		return fmt.Errorf("invalid sandbox name: %w", err)
	}

	if config.SandboxExists(c.paths.SandboxesDir, opts.Name) {
		return fmt.Errorf("sandbox %s already exists", opts.Name)
	}

	return nil
}

// loadResources loads the template and allocates network slot.
func (c *Creator) loadResources(opts CreateOptions) (*resourceAllocation, error) {
	template, err := config.LoadTemplate(c.paths.TemplatesDir, opts.Template)
	if err != nil {
		return nil, fmt.Errorf("template not found: %s", opts.Template)
	}

	sandboxes, err := config.ListSandboxes(c.paths.SandboxesDir)
	if err != nil {
		logging.Debug("no existing sandboxes found", "error", err)
		sandboxes = []*config.SandboxMetadata{}
	}

	networkSlot, err := port.AllocateSlot(sandboxes)
	if err != nil {
		return nil, fmt.Errorf("slot allocation failed: %w", err)
	}
	logging.Debug("network slot allocated", "slot", networkSlot)

	return &resourceAllocation{
		template:    template,
		networkSlot: networkSlot,
	}, nil
}

// createMetadata creates the sandbox metadata struct.
func (c *Creator) createMetadata(opts CreateOptions, resources *resourceAllocation, ws *workspaceSetup, identity *config.AgentIdentity) *config.SandboxMetadata {
	return &config.SandboxMetadata{
		Name:            opts.Name,
		Template:        opts.Template,
		Workspace:       ws.effectivePath,
		NetworkSlot:     resources.networkSlot,
		CreatedAt:       time.Now().Format(time.RFC3339),
		WorkspaceMode:   string(ws.mode),
		SourceRepo:      ws.sourceRepo,
		JJWorkspaceName: opts.Name,
		GitBranch:       ws.gitBranch,
		AgentIdentity:   identity,
	}
}

// writeContainerConfig generates and writes the Nix container configuration.
func (c *Creator) writeContainerConfig(opts CreateOptions, resources *resourceAllocation, ws *workspaceSetup, secretsPath string, permsMounts []generator.PermissionsMount, identity *config.AgentIdentity) (string, error) {
	proxyURL := ""
	if resources.template.UseProxy && c.hostConfig.ProxyURL != "" {
		proxyURL = c.hostConfig.ProxyURL
		logging.Debug("using API proxy", "url", proxyURL)
	}

	containerCfg := &generator.ContainerConfig{
		Name:              opts.Name,
		NetworkSlot:       resources.networkSlot,
		Workspace:         ws.effectivePath,
		SecretsPath:       secretsPath,
		AuthorizedKeys:    c.resolveSSHKeys(opts),
		Template:          resources.template,
		HostConfig:        c.hostConfig,
		WorkspaceMode:     string(ws.mode),
		SourceRepo:        ws.sourceRepo,
		NixpkgsRev:        c.hostConfig.NixpkgsRev,
		ProxyURL:          proxyURL,
		UID:               c.hostConfig.UID,
		GID:               c.hostConfig.GID,
		NoTmuxConfig:      opts.NoTmuxConfig,
		PermissionsMounts: permsMounts,
		AgentIdentity:     identity,
	}

	nixConfig, err := generator.GenerateNixConfig(containerCfg)
	if err != nil {
		return "", fmt.Errorf("failed to generate container config: %w", err)
	}

	configPath := filepath.Join(c.paths.SandboxesDir, opts.Name+".nix")
	if err := os.MkdirAll(c.paths.SandboxesDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create sandboxes directory: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(nixConfig), 0644); err != nil {
		return "", fmt.Errorf("failed to write config: %w", err)
	}

	return configPath, nil
}

// startContainer creates and starts the container via the runtime.
func (c *Creator) startContainer(name, configPath string) error {
	logging.Debug("creating container via runtime", "name", name, "config", configPath)
	if err := runtime.Create(runtime.CreateOptions{
		Name:       name,
		ConfigPath: configPath,
		Start:      true,
	}); err != nil {
		return fmt.Errorf("container creation failed: %w", err)
	}
	return nil
}

// postCreationSetup performs post-creation setup (SSH wait, skills injection).
func (c *Creator) postCreationSetup(metadata *config.SandboxMetadata, template *config.Template, workspacePath string) {
	containerIP := metadata.ContainerIP()
	logging.Debug("waiting for SSH", "host", containerIP, "timeout", health.SSHReadyTimeoutSeconds)
	c.waitForSSH(containerIP, health.SSHReadyTimeoutSeconds)

	c.injectSkills(metadata.Name, workspacePath, metadata, template)
}

// workspaceSetup holds workspace setup results.
type workspaceSetup struct {
	effectivePath string
	sourceRepo    string
	gitBranch     string
	backend       workspace.Backend
	mode          WorkspaceMode
}

// setupWorkspace sets up the workspace based on the options.
func (c *Creator) setupWorkspace(opts CreateOptions) (*workspaceSetup, error) {
	ws := &workspaceSetup{}

	absPath, err := filepath.Abs(opts.RepoPath)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	if opts.Direct {
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("workspace does not exist: %s", absPath)
		}
		ws.effectivePath = absPath
		ws.mode = WorkspaceModeDirect
		return ws, nil
	}

	// Auto-detect VCS backend
	ws.backend = workspace.DetectBackend(absPath)
	if ws.backend == nil {
		return nil, fmt.Errorf("not a supported repository: %s\n  Use --direct for non-repo directories", absPath)
	}

	switch ws.backend.Name() {
	case "jj":
		ws.mode = WorkspaceModeJJ
	case "git-worktree":
		ws.mode = WorkspaceModeGitWorktree
	}

	if ws.backend.Exists(absPath, opts.Name) {
		return nil, fmt.Errorf("%s workspace %s already exists in repo", ws.backend.Name(), opts.Name)
	}

	ws.sourceRepo = absPath
	ws.effectivePath = filepath.Join(c.paths.WorkspacesDir, opts.Name)

	if gitBackend, ok := ws.backend.(*workspace.GitBackend); ok {
		ws.gitBranch = gitBackend.BranchName(opts.Name)
	}

	if err := os.MkdirAll(c.paths.WorkspacesDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspaces directory: %w", err)
	}

	logging.Debug("creating workspace", "backend", ws.backend.Name(), "repo", absPath, "name", opts.Name)
	if err := ws.backend.Create(absPath, opts.Name, ws.effectivePath); err != nil {
		return nil, fmt.Errorf("failed to create %s workspace: %w", ws.backend.Name(), err)
	}

	return ws, nil
}

// templateHasSecrets returns true if any agent in the template uses secrets.
func (c *Creator) templateHasSecrets(template *config.Template) bool {
	for _, agent := range template.Agents {
		if agent.SecretName != "" {
			return true
		}
	}
	return false
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

// waitForSSH waits for SSH to be ready on the given host.
func (c *Creator) waitForSSH(host string, timeoutSeconds int) bool {
	for i := 0; i < timeoutSeconds; i++ {
		if health.CheckSSH(host) {
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
	if err := copySkillsToContainer(metadata.ContainerIP(), skillsContent); err != nil {
		logging.Warn("failed to inject skills", "error", err)
	}
}

// generatePermissionsFiles generates agent permissions settings files on the host
// and returns the mounts needed to bind them into the container.
func (c *Creator) generatePermissionsFiles(sandboxName string, template *config.Template) ([]generator.PermissionsMount, error) {
	var mounts []generator.PermissionsMount

	for agentName, agent := range template.Agents {
		if agent.Permissions == nil {
			continue
		}

		policy, ok := generator.GetPermissionsPolicy(agentName)
		if !ok {
			logging.Debug("no permissions policy for agent, skipping", "agent", agentName)
			continue
		}

		content, err := policy.GenerateSettings(agent.Permissions)
		if err != nil {
			return nil, fmt.Errorf("agent %s: %w", agentName, err)
		}
		if content == nil {
			continue
		}

		hostPath := filepath.Join(c.paths.SandboxesDir, fmt.Sprintf("%s.%s-permissions.json", sandboxName, agentName))
		if err := os.MkdirAll(c.paths.SandboxesDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create sandboxes directory: %w", err)
		}
		if err := os.WriteFile(hostPath, content, 0644); err != nil {
			return nil, fmt.Errorf("failed to write permissions file for %s: %w", agentName, err)
		}
		logging.Debug("wrote permissions file", "agent", agentName, "path", hostPath)

		mounts = append(mounts, generator.PermissionsMount{
			HostPath:      hostPath,
			ContainerPath: policy.ContainerSettingsPath(),
		})
	}

	return mounts, nil
}

// cleanup removes resources created during a failed sandbox creation.
func (c *Creator) cleanup(metadata *config.SandboxMetadata) {
	logging.Debug("cleaning up failed sandbox creation", "name", metadata.Name)

	// Use unified cleanup function with all options enabled
	Cleanup(metadata, c.paths, DefaultCleanupOptions(), c.rt)
}

// copySkillsToContainer copies skills content into the container.
// Uses stdin piping to safely transfer content without shell injection risks.
// The content is passed via stdin to avoid any shell interpolation or heredoc escaping issues.
func copySkillsToContainer(host string, content string) error {
	// Pass content via stdin to sh, which writes it to the file.
	// This is safe because content never appears in the command string.
	return ssh.ExecWithStdin(host, content, "sh", "-c", "cat > /workspace/CLAUDE.md")
}

// resolveIdentity merges host-level defaults with per-sandbox overrides from opts.
// Returns nil if all fields are empty (no identity configured).
func (c *Creator) resolveIdentity(opts CreateOptions) *config.AgentIdentity {
	var gitUser, gitEmail, sshKeyPath string

	// Start with host-level defaults
	if c.hostConfig.AgentIdentity != nil {
		gitUser = c.hostConfig.AgentIdentity.GitUser
		gitEmail = c.hostConfig.AgentIdentity.GitEmail
		sshKeyPath = c.hostConfig.AgentIdentity.SSHKeyPath
	}

	// Override with per-sandbox values
	if opts.GitUser != "" {
		gitUser = opts.GitUser
	}
	if opts.GitEmail != "" {
		gitEmail = opts.GitEmail
	}
	if opts.SSHKeyPath != "" {
		sshKeyPath = opts.SSHKeyPath
	}

	// Return nil if nothing is set
	if gitUser == "" && gitEmail == "" && sshKeyPath == "" {
		return nil
	}

	return &config.AgentIdentity{
		GitUser:    gitUser,
		GitEmail:   gitEmail,
		SSHKeyPath: sshKeyPath,
	}
}

// resolveSSHKeys returns SSH keys to use, in order of priority:
// 1. Explicit keys from CreateOptions
// 2. Keys from host config
// 3. Keys from ~/.ssh/*.pub
func (c *Creator) resolveSSHKeys(opts CreateOptions) []string {
	// 1. Explicit keys from options (highest priority)
	if len(opts.SSHKeys) > 0 {
		logging.Debug("using explicit SSH keys", "count", len(opts.SSHKeys))
		return opts.SSHKeys
	}

	// 2. Keys from host config
	if len(c.hostConfig.AuthorizedKeys) > 0 {
		logging.Debug("using SSH keys from config", "count", len(c.hostConfig.AuthorizedKeys))
		return c.hostConfig.AuthorizedKeys
	}

	// 3. Auto-detect from ~/.ssh/*.pub
	keys := readSSHPublicKeys()
	if len(keys) > 0 {
		logging.Debug("using SSH keys from ~/.ssh", "count", len(keys))
		return keys
	}

	logging.Warn("no SSH keys found")
	return nil
}

// readSSHPublicKeys reads all SSH public keys from ~/.ssh/*.pub
func readSSHPublicKeys() []string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logging.Debug("failed to get home directory", "error", err)
		return nil
	}

	sshDir := filepath.Join(homeDir, ".ssh")
	entries, err := os.ReadDir(sshDir)
	if err != nil {
		logging.Debug("failed to read ~/.ssh directory", "error", err)
		return nil
	}

	var keys []string
	for _, entry := range entries {
		if entry.IsDir() || !isPubKeyFile(entry.Name()) {
			continue
		}

		keyPath := filepath.Join(sshDir, entry.Name())
		content, err := os.ReadFile(keyPath)
		if err != nil {
			logging.Debug("failed to read key file", "path", keyPath, "error", err)
			continue
		}

		// Trim whitespace and skip empty files
		key := string(content)
		key = trimKey(key)
		if key != "" && isValidSSHKey(key) {
			keys = append(keys, key)
			logging.Debug("found SSH key", "file", entry.Name())
		}
	}

	return keys
}

// isPubKeyFile returns true if the filename looks like a public key file
func isPubKeyFile(name string) bool {
	return filepath.Ext(name) == ".pub"
}

// trimKey removes leading/trailing whitespace and trailing newlines
func trimKey(key string) string {
	// Remove trailing newlines and whitespace
	for len(key) > 0 && (key[len(key)-1] == '\n' || key[len(key)-1] == '\r' || key[len(key)-1] == ' ') {
		key = key[:len(key)-1]
	}
	// Remove leading whitespace
	for len(key) > 0 && (key[0] == ' ' || key[0] == '\t') {
		key = key[1:]
	}
	return key
}

// isValidSSHKey checks if a string looks like a valid SSH public key
func isValidSSHKey(key string) bool {
	// Valid SSH keys start with a key type
	validPrefixes := []string{
		"ssh-rsa ",
		"ssh-ed25519 ",
		"ssh-dss ",
		"ecdsa-sha2-nistp256 ",
		"ecdsa-sha2-nistp384 ",
		"ecdsa-sha2-nistp521 ",
		"sk-ssh-ed25519@openssh.com ",
		"sk-ecdsa-sha2-nistp256@openssh.com ",
	}

	for _, prefix := range validPrefixes {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

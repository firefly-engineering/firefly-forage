package sandbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/audit"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/generator"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/health"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/injection"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/multiplexer"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/port"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
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

	rt, err := runtime.New(&runtime.Config{
		Type:               runtime.RuntimeAuto,
		ContainerPrefix:    config.ContainerPrefix,
		ExtraContainerPath: hostConfig.ExtraContainerPath,
		NixpkgsPath:        hostConfig.NixpkgsPath,
		SandboxesDir:       paths.SandboxesDir,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize runtime: %w", err)
	}

	return &Creator{
		paths:      paths,
		hostConfig: hostConfig,
		rt:         rt,
	}, nil
}

// Create creates a new sandbox with the given options.
// File locking is used to prevent TOCTOU races during slot allocation.
func (c *Creator) Create(ctx context.Context, opts CreateOptions) (*CreateResult, error) {
	logging.Debug("starting sandbox creation", "name", opts.Name, "template", opts.Template)

	// Phase 1: Validate inputs
	if err := c.validateInputs(opts); err != nil {
		return nil, err
	}

	// Acquire an exclusive lock on the sandboxes directory to prevent
	// concurrent slot allocation races (TOCTOU in AllocateSlot).
	unlock, err := acquireSandboxLock(c.paths.SandboxesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire sandbox lock: %w", err)
	}
	defer unlock()

	// Check runtime capabilities and warn about unsupported features
	warnings := c.checkCapabilities()

	// Phase 2: Load resources and allocate ports
	resources, err := c.loadResources(opts)
	if err != nil {
		return nil, err
	}

	// Resolve and validate agent identity (after template load for template-level identity)
	identity := c.resolveIdentity(opts, resources.template)
	if err = config.ValidateAgentIdentity(identity); err != nil {
		return nil, fmt.Errorf("invalid agent identity: %w", err)
	}

	// Phase 3: Set up workspace
	var ws *workspaceSetup
	if len(resources.template.WorkspaceMounts) > 0 {
		ws, err = c.setupWorkspaceMounts(opts, resources.template)
	} else {
		if opts.RepoPath == "" {
			return nil, fmt.Errorf("--repo is required (template has no workspace mounts configured)")
		}
		ws, err = c.setupWorkspace(opts)
	}
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

	// Phase 6: Generate and write container config using contribution system
	configPath, err := c.writeContainerConfig(ctx, opts, resources, ws, secretsPath, identity, metadata)
	if err != nil {
		cleanup()
		return nil, err
	}

	// Phase 7: Save metadata (before container creation so runtime can resolve container name)
	if err := config.SaveSandboxMetadata(c.paths.SandboxesDir, metadata); err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to save metadata: %w", err)
	}

	// Phase 8: Create and start container
	if err := c.startContainer(opts.Name, configPath); err != nil {
		cleanup()
		return nil, err
	}

	// Phase 9: Post-creation setup (wait for SSH)
	c.postCreationSetup(metadata)

	// Phase 10: Run init commands
	initResult := c.runInitCommands(ctx, metadata, resources.template)

	// Log creation event
	auditLogger := audit.NewLogger(c.paths.StateDir)
	_ = auditLogger.LogEvent(audit.EventCreate, opts.Name, "template="+opts.Template)

	return &CreateResult{
		Name:               opts.Name,
		ContainerIP:        metadata.ContainerIP(),
		Workspace:          ws.effectivePath,
		Metadata:           metadata,
		CapabilityWarnings: warnings,
		InitResult:         initResult,
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
	meta := &config.SandboxMetadata{
		Name:          opts.Name,
		Template:      opts.Template,
		NetworkSlot:   resources.networkSlot,
		CreatedAt:     time.Now().Format(time.RFC3339),
		AgentIdentity: identity,
		Multiplexer:   resources.template.Multiplexer,
		ContainerName: config.ContainerNameForSlot(resources.networkSlot),
		Runtime:       c.rt.Name(),
	}

	if len(ws.mounts) > 0 {
		// Multi-mount path
		meta.WorkspaceMounts = ws.mounts
		// Set legacy fields from first mount for backward compat
		if len(ws.mounts) > 0 {
			first := ws.mounts[0]
			meta.Workspace = first.HostPath
			meta.WorkspaceMode = first.Mode
			meta.SourceRepo = first.SourceRepo
			meta.GitBranch = first.GitBranch
		}
	} else {
		// Legacy single-mount path
		meta.Workspace = ws.effectivePath
		meta.WorkspaceMode = string(ws.mode)
		meta.SourceRepo = ws.sourceRepo
		meta.JJWorkspaceName = opts.Name
		meta.GitBranch = ws.gitBranch
	}

	return meta
}

// writeContainerConfig generates and writes the Nix container configuration using the contribution system.
func (c *Creator) writeContainerConfig(ctx context.Context, opts CreateOptions, resources *resourceAllocation, ws *workspaceSetup, secretsPath string, identity *config.AgentIdentity, metadata *config.SandboxMetadata) (string, error) {
	// Determine proxy URL
	proxyURL := ""
	if resources.template.UseProxy && c.hostConfig.ProxyURL != "" {
		proxyURL = c.hostConfig.ProxyURL
		logging.Debug("using API proxy", "url", proxyURL)
	}

	// Create multiplexer instance
	mux := multiplexer.New(multiplexer.Type(resources.template.Multiplexer))

	// Build contribution sources from all backends
	contribParams := ContributionSourcesParams{
		Runtime:       c.rt,
		Template:      resources.template,
		Metadata:      metadata,
		WsBackend:     ws.backend,
		Mux:           mux,
		Identity:      identity,
		WorkspacePath: ws.effectivePath,
		SourceRepo:    ws.sourceRepo,
		SecretsPath:   secretsPath,
		ProxyURL:      proxyURL,
		SandboxName:   opts.Name,
		HostConfig:    c.hostConfig,
	}
	if len(ws.mounts) > 0 {
		contribParams.WorkspaceMounts = ws.mounts
		contribParams.MountBackends = ws.backends
	}
	contribResult := buildContributionSources(contribParams)

	// Collect contributions from all sources
	collector := injection.NewCollector()
	contributions, err := collector.Collect(ctx, contribResult.Sources)
	if err != nil {
		return "", fmt.Errorf("failed to collect contributions: %w", err)
	}

	// Pass resource limits if runtime supports them
	var resourceLimits *config.ResourceLimits
	caps := runtime.GetCapabilities(c.rt)
	if caps.ResourceLimits && resources.template.ResourceLimits != nil {
		resourceLimits = resources.template.ResourceLimits
	} else if resources.template.ResourceLimits != nil && !resources.template.ResourceLimits.IsEmpty() {
		logging.Warn("runtime does not support resource limits; ignoring resource limit configuration")
	}

	containerCfg := &generator.ContainerConfig{
		Name:            opts.Name,
		NetworkSlot:     resources.networkSlot,
		AuthorizedKeys:  c.resolveSSHKeys(opts),
		Template:        resources.template,
		UID:             c.hostConfig.UID,
		GID:             c.hostConfig.GID,
		Mux:             mux,
		AgentIdentity:   identity,
		Runtime:         c.rt.Name(),
		Username:        c.hostConfig.ResolvedContainerUsername(),
		WorkspaceDir:    c.hostConfig.ResolvedWorkspacePath(),
		StateVersion:    c.hostConfig.ResolvedStateVersion(),
		ResourceLimits:  resourceLimits,
		Contributions:   contributions,
		Reproducibility: contribResult.Reproducibility,
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
	if err := c.rt.Create(context.Background(), runtime.CreateOptions{
		Name:       name,
		ConfigPath: configPath,
		Start:      true,
	}); err != nil {
		return fmt.Errorf("container creation failed: %w", err)
	}
	return nil
}

// runInitCommands executes template-level init commands and per-project .forage/init
// inside the container. Failures are logged as warnings and do not block creation.
func (c *Creator) runInitCommands(ctx context.Context, metadata *config.SandboxMetadata, template *config.Template) *InitCommandResult {
	containerName := metadata.ResolvedContainerName()
	username := c.hostConfig.ResolvedContainerUsername()
	workspacePath := c.hostConfig.ResolvedWorkspacePath()
	execOpts := runtime.ExecOptions{
		User:       username,
		WorkingDir: workspacePath,
	}

	result := &InitCommandResult{}

	// Run template init commands
	for _, cmd := range template.InitCommands {
		result.TemplateCommandsRun++
		logging.Debug("running init command", "command", cmd, "container", containerName)

		execResult, err := c.rt.Exec(ctx, containerName, []string{"sh", "-c", cmd}, execOpts)
		if err != nil {
			warning := fmt.Sprintf("init command %q: %v", cmd, err)
			logging.Warn(warning)
			result.TemplateWarnings = append(result.TemplateWarnings, warning)
			continue
		}
		if execResult.ExitCode != 0 {
			warning := fmt.Sprintf("init command %q exited with code %d", cmd, execResult.ExitCode)
			if execResult.Stderr != "" {
				warning += ": " + execResult.Stderr
			}
			logging.Warn(warning)
			result.TemplateWarnings = append(result.TemplateWarnings, warning)
		}
	}

	// Check for per-project .forage/init script
	initScriptPath := filepath.Join(workspacePath, ".forage", "init")
	checkResult, err := c.rt.Exec(ctx, containerName, []string{"test", "-f", initScriptPath}, execOpts)
	if err != nil || checkResult.ExitCode != 0 {
		// No .forage/init script found, that's fine
		return result
	}

	// Run the per-project init script
	logging.Debug("running .forage/init script", "container", containerName)
	result.ProjectInitRun = true
	execResult, err := c.rt.Exec(ctx, containerName, []string{"sh", initScriptPath}, execOpts)
	if err != nil {
		result.ProjectInitWarning = fmt.Sprintf(".forage/init: %v", err)
		logging.Warn(result.ProjectInitWarning)
	} else if execResult.ExitCode != 0 {
		result.ProjectInitWarning = fmt.Sprintf(".forage/init exited with code %d", execResult.ExitCode)
		if execResult.Stderr != "" {
			result.ProjectInitWarning += ": " + execResult.Stderr
		}
		logging.Warn(result.ProjectInitWarning)
	}

	return result
}

// postCreationSetup performs post-creation setup (SSH wait).
func (c *Creator) postCreationSetup(metadata *config.SandboxMetadata) {
	containerIP := metadata.ContainerIP()
	logging.Debug("waiting for SSH", "host", containerIP, "timeout", health.SSHReadyTimeoutSeconds)
	c.waitForSSH(containerIP, health.SSHReadyTimeoutSeconds)
}

// workspaceSetup holds workspace setup results.
type workspaceSetup struct {
	effectivePath string
	sourceRepo    string
	gitBranch     string
	backend       workspace.Backend
	mode          WorkspaceMode

	// Multi-mount results (when template has WorkspaceMounts)
	mounts   []config.WorkspaceMountMeta
	backends map[string]workspace.Backend // mount name -> backend
}

// setupWorkspace sets up the workspace based on the options (legacy single-mount path).
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

// resolveRepoPath resolves a mount's repo reference to an absolute path.
// Empty/null repo uses the default --repo, a name looks up in named repos,
// an absolute path is used as-is.
func resolveRepoPath(repoRef string, opts CreateOptions) (string, error) {
	if repoRef == "" {
		// Uses default --repo
		if opts.RepoPath == "" {
			return "", fmt.Errorf("mount requires --repo but none provided")
		}
		return filepath.Abs(opts.RepoPath)
	}
	if filepath.IsAbs(repoRef) {
		return repoRef, nil
	}
	// Named repo lookup
	if path, ok := opts.Repos[repoRef]; ok {
		return filepath.Abs(path)
	}
	return "", fmt.Errorf("named repo %q not provided via --repo", repoRef)
}

// validateMountSpecs checks mount specs for conflicts before creation.
func validateMountSpecs(mounts map[string]*config.WorkspaceMount) error {
	seen := make(map[string]string) // containerPath -> mount name
	for name, m := range mounts {
		if m.ContainerPath == "" {
			return fmt.Errorf("mount %q: containerPath is required", name)
		}
		if prev, ok := seen[m.ContainerPath]; ok {
			return fmt.Errorf("mounts %q and %q both claim container path %s", prev, name, m.ContainerPath)
		}
		seen[m.ContainerPath] = name
		if m.HostPath == "" && m.Repo == "" {
			// Repo-backed mount using default --repo (valid if --repo is provided)
		}
		if m.HostPath != "" && m.Repo != "" {
			return fmt.Errorf("mount %q: cannot set both hostPath and repo", name)
		}
	}
	return nil
}

// setupWorkspaceMounts sets up multiple workspace mounts from template specs.
func (c *Creator) setupWorkspaceMounts(opts CreateOptions, template *config.Template) (*workspaceSetup, error) {
	ws := &workspaceSetup{
		backends: make(map[string]workspace.Backend),
	}

	if err := validateMountSpecs(template.WorkspaceMounts); err != nil {
		return nil, fmt.Errorf("invalid mount configuration: %w", err)
	}

	// Managed workspace base dir for this sandbox: workspaces/<sandbox>/<mount-name>/
	sandboxWsDir := filepath.Join(c.paths.WorkspacesDir, opts.Name)

	// Track created workspaces for rollback on failure
	var created []config.WorkspaceMountMeta

	rollback := func() {
		for _, m := range created {
			if m.SourceRepo != "" {
				if backend := workspace.BackendForMode(m.Mode); backend != nil {
					_ = backend.Remove(m.SourceRepo, m.Name, m.HostPath)
				}
			}
		}
		_ = os.RemoveAll(sandboxWsDir)
	}

	for name, spec := range template.WorkspaceMounts {
		meta := config.WorkspaceMountMeta{
			Name:          name,
			ContainerPath: spec.ContainerPath,
			ReadOnly:      spec.ReadOnly,
		}

		if spec.HostPath != "" {
			// Literal bind mount — validate host path exists
			absPath, err := filepath.Abs(spec.HostPath)
			if err != nil {
				rollback()
				return nil, fmt.Errorf("mount %q: invalid hostPath: %w", name, err)
			}
			if _, err := os.Stat(absPath); os.IsNotExist(err) {
				rollback()
				return nil, fmt.Errorf("mount %q: hostPath does not exist: %s", name, absPath)
			}
			meta.HostPath = absPath
			meta.Mode = "direct"
		} else {
			// Repo-backed mount
			repoPath, err := resolveRepoPath(spec.Repo, opts)
			if err != nil {
				rollback()
				return nil, fmt.Errorf("mount %q: %w", name, err)
			}

			if _, err := os.Stat(repoPath); os.IsNotExist(err) {
				rollback()
				return nil, fmt.Errorf("mount %q: repo path does not exist: %s", name, repoPath)
			}

			meta.SourceRepo = repoPath

			// Determine mode (auto-detect or explicit)
			var backend workspace.Backend
			if spec.Mode != "" && spec.Mode != "direct" {
				backend = workspace.BackendForMode(spec.Mode)
				if backend == nil {
					rollback()
					return nil, fmt.Errorf("mount %q: unsupported mode %q", name, spec.Mode)
				}
			} else if spec.Mode != "direct" {
				backend = workspace.DetectBackend(repoPath)
			}

			if backend == nil || spec.Mode == "direct" {
				// Direct mount — use repo path directly
				meta.HostPath = repoPath
				meta.Mode = "direct"
			} else {
				// VCS workspace — create isolated workspace
				meta.Mode = backend.Name()

				// Use a unique workspace name combining sandbox name and mount name
				wsName := opts.Name + "-" + name

				if backend.Exists(repoPath, wsName) {
					rollback()
					return nil, fmt.Errorf("mount %q: %s workspace %s already exists in repo", name, backend.Name(), wsName)
				}

				wsPath := filepath.Join(sandboxWsDir, name)
				if err := os.MkdirAll(sandboxWsDir, 0755); err != nil {
					rollback()
					return nil, fmt.Errorf("mount %q: failed to create workspace directory: %w", name, err)
				}

				logging.Debug("creating workspace mount", "name", name, "backend", backend.Name(), "repo", repoPath, "wsName", wsName)
				if err := backend.Create(repoPath, wsName, wsPath); err != nil {
					rollback()
					return nil, fmt.Errorf("mount %q: failed to create %s workspace: %w", name, backend.Name(), err)
				}

				meta.HostPath = wsPath

				if gitBackend, ok := backend.(*workspace.GitBackend); ok {
					meta.GitBranch = gitBackend.BranchName(wsName)
				}

				ws.backends[name] = backend
			}

			meta.Branch = spec.Branch
		}

		created = append(created, meta)
	}

	ws.mounts = created

	// Set effectivePath to the first mount's container path for backward compat in the result
	if len(created) > 0 {
		ws.effectivePath = created[0].HostPath
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

// setupSecrets reads secrets from host file paths and writes them to the sandbox secrets directory.
func (c *Creator) setupSecrets(secretsPath string, template *config.Template) error {
	if err := os.MkdirAll(secretsPath, 0700); err != nil {
		return err
	}

	for _, agent := range template.Agents {
		if agent.SecretName == "" {
			continue
		}

		secretSourcePath, ok := c.hostConfig.Secrets[agent.SecretName]
		if !ok {
			logging.Debug("secret not found in host config", "secret", agent.SecretName)
			continue
		}

		secretData, err := os.ReadFile(secretSourcePath)
		if err != nil {
			return fmt.Errorf("failed to read secret %s from %s: %w", agent.SecretName, secretSourcePath, err)
		}

		secretFile := filepath.Join(secretsPath, agent.SecretName)
		if err := os.WriteFile(secretFile, secretData, 0600); err != nil {
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

// cleanup removes resources created during a failed sandbox creation.
func (c *Creator) cleanup(metadata *config.SandboxMetadata) {
	logging.Debug("cleaning up failed sandbox creation", "name", metadata.Name)

	// Use unified cleanup function with all options enabled
	Cleanup(metadata, c.paths, DefaultCleanupOptions(), c.rt)
}

// resolveIdentity merges identity from four levels (lowest to highest priority):
//  1. Host user's ~/.gitconfig (fallback for name/email only)
//  2. HostConfig.AgentIdentity (host-level defaults)
//  3. Template.AgentIdentity (template-level defaults)
//  4. Per-sandbox CreateOptions (explicit overrides)
//
// Returns nil if all fields are empty (no identity configured).
func (c *Creator) resolveIdentity(opts CreateOptions, template *config.Template) *config.AgentIdentity {
	var gitUser, gitEmail, sshKeyPath string

	// 1. Host user gitconfig (lowest priority fallback, name/email only)
	if hostGit := config.ReadHostUserGitIdentity(c.hostConfig.User, opts.RepoPath); hostGit != nil {
		gitUser = hostGit.GitUser
		gitEmail = hostGit.GitEmail
	}

	// 2. Host-level defaults
	if c.hostConfig.AgentIdentity != nil {
		if c.hostConfig.AgentIdentity.GitUser != "" {
			gitUser = c.hostConfig.AgentIdentity.GitUser
		}
		if c.hostConfig.AgentIdentity.GitEmail != "" {
			gitEmail = c.hostConfig.AgentIdentity.GitEmail
		}
		if c.hostConfig.AgentIdentity.SSHKeyPath != "" {
			sshKeyPath = c.hostConfig.AgentIdentity.SSHKeyPath
		}
	}

	// 3. Template-level defaults
	if template != nil && template.AgentIdentity != nil {
		if template.AgentIdentity.GitUser != "" {
			gitUser = template.AgentIdentity.GitUser
		}
		if template.AgentIdentity.GitEmail != "" {
			gitEmail = template.AgentIdentity.GitEmail
		}
		if template.AgentIdentity.SSHKeyPath != "" {
			sshKeyPath = template.AgentIdentity.SSHKeyPath
		}
	}

	// 4. Per-sandbox overrides (highest priority)
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
			logging.Debug("failed to read key file", "file", entry.Name(), "error", err)
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

// acquireSandboxLock acquires an exclusive file lock on the sandboxes directory
// to prevent concurrent operations from racing on slot allocation or metadata writes.
// Returns an unlock function that must be called when the critical section is done.
func acquireSandboxLock(sandboxesDir string) (func(), error) {
	if err := os.MkdirAll(sandboxesDir, 0755); err != nil {
		return nil, err
	}

	lockPath := filepath.Join(sandboxesDir, ".lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}

	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

// checkCapabilities checks runtime capabilities against the sandbox configuration
// and returns warnings for unsupported features. It does not block creation.
func (c *Creator) checkCapabilities() []string {
	caps := runtime.GetCapabilities(c.rt)
	var warnings []string

	if !caps.NixOSConfig {
		warnings = append(warnings, "Runtime "+c.rt.Name()+" does not support NixOS config generation; container may have reduced functionality")
	}
	if !caps.NetworkIsolation {
		warnings = append(warnings, "Runtime "+c.rt.Name()+" does not support network isolation; network mode filtering will not be enforced")
	}
	if !caps.SSHAccess {
		warnings = append(warnings, "Runtime "+c.rt.Name()+" does not support SSH access; use exec instead")
	}
	if !caps.GeneratedFiles {
		warnings = append(warnings, "Runtime "+c.rt.Name()+" does not support generated file mounting; skills and permissions may not be available")
	}

	for _, w := range warnings {
		logging.Warn(w)
	}

	return warnings
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

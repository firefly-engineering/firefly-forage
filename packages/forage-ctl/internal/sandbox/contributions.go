package sandbox

import (
	"context"
	"os/user"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/agent"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/injection"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/multiplexer"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/reproducibility"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/workspace"
)

// Ensure context is used
var _ = context.Background

// ContributionSourcesParams holds parameters for building contribution sources.
type ContributionSourcesParams struct {
	Runtime       runtime.Runtime
	Template      *config.Template
	Metadata      *config.SandboxMetadata // Needed for skills generation
	WsBackend     workspace.Backend
	Mux           multiplexer.Multiplexer
	Identity      *config.AgentIdentity
	WorkspacePath string
	SourceRepo    string
	SecretsPath   string
	ProxyURL      string
	SandboxName   string
	HostConfig    *config.HostConfig
}

// ContributionSourcesResult holds the result of building contribution sources.
type ContributionSourcesResult struct {
	Sources         injection.CollectionSources
	Reproducibility reproducibility.Reproducibility
}

// buildContributionSources builds the collection sources for the injection collector.
// This centralizes the construction of all the contributors that participate in
// container configuration.
func buildContributionSources(params ContributionSourcesParams) ContributionSourcesResult {
	rt := params.Runtime
	template := params.Template
	wsBackend := params.WsBackend
	mux := params.Mux
	identity := params.Identity
	workspacePath := params.WorkspacePath
	sourceRepo := params.SourceRepo
	secretsPath := params.SecretsPath
	proxyURL := params.ProxyURL
	sandboxName := params.SandboxName
	hostConfig := params.HostConfig
	// Get container info from runtime if available
	var containerInfo runtime.SandboxContainerInfo
	if gfr, ok := rt.(runtime.GeneratedFileRuntime); ok {
		containerInfo = gfr.ContainerInfo()
	} else {
		containerInfo = runtime.DefaultContainerInfo()
	}

	// Get host home directory
	hostHomeDir := ""
	if hostConfig != nil && hostConfig.User != "" {
		if u, err := user.Lookup(hostConfig.User); err == nil {
			hostHomeDir = u.HomeDir
		}
	}

	// Build the list of contributors
	var contributors []any

	// 1. Reproducibility (Nix store mount, base packages)
	repro := reproducibility.NewNixReproducibility()
	contributors = append(contributors, repro)

	// 2. Workspace mount contributor
	workspaceMount := injection.NewWorkspaceMountContributor(workspacePath, containerInfo.WorkspaceDir)
	contributors = append(contributors, workspaceMount)

	// 3. Secrets contributor (if secrets are configured)
	if secretsPath != "" {
		secrets := injection.NewSecretsContributor(secretsPath)
		contributors = append(contributors, secrets)
	}

	// 4. Workspace backend contributor (if available)
	if wsBackend != nil {
		contributors = append(contributors, wsBackend)
	}

	// 5. Multiplexer contributor
	if mux != nil {
		contributors = append(contributors, mux)
	}

	// 6. Identity contributor (if identity is configured)
	if identity != nil {
		identityContrib := injection.NewIdentityContributor(
			identity.GitUser,
			identity.GitEmail,
			identity.SSHKeyPath,
			containerInfo.HomeDir,
		)
		contributors = append(contributors, identityContrib)
	}

	// 7. Proxy contributor (if proxy is configured)
	if proxyURL != "" {
		proxy := injection.NewProxyContributor(proxyURL, sandboxName)
		contributors = append(contributors, proxy)
	}

	// 8. Base tmpfiles contributor
	baseTmpfiles := injection.NewBaseTmpfilesContributor(containerInfo.HomeDir, containerInfo.Username)
	contributors = append(contributors, baseTmpfiles)

	// 9. Agent contributors
	if gfr, ok := rt.(runtime.GeneratedFileRuntime); ok {
		for agentName, agentCfg := range template.Agents {
			cfg := &agent.Config{
				PackagePath:           agentCfg.PackagePath,
				AuthEnvVar:            agentCfg.AuthEnvVar,
				SecretName:            agentCfg.SecretName,
				HostConfigDir:         agentCfg.HostConfigDir,
				ContainerConfigDir:    agentCfg.ContainerConfigDir,
				HostConfigDirReadOnly: agentCfg.HostConfigDirReadOnly,
			}
			if agentCfg.Permissions != nil {
				cfg.Permissions = &agent.Permissions{
					SkipAll: agentCfg.Permissions.SkipAll,
					Allow:   agentCfg.Permissions.Allow,
					Deny:    agentCfg.Permissions.Deny,
				}
			}
			if a := agent.NewAgent(agentName, cfg, gfr); a != nil {
				contributors = append(contributors, a)

				// Add Claude-specific tmpfiles if this is a Claude agent
				if agentName == "claude" {
					claudeTmpfiles := injection.NewClaudeTmpfilesContributor(containerInfo.HomeDir, containerInfo.Username)
					contributors = append(contributors, claudeTmpfiles)
				}
			}
		}
	}

	// 10. Skills contributor (generates system prompt and skill files)
	if params.Metadata != nil && template != nil {
		skillsContrib := NewSkillsContributor(containerInfo.HomeDir, template, params.Metadata)
		contributors = append(contributors, skillsContrib)
	}

	// Build request contexts
	mountReq := &injection.MountRequest{
		WorkspacePath: workspacePath,
		SourceRepo:    sourceRepo,
		HostHomeDir:   hostHomeDir,
	}

	envVarReq := &injection.EnvVarRequest{
		SandboxName: sandboxName,
		SecretsPath: secretsPath,
		ProxyURL:    proxyURL,
	}

	var initCmdReq *injection.InitCommandRequest
	if identity != nil {
		initCmdReq = &injection.InitCommandRequest{
			GitUser:    identity.GitUser,
			GitEmail:   identity.GitEmail,
			SSHKeyPath: identity.SSHKeyPath,
		}
	}

	genFileReq := &injection.GeneratedFileRequest{
		SandboxName:   sandboxName,
		SourceRepo:    sourceRepo,
		WorkspacePath: workspacePath,
		Template:      template.Name,
	}

	tmpfilesReq := &injection.TmpfilesRequest{
		HomeDir:  containerInfo.HomeDir,
		Username: containerInfo.Username,
	}

	// Build the generated file mounter if runtime supports it
	var gfMounter interface {
		MountGeneratedFile(ctx context.Context, sandboxName string, file injection.GeneratedFile) (injection.Mount, error)
	}
	if gfr, ok := rt.(runtime.GeneratedFileRuntime); ok {
		gfMounter = gfr
	}

	return ContributionSourcesResult{
		Sources: injection.CollectionSources{
			Contributors:         contributors,
			MountRequest:         mountReq,
			EnvVarRequest:        envVarReq,
			InitCommandRequest:   initCmdReq,
			GeneratedFileRequest: genFileReq,
			TmpfilesRequest:      tmpfilesReq,
			GeneratedFileMounter: gfMounter,
			SandboxName:          sandboxName,
		},
		Reproducibility: repro,
	}
}

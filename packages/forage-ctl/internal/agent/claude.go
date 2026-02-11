package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/injection"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
)

// ClaudeAgent implements Agent for Claude Code.
type ClaudeAgent struct {
	config  *Config
	runtime runtime.GeneratedFileRuntime
}

// NewClaudeAgent creates a new ClaudeAgent.
func NewClaudeAgent(cfg *Config, rt runtime.GeneratedFileRuntime) *ClaudeAgent {
	return &ClaudeAgent{
		config:  cfg,
		runtime: rt,
	}
}

// Name returns the agent identifier.
func (a *ClaudeAgent) Name() string {
	return "claude"
}

// ContributePackages returns the Claude Code package.
func (a *ClaudeAgent) ContributePackages(ctx context.Context) ([]injection.Package, error) {
	if a.config.PackagePath == "" {
		return nil, nil
	}
	// Return the package path as-is - it's a Nix flake reference
	return []injection.Package{{Name: a.config.PackagePath}}, nil
}

// ContributeMounts returns mounts for existing Claude config files.
func (a *ClaudeAgent) ContributeMounts(ctx context.Context, req *injection.MountRequest) ([]injection.Mount, error) {
	var mounts []injection.Mount

	// Mount .claude project directory from source repo
	if req.SourceRepo != "" {
		claudeDir := filepath.Join(req.SourceRepo, ".claude")
		if info, err := os.Stat(claudeDir); err == nil && info.IsDir() {
			mounts = append(mounts, injection.Mount{
				HostPath:      claudeDir,
				ContainerPath: filepath.Join("/workspace", ".claude"),
				ReadOnly:      false,
			})
		}
	}

	// Mount .claude.json from host home directory
	if req.HostHomeDir != "" {
		claudeJson := filepath.Join(req.HostHomeDir, ".claude.json")
		if _, err := os.Stat(claudeJson); err == nil {
			info := a.runtime.ContainerInfo()
			mounts = append(mounts, injection.Mount{
				HostPath:      claudeJson,
				ContainerPath: filepath.Join(info.HomeDir, ".claude.json"),
				ReadOnly:      false,
			})
		}
	}

	// Mount host config directory if specified
	if a.config.HostConfigDir != "" && a.config.ContainerConfigDir != "" {
		mounts = append(mounts, injection.Mount{
			HostPath:      a.config.HostConfigDir,
			ContainerPath: a.config.ContainerConfigDir,
			ReadOnly:      a.config.HostConfigDirReadOnly,
		})
	}

	return mounts, nil
}

// ContributeEnvVars returns environment variables for Claude authentication.
func (a *ClaudeAgent) ContributeEnvVars(ctx context.Context, req *injection.EnvVarRequest) ([]injection.EnvVar, error) {
	// If using proxy, the proxy contributor handles env vars
	if req.ProxyURL != "" {
		return nil, nil
	}

	// If using secrets, set up the auth env var
	if a.config.AuthEnvVar != "" && a.config.SecretName != "" && req.SecretsPath != "" {
		return []injection.EnvVar{{
			Name:  a.config.AuthEnvVar,
			Value: fmt.Sprintf(`"$(cat /run/secrets/%s 2>/dev/null || echo '')"`, a.config.SecretName),
		}}, nil
	}

	return nil, nil
}

// ContributeGeneratedFiles returns generated files for Claude configuration.
func (a *ClaudeAgent) ContributeGeneratedFiles(ctx context.Context, req *injection.GeneratedFileRequest) ([]injection.GeneratedFile, error) {
	var files []injection.GeneratedFile
	info := a.runtime.ContainerInfo()
	claudeDir := filepath.Join(info.HomeDir, ".claude")

	// Generate permissions policy
	if a.config.Permissions != nil {
		permContent, err := a.generatePermissions()
		if err != nil {
			return nil, fmt.Errorf("failed to generate permissions: %w", err)
		}
		if permContent != nil {
			files = append(files, injection.GeneratedFile{
				ContainerPath: "/etc/claude-code/managed-settings.json",
				Content:       permContent,
				Mode:          0644,
				ReadOnly:      true,
			})
		}
	}

	// Skills and system prompt generation are handled by SkillsContributor,
	// which wraps the skills package and uses project analysis context.

	// Ensure .claude directory exists (handled by ClaudeTmpfilesContributor)
	_ = claudeDir

	return files, nil
}

// generatePermissions creates the Claude Code managed-settings.json content.
func (a *ClaudeAgent) generatePermissions() ([]byte, error) {
	if a.config.Permissions == nil {
		return nil, nil
	}

	type permissionsBlock struct {
		Allow []string `json:"allow,omitempty"`
		Deny  []string `json:"deny,omitempty"`
	}

	type managedSettings struct {
		Permissions permissionsBlock `json:"permissions"`
	}

	var settings managedSettings

	if a.config.Permissions.SkipAll {
		settings.Permissions.Allow = claudeToolFamilies
	} else {
		if len(a.config.Permissions.Allow) == 0 && len(a.config.Permissions.Deny) == 0 {
			return nil, nil
		}
		settings.Permissions.Allow = a.config.Permissions.Allow
		settings.Permissions.Deny = a.config.Permissions.Deny
	}

	return json.Marshal(settings)
}

// claudeToolFamilies lists all Claude Code tool families for skipAll mode.
var claudeToolFamilies = []string{
	"Bash",
	"Edit",
	"Read",
	"Write",
	"WebFetch",
	"WebSearch",
	"Glob",
	"Grep",
	"NotebookEdit",
	"NotebookRead",
	"TodoRead",
	"TodoWrite",
}

// Ensure ClaudeAgent implements Agent
var _ Agent = (*ClaudeAgent)(nil)

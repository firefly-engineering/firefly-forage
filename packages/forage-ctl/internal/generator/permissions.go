package generator

import (
	"encoding/json"
	"fmt"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
)

// PermissionsPolicy defines how agent permissions are materialized into
// settings files for bind-mounting into containers.
type PermissionsPolicy interface {
	// ContainerSettingsPath is the absolute path inside the container
	// where the settings file should be mounted.
	ContainerSettingsPath() string
	// GenerateSettings produces the settings file content.
	// Returns nil when no permissions are configured (skip mount).
	GenerateSettings(permissions *config.AgentPermissions) ([]byte, error)
}

// agentPermissionsPolicy maps agent names to their permissions policy.
var agentPermissionsPolicy = map[string]PermissionsPolicy{
	"claude": &claudePermissionsPolicy{},
}

// GetPermissionsPolicy returns the PermissionsPolicy for the given agent name.
func GetPermissionsPolicy(agentName string) (PermissionsPolicy, bool) {
	p, ok := agentPermissionsPolicy[agentName]
	return p, ok
}

// claudePermissionsPolicy implements PermissionsPolicy for Claude Code.
// It writes a managed-settings.json file (highest precedence scope).
type claudePermissionsPolicy struct{}

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

func (p *claudePermissionsPolicy) ContainerSettingsPath() string {
	return "/etc/claude-code/managed-settings.json"
}

func (p *claudePermissionsPolicy) GenerateSettings(permissions *config.AgentPermissions) ([]byte, error) {
	if permissions == nil {
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

	if permissions.SkipAll {
		settings.Permissions.Allow = claudeToolFamilies
	} else {
		if len(permissions.Allow) == 0 && len(permissions.Deny) == 0 {
			return nil, nil
		}
		settings.Permissions.Allow = permissions.Allow
		settings.Permissions.Deny = permissions.Deny
	}

	data, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal claude settings: %w", err)
	}
	return data, nil
}

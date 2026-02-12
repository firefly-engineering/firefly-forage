// Package agent provides implementations for AI agents that can run in sandboxes.
// Each agent type (e.g., Claude) implements contribution interfaces to provide
// its specific mounts, packages, environment variables, and generated files.
package agent

import (
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/injection"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
)

// Agent represents an AI agent type (e.g., Claude) and its injection needs.
// Agents implement contribution interfaces for the resources they need.
type Agent interface {
	// Name returns the agent identifier (e.g., "claude").
	Name() string

	// Implements contribution interfaces:
	injection.MountContributor         // existing host files (.claude, .claude.json)
	injection.PackageContributor       // agent package
	injection.EnvVarContributor        // API keys
	injection.GeneratedFileContributor // permissions, skills, system prompt
}

// Config holds configuration for an agent from the template.
type Config struct {
	PackagePath           string
	AuthEnvVar            string
	SecretName            string
	HostConfigDir         string
	ContainerConfigDir    string
	HostConfigDirReadOnly bool
	Permissions           *Permissions
}

// Permissions defines what tool families an agent can use.
type Permissions struct {
	SkipAll bool     // allow all tool families
	Allow   []string // tool families to allow
	Deny    []string // tool families to deny
}

// NewAgent creates an Agent instance for the given agent name and configuration.
// Returns nil if the agent type is not supported.
func NewAgent(name string, cfg *Config, rt runtime.GeneratedFileRuntime) Agent {
	switch name {
	case "claude":
		return NewClaudeAgent(cfg, rt)
	default:
		return nil
	}
}

// ForTemplate returns Agent instances for all agents defined in a template.
func ForTemplate(agents map[string]*Config, rt runtime.GeneratedFileRuntime) []Agent {
	var result []Agent
	for name, cfg := range agents {
		if agent := NewAgent(name, cfg, rt); agent != nil {
			result = append(result, agent)
		}
	}
	return result
}

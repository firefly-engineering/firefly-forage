// Package injection provides types and interfaces for sandbox injection contributions.
// Backends (runtime, workspace, multiplexer, agent) implement contribution interfaces
// to provide mounts, packages, environment variables, and other injections.
package injection

import (
	"os"
)

// Mount represents a filesystem mount for a container.
type Mount struct {
	HostPath      string
	ContainerPath string
	ReadOnly      bool
}

// EnvVar represents an environment variable to set in the container.
// Value must be a complete Nix expression, typically a double-quoted string
// (e.g., `"1"` or `"$(cat /run/secrets/foo ...)"`) because it is inserted
// directly into the Nix template without additional escaping or quoting.
type EnvVar struct {
	Name  string
	Value string
}

// Package represents a package to install with optional version pinning.
type Package struct {
	Name    string
	Version string // optional, empty means latest/default
}

// PromptSection identifies a section of the agent system prompt.
type PromptSection int

const (
	PromptSectionEnvironment PromptSection = iota
	PromptSectionVCS
	PromptSectionIdentity
	PromptSectionAgent
)

// PromptFragment is text to add to agent prompts.
type PromptFragment struct {
	Section  PromptSection
	Priority int // Lower priority = earlier in section
	Content  string
}

// GeneratedFile represents a file that needs to be generated and mounted.
// The runtime handles the actual mechanism for making the content available
// in the container (e.g., writing to a temp dir that gets bind-mounted).
type GeneratedFile struct {
	ContainerPath string
	Content       []byte
	Mode          os.FileMode
	ReadOnly      bool
}

// MountRequest provides context for mount contributions.
type MountRequest struct {
	WorkspacePath string
	SourceRepo    string // empty for direct mode
	HostHomeDir   string
}

// EnvVarRequest provides context for env var contributions.
type EnvVarRequest struct {
	SandboxName string
	SecretsPath string
	ProxyURL    string
}

// GeneratedFileRequest provides context for generating files.
type GeneratedFileRequest struct {
	SandboxName   string
	SourceRepo    string
	WorkspacePath string
	Template      string // sandbox template name

	// Extended context for skills generation (optional)
	WorkspaceMode string            // "jj", "git-worktree", or "direct"
	GitBranch     string            // git branch name (for git-worktree mode)
	Network       string            // "full", "restricted", or "none"
	AllowedHosts  []string          // allowed hosts (for restricted network)
	UseProxy      bool              // whether proxy is used
	Multiplexer   string            // "tmux" or "wezterm"
	GitUser       string            // git user name (for identity)
	GitEmail      string            // git email (for identity)
	SSHKeyPath    string            // SSH key path (for identity)
	Agents        map[string]string // agent name -> auth label (e.g., "$ANTHROPIC_API_KEY" or "proxy")
}

// TmpfilesRequest provides context for tmpfiles rules.
type TmpfilesRequest struct {
	HomeDir  string
	Username string
}

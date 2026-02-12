// Package config provides configuration types and loading for forage-ctl.
//
// # Configuration Files
//
// The package handles three types of configuration:
//
//   - HostConfig: Host-level settings loaded from /etc/firefly-forage/config.json
//   - Template: Sandbox templates loaded from /etc/firefly-forage/templates/*.json
//   - SandboxMetadata: Runtime sandbox state stored in /var/lib/firefly-forage/sandboxes/*.json
//
// # Host Configuration
//
// HostConfig contains system-wide settings:
//
//	type HostConfig struct {
//	    User           string            // SSH user for sandboxes
//	    UID            int               // Host user's UID
//	    GID            int               // Host user's GID
//	    AuthorizedKeys []string          // SSH public keys
//	    Secrets        map[string]string // Secret paths by name
//	}
//
// # Templates
//
// Templates define sandbox configurations:
//
//	type Template struct {
//	    Name         string                 // Template identifier
//	    Network      string                 // "full", "restricted", or "none"
//	    AllowedHosts []string               // For restricted mode
//	    Agents       map[string]AgentConfig // Agent configurations
//	}
//
// # Sandbox Metadata
//
// SandboxMetadata tracks running sandbox state:
//
//	type SandboxMetadata struct {
//	    Name          string // Sandbox name
//	    Template      string // Template used
//	    Port          int    // SSH port
//	    Workspace     string // Workspace path
//	    WorkspaceMode string // "direct", "jj", or "git-worktree"
//	}
//
// # Validation
//
// All configuration types implement Validate() to check for required fields
// and valid values. Loading functions automatically validate after parsing.
package config

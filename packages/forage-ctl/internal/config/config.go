package config

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// sandboxNameRegex validates sandbox names.
// Names must start with a lowercase letter or digit, followed by lowercase letters, digits, underscores, or hyphens.
// Maximum length is 63 characters (common container name limit).
var sandboxNameRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

// ValidateSandboxName checks if a sandbox name is valid.
// Valid names:
//   - Start with a lowercase letter or digit
//   - Contain only lowercase letters, digits, underscores, or hyphens
//   - Are between 1 and 63 characters long
//   - Do not contain path separators or special characters
func ValidateSandboxName(name string) error {
	if name == "" {
		return fmt.Errorf("sandbox name cannot be empty")
	}

	if !sandboxNameRegex.MatchString(name) {
		return fmt.Errorf("invalid sandbox name %q: must start with a lowercase letter or digit, contain only lowercase letters, digits, underscores, or hyphens, and be at most 63 characters", name)
	}

	return nil
}

// safePath validates that a constructed path stays within the base directory.
// This prevents path traversal attacks where names like "../../../etc/passwd"
// could escape the intended directory.
func safePath(baseDir, name, suffix string) (string, error) {
	// Reject absolute paths in name
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("name cannot be an absolute path")
	}

	// Reject names containing path separators
	if filepath.Dir(name) != "." {
		return "", fmt.Errorf("name cannot contain path separators")
	}

	// Construct the path
	path := filepath.Join(baseDir, name+suffix)

	// Get absolute paths for comparison
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("invalid base directory: %w", err)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	// Ensure the resolved path is within the base directory
	// Add separator to prevent prefix matching (e.g., /var/lib/forage vs /var/lib/forage-evil)
	if !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) && absPath != absBase {
		return "", fmt.Errorf("path escapes base directory")
	}

	return path, nil
}

const (
	DefaultConfigDir  = "/etc/firefly-forage"
	DefaultStateDir   = "/var/lib/firefly-forage"
	DefaultSecretsDir = "/run/forage-secrets"
	ContainerPrefix   = "forage-"
	TmuxSessionName   = "forage"
)

// HostConfig represents the host configuration from config.json
type HostConfig struct {
	User               string            `json:"user"`
	UID                int               `json:"uid"`                      // Host user's UID
	GID                int               `json:"gid"`                      // Host user's GID
	AuthorizedKeys     []string          `json:"authorizedKeys"`
	Secrets            map[string]string `json:"secrets"`
	StateDir           string            `json:"stateDir"`
	ExtraContainerPath string            `json:"extraContainerPath"`
	NixpkgsRev         string            `json:"nixpkgsRev"`
	ProxyURL           string            `json:"proxyUrl,omitempty"`       // URL of the forage-proxy server
	UserShell          string            `json:"-"`                        // Resolved at runtime from $SHELL
}

// resolveUID looks up the UID/GID from the OS for the configured user
// when they weren't explicitly set in the NixOS config (i.e., null/0 in JSON).
func (c *HostConfig) resolveUID() error {
	if c.UID != 0 && c.GID != 0 {
		return nil
	}

	u, err := user.Lookup(c.User)
	if err != nil {
		return fmt.Errorf("failed to look up user %q: %w", c.User, err)
	}

	if c.UID == 0 {
		uid, err := strconv.Atoi(u.Uid)
		if err != nil {
			return fmt.Errorf("failed to parse UID for user %q: %w", c.User, err)
		}
		c.UID = uid
	}

	if c.GID == 0 {
		gid, err := strconv.Atoi(u.Gid)
		if err != nil {
			return fmt.Errorf("failed to parse GID for user %q: %w", c.User, err)
		}
		c.GID = gid
	}

	return nil
}

// resolveShell sets UserShell from $SHELL, falling back to "bash".
func (c *HostConfig) resolveShell() {
	shell := os.Getenv("SHELL")
	if shell != "" {
		c.UserShell = filepath.Base(shell)
	} else {
		c.UserShell = "bash"
	}
}

// Validate checks that the HostConfig is valid.
func (c *HostConfig) Validate() error {
	if c.User == "" {
		return fmt.Errorf("user is required")
	}

	return nil
}

// Template represents a sandbox template configuration
type Template struct {
	Name          string                 `json:"name"`
	Description   string                 `json:"description"`
	Network       string                 `json:"network"`
	AllowedHosts  []string               `json:"allowedHosts"`
	Agents        map[string]AgentConfig `json:"agents"`
	ExtraPackages []string               `json:"extraPackages"`
	UseProxy      bool                   `json:"useProxy,omitempty"` // Use forage-proxy for API calls
}

type AgentConfig struct {
	PackagePath           string `json:"packagePath"`
	SecretName            string `json:"secretName"`
	AuthEnvVar            string `json:"authEnvVar"`
	HostConfigDir         string `json:"hostConfigDir,omitempty"`
	ContainerConfigDir    string `json:"containerConfigDir,omitempty"`
	HostConfigDirReadOnly bool   `json:"hostConfigDirReadOnly,omitempty"`
}

// Validate checks that the Template is valid.
func (t *Template) Validate() error {
	if t.Name == "" {
		return fmt.Errorf("name is required")
	}

	if len(t.Agents) == 0 {
		return fmt.Errorf("at least one agent is required")
	}

	for name, agent := range t.Agents {
		if err := agent.Validate(); err != nil {
			return fmt.Errorf("agent %s: %w", name, err)
		}
	}

	validNetworks := map[string]bool{"full": true, "restricted": true, "none": true, "": true}
	if !validNetworks[t.Network] {
		return fmt.Errorf("invalid network mode: %s (must be full, restricted, or none)", t.Network)
	}

	return nil
}

// Validate checks that the AgentConfig is valid.
func (a *AgentConfig) Validate() error {
	if a.PackagePath == "" {
		return fmt.Errorf("packagePath is required")
	}

	// If one of secretName/authEnvVar is set, both must be set
	if (a.SecretName != "") != (a.AuthEnvVar != "") {
		return fmt.Errorf("secretName and authEnvVar must both be set or both be empty")
	}

	// Either secret-based auth OR credential mount is required
	hasSecretAuth := a.SecretName != "" && a.AuthEnvVar != ""
	hasCredentialMount := a.HostConfigDir != ""

	if !hasSecretAuth && !hasCredentialMount {
		return fmt.Errorf("either secretName/authEnvVar or hostConfigDir is required")
	}

	// Validate host config directory paths if specified
	if a.HostConfigDir != "" {
		if !filepath.IsAbs(a.HostConfigDir) {
			return fmt.Errorf("hostConfigDir must be an absolute path (got %q)", a.HostConfigDir)
		}
	}
	if a.ContainerConfigDir != "" {
		if !filepath.IsAbs(a.ContainerConfigDir) {
			return fmt.Errorf("containerConfigDir must be an absolute path (got %q)", a.ContainerConfigDir)
		}
	}
	// If hostConfigDir is set, containerConfigDir should also be set (NixOS module does this)
	if a.HostConfigDir != "" && a.ContainerConfigDir == "" {
		return fmt.Errorf("containerConfigDir is required when hostConfigDir is set")
	}
	return nil
}

// SandboxMetadata represents the metadata for a running sandbox
type SandboxMetadata struct {
	Name            string `json:"name"`
	Template        string `json:"template"`
	Workspace       string `json:"workspace"`
	NetworkSlot     int    `json:"networkSlot"`
	CreatedAt       string `json:"createdAt"`
	WorkspaceMode   string `json:"workspaceMode,omitempty"`   // "direct", "jj", or "git-worktree"
	SourceRepo      string `json:"sourceRepo,omitempty"`      // Source repo path for jj/git-worktree
	JJWorkspaceName string `json:"jjWorkspaceName,omitempty"` // JJ workspace name
	GitBranch       string `json:"gitBranch,omitempty"`       // Git branch name for worktree
}

// ContainerIP returns the container's IP address based on its network slot.
// Containers use the 10.100.X.0/24 network where X is the NetworkSlot.
// The container gets .2 (host gets .1).
func (m *SandboxMetadata) ContainerIP() string {
	return fmt.Sprintf("10.100.%d.2", m.NetworkSlot)
}

// Validate checks that the SandboxMetadata is valid.
func (m *SandboxMetadata) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}
	if m.Template == "" {
		return fmt.Errorf("template is required")
	}
	if m.NetworkSlot < 1 || m.NetworkSlot > 254 {
		return fmt.Errorf("networkSlot must be between 1 and 254 (got %d)", m.NetworkSlot)
	}
	if m.Workspace == "" {
		return fmt.Errorf("workspace is required")
	}

	validModes := map[string]bool{"direct": true, "jj": true, "git-worktree": true, "": true}
	if !validModes[m.WorkspaceMode] {
		return fmt.Errorf("invalid workspaceMode: %s", m.WorkspaceMode)
	}

	return nil
}

// Paths holds the configured paths
type Paths struct {
	ConfigDir     string
	StateDir      string
	SecretsDir    string
	SandboxesDir  string
	WorkspacesDir string
	TemplatesDir  string
}

// DefaultPaths returns the default path configuration
func DefaultPaths() *Paths {
	stateDir := DefaultStateDir
	return &Paths{
		ConfigDir:     DefaultConfigDir,
		StateDir:      stateDir,
		SecretsDir:    DefaultSecretsDir,
		SandboxesDir:  filepath.Join(stateDir, "sandboxes"),
		WorkspacesDir: filepath.Join(stateDir, "workspaces"),
		TemplatesDir:  filepath.Join(DefaultConfigDir, "templates"),
	}
}

// LoadHostConfig loads the host configuration from config.json
func LoadHostConfig(configDir string) (*HostConfig, error) {
	configPath := filepath.Join(configDir, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read host config: %w", err)
	}

	var config HostConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse host config: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid host config: %w", err)
	}

	// Resolve UID/GID from OS if not set in config (NixOS auto-assigns UIDs)
	if err := config.resolveUID(); err != nil {
		return nil, fmt.Errorf("failed to resolve user IDs: %w", err)
	}

	// Resolve user's shell from $SHELL
	config.resolveShell()

	return &config, nil
}

// LoadTemplate loads a template configuration
func LoadTemplate(templatesDir, name string) (*Template, error) {
	templatePath, err := safePath(templatesDir, name, ".json")
	if err != nil {
		return nil, fmt.Errorf("invalid template name: %w", err)
	}
	data, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read template %s: %w", name, err)
	}

	var template Template
	if err := json.Unmarshal(data, &template); err != nil {
		return nil, fmt.Errorf("failed to parse template %s: %w", name, err)
	}

	// Set name from filename if not specified in JSON
	if template.Name == "" {
		template.Name = name
	}

	if err := template.Validate(); err != nil {
		return nil, fmt.Errorf("invalid template %s: %w", name, err)
	}

	return &template, nil
}

// ListTemplates returns all available templates
func ListTemplates(templatesDir string) ([]*Template, error) {
	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read templates directory: %w", err)
	}

	var templates []*Template
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		name := entry.Name()[:len(entry.Name())-5] // Remove .json extension
		template, err := LoadTemplate(templatesDir, name)
		if err != nil {
			continue // Skip invalid templates
		}
		templates = append(templates, template)
	}

	return templates, nil
}

// LoadSandboxMetadata loads metadata for a sandbox
func LoadSandboxMetadata(sandboxesDir, name string) (*SandboxMetadata, error) {
	metaPath, err := safePath(sandboxesDir, name, ".json")
	if err != nil {
		return nil, fmt.Errorf("invalid sandbox name: %w", err)
	}
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("sandbox not found: %s", name)
	}

	var metadata SandboxMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse sandbox metadata: %w", err)
	}

	// Default workspace mode
	if metadata.WorkspaceMode == "" {
		metadata.WorkspaceMode = "direct"
	}

	return &metadata, nil
}

// SaveSandboxMetadata saves metadata for a sandbox
func SaveSandboxMetadata(sandboxesDir string, metadata *SandboxMetadata) error {
	if err := os.MkdirAll(sandboxesDir, 0755); err != nil {
		return fmt.Errorf("failed to create sandboxes directory: %w", err)
	}

	metaPath, err := safePath(sandboxesDir, metadata.Name, ".json")
	if err != nil {
		return fmt.Errorf("invalid sandbox name: %w", err)
	}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.WriteFile(metaPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	return nil
}

// DeleteSandboxMetadata removes metadata for a sandbox
func DeleteSandboxMetadata(sandboxesDir, name string) error {
	metaPath, err := safePath(sandboxesDir, name, ".json")
	if err != nil {
		return fmt.Errorf("invalid sandbox name: %w", err)
	}
	return os.Remove(metaPath)
}

// ListSandboxes returns all sandbox metadata
func ListSandboxes(sandboxesDir string) ([]*SandboxMetadata, error) {
	entries, err := os.ReadDir(sandboxesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read sandboxes directory: %w", err)
	}

	var sandboxes []*SandboxMetadata
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		name := entry.Name()[:len(entry.Name())-5]
		metadata, err := LoadSandboxMetadata(sandboxesDir, name)
		if err != nil {
			continue
		}
		sandboxes = append(sandboxes, metadata)
	}

	return sandboxes, nil
}

// SandboxExists checks if a sandbox exists
func SandboxExists(sandboxesDir, name string) bool {
	metaPath, err := safePath(sandboxesDir, name, ".json")
	if err != nil {
		return false // Invalid name means it doesn't exist
	}
	_, err = os.Stat(metaPath)
	return err == nil
}

// ContainerName returns the container name for a sandbox
func ContainerName(sandboxName string) string {
	return ContainerPrefix + sandboxName
}

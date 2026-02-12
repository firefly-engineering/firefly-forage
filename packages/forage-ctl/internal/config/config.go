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

// IsSandboxMetadataFile returns true if the filename is a valid sandbox metadata file.
// Valid metadata files are "<name>.json" where name contains no dots.
// This excludes files like "test.claude-permissions.json".
func IsSandboxMetadataFile(filename string) bool {
	if filepath.Ext(filename) != ".json" {
		return false
	}
	name := strings.TrimSuffix(filename, ".json")
	return !strings.Contains(name, ".")
}

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
	MuxSessionName    = TmuxSessionName // alias for new code
)

// AgentIdentity holds optional git authorship and SSH key configuration
// for agents running inside sandboxes. All fields are optional.
type AgentIdentity struct {
	GitUser    string `json:"gitUser,omitempty"`
	GitEmail   string `json:"gitEmail,omitempty"`
	SSHKeyPath string `json:"sshKeyPath,omitempty"` // absolute path to private key on host
}

// ValidateAgentIdentity validates an AgentIdentity configuration.
// When SSHKeyPath is non-empty, checks it's absolute, the file exists, and the .pub companion exists.
// Returns nil if identity is nil or all fields are empty.
func ValidateAgentIdentity(id *AgentIdentity) error {
	if id == nil {
		return nil
	}
	if id.SSHKeyPath == "" {
		return nil
	}
	if !filepath.IsAbs(id.SSHKeyPath) {
		return fmt.Errorf("sshKeyPath must be an absolute path (got %q)", id.SSHKeyPath)
	}
	if _, err := os.Stat(id.SSHKeyPath); err != nil {
		return fmt.Errorf("sshKeyPath %q: %w", id.SSHKeyPath, err)
	}
	pubPath := id.SSHKeyPath + ".pub"
	if _, err := os.Stat(pubPath); err != nil {
		return fmt.Errorf("sshKeyPath companion %q: %w", pubPath, err)
	}
	return nil
}

// ReadHostUserGitIdentity reads the git/jj user.name and user.email from the
// given user's config files as a fallback identity. Checks jj config first
// (since that's what forage workspaces typically use), then falls back to git config.
// Returns nil if no config is found or no user section exists.
func ReadHostUserGitIdentity(username string) *AgentIdentity {
	u, err := user.Lookup(username)
	if err != nil {
		return nil
	}

	// Try jj config first (preferred for forage workspaces)
	jjPaths := []string{
		filepath.Join(u.HomeDir, ".config", "jj", "config.toml"),
		filepath.Join(u.HomeDir, ".jjconfig.toml"),
	}
	for _, p := range jjPaths {
		if id := parseJJConfigIdentity(p); id != nil {
			return id
		}
	}

	// Fall back to git config
	gitPaths := []string{
		filepath.Join(u.HomeDir, ".gitconfig"),
		filepath.Join(u.HomeDir, ".config", "git", "config"),
	}
	for _, p := range gitPaths {
		if id := parseGitConfigIdentity(p); id != nil {
			return id
		}
	}
	return nil
}

// parseGitConfigIdentity extracts user.name and user.email from a git config file.
func parseGitConfigIdentity(path string) *AgentIdentity {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var gitUser, gitEmail string
	inUserSection := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "[user]" {
			inUserSection = true
			continue
		}
		if strings.HasPrefix(line, "[") {
			inUserSection = false
			continue
		}
		if !inUserSection {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "name":
			gitUser = val
		case "email":
			gitEmail = val
		}
	}

	if gitUser == "" && gitEmail == "" {
		return nil
	}
	return &AgentIdentity{GitUser: gitUser, GitEmail: gitEmail}
}

// parseJJConfigIdentity extracts user.name and user.email from a jj config file (TOML format).
func parseJJConfigIdentity(path string) *AgentIdentity {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var jjUser, jjEmail string
	inUserSection := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "[user]" {
			inUserSection = true
			continue
		}
		if strings.HasPrefix(line, "[") {
			inUserSection = false
			continue
		}
		if !inUserSection {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		// TOML strings are quoted
		val = strings.Trim(val, `"'`)
		switch key {
		case "name":
			jjUser = val
		case "email":
			jjEmail = val
		}
	}

	if jjUser == "" && jjEmail == "" {
		return nil
	}
	return &AgentIdentity{GitUser: jjUser, GitEmail: jjEmail}
}

// HostConfig represents the host configuration from config.json
type HostConfig struct {
	User               string            `json:"user"`
	UID                int               `json:"uid"` // Host user's UID
	GID                int               `json:"gid"` // Host user's GID
	AuthorizedKeys     []string          `json:"authorizedKeys"`
	Secrets            map[string]string `json:"secrets"`
	StateDir           string            `json:"stateDir"`
	ExtraContainerPath string            `json:"extraContainerPath"`
	NixpkgsPath        string            `json:"nixpkgsPath"`
	NixpkgsRev         string            `json:"nixpkgsRev"`
	ProxyURL           string            `json:"proxyUrl,omitempty"`      // URL of the forage-proxy server
	AgentIdentity      *AgentIdentity    `json:"agentIdentity,omitempty"` // Host-level default agent identity
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

// Validate checks that the HostConfig is valid.
func (c *HostConfig) Validate() error {
	if c.User == "" {
		return fmt.Errorf("user is required")
	}

	return nil
}

// TmuxWindow describes a tmux window to create at sandbox start.
type TmuxWindow struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

// Template represents a sandbox template configuration
type Template struct {
	Name              string                 `json:"name"`
	Description       string                 `json:"description"`
	Network           string                 `json:"network"`
	AllowedHosts      []string               `json:"allowedHosts"`
	Agents            map[string]AgentConfig `json:"agents"`
	ExtraPackages     []string               `json:"extraPackages"`
	UseProxy          bool                   `json:"useProxy,omitempty"`          // Use forage-proxy for API calls
	AgentIdentity     *AgentIdentity         `json:"agentIdentity,omitempty"`     // Template-level default agent identity
	TmuxWindows       []TmuxWindow           `json:"tmuxWindows,omitempty"`       // Explicit tmux window layout
	Multiplexer       string                 `json:"multiplexer,omitempty"`       // "tmux" (default) or "wezterm"
	ReadOnlyWorkspace bool                   `json:"readOnlyWorkspace,omitempty"` // Mount workspace as read-only
}

// AgentPermissions controls agent permission settings.
// When nil, no permission settings are generated.
type AgentPermissions struct {
	SkipAll bool     `json:"skipAll,omitempty"`
	Allow   []string `json:"allow,omitempty"`
	Deny    []string `json:"deny,omitempty"`
}

type AgentConfig struct {
	PackagePath           string            `json:"packagePath"`
	SecretName            string            `json:"secretName"`
	AuthEnvVar            string            `json:"authEnvVar"`
	HostConfigDir         string            `json:"hostConfigDir,omitempty"`
	ContainerConfigDir    string            `json:"containerConfigDir,omitempty"`
	HostConfigDirReadOnly bool              `json:"hostConfigDirReadOnly,omitempty"`
	Permissions           *AgentPermissions `json:"permissions,omitempty"`
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

// secretNameRegex validates secret names to prevent shell injection.
// Secret names are used in shell commands like $(cat /run/secrets/<name> ...),
// so they must be restricted to safe filename characters.
var secretNameRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9._-]*$`)

// Validate checks that the AgentConfig is valid.
func (a *AgentConfig) Validate() error {
	if a.PackagePath == "" {
		return fmt.Errorf("packagePath is required")
	}

	// If one of secretName/authEnvVar is set, both must be set
	if (a.SecretName != "") != (a.AuthEnvVar != "") {
		return fmt.Errorf("secretName and authEnvVar must both be set or both be empty")
	}

	// Validate secret name format to prevent shell injection
	if a.SecretName != "" && !secretNameRegex.MatchString(a.SecretName) {
		return fmt.Errorf("invalid secretName %q: must start with a letter and contain only letters, digits, dots, hyphens, or underscores", a.SecretName)
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

	// Validate permissions
	if a.Permissions != nil {
		if a.Permissions.SkipAll && (len(a.Permissions.Allow) > 0 || len(a.Permissions.Deny) > 0) {
			return fmt.Errorf("permissions: skipAll cannot be combined with allow or deny")
		}
	}

	return nil
}

// SandboxMetadata represents the metadata for a running sandbox
type SandboxMetadata struct {
	Name            string         `json:"name"`
	Template        string         `json:"template"`
	Workspace       string         `json:"workspace"`
	NetworkSlot     int            `json:"networkSlot"`
	CreatedAt       string         `json:"createdAt"`
	WorkspaceMode   string         `json:"workspaceMode,omitempty"`   // "direct", "jj", or "git-worktree"
	SourceRepo      string         `json:"sourceRepo,omitempty"`      // Source repo path for jj/git-worktree
	JJWorkspaceName string         `json:"jjWorkspaceName,omitempty"` // JJ workspace name
	GitBranch       string         `json:"gitBranch,omitempty"`       // Git branch name for worktree
	AgentIdentity   *AgentIdentity `json:"agentIdentity,omitempty"`   // Resolved agent identity
	Multiplexer     string         `json:"multiplexer,omitempty"`     // "tmux" (default) or "wezterm"
	ContainerName   string         `json:"containerName,omitempty"`   // Short container name (e.g. "f42"); empty for legacy sandboxes
	Runtime         string         `json:"runtime,omitempty"`         // Runtime backend used (e.g. "nspawn", "docker", "podman")
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
		if entry.IsDir() || !IsSandboxMetadataFile(entry.Name()) {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
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

// ContainerName returns the legacy container name for a sandbox.
// Deprecated: Use ContainerNameForSlot for new sandboxes or
// SandboxMetadata.ResolvedContainerName for existing ones.
func ContainerName(sandboxName string) string {
	return ContainerPrefix + sandboxName
}

// ContainerNameForSlot returns a short container name derived from the network slot.
// This produces names like "f1", "f42", "f254" that fit within the 11-character
// limit imposed by NixOS containers with privateNetwork.
func ContainerNameForSlot(slot int) string {
	return fmt.Sprintf("f%d", slot)
}

// ResolvedContainerName returns the container name to use for this sandbox.
// Returns the new short ContainerName if set, otherwise falls back to the
// legacy "forage-{name}" format for backward compatibility.
func (m *SandboxMetadata) ResolvedContainerName() string {
	if m.ContainerName != "" {
		return m.ContainerName
	}
	return ContainerPrefix + m.Name
}

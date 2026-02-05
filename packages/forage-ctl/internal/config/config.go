package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	DefaultConfigDir    = "/etc/firefly-forage"
	DefaultStateDir     = "/var/lib/firefly-forage"
	DefaultSecretsDir   = "/run/forage-secrets"
	ContainerPrefix     = "forage-"
	TmuxSessionName     = "forage"
)

// HostConfig represents the host configuration from config.json
type HostConfig struct {
	User               string            `json:"user"`
	PortRange          PortRange         `json:"portRange"`
	AuthorizedKeys     []string          `json:"authorizedKeys"`
	Secrets            map[string]string `json:"secrets"`
	StateDir           string            `json:"stateDir"`
	ExtraContainerPath string            `json:"extraContainerPath"`
	NixpkgsRev         string            `json:"nixpkgsRev"`
	ProxyURL           string            `json:"proxyUrl,omitempty"` // URL of the forage-proxy server
}

type PortRange struct {
	From int `json:"from"`
	To   int `json:"to"`
}

// Validate checks that the HostConfig is valid.
func (c *HostConfig) Validate() error {
	if c.User == "" {
		return fmt.Errorf("user is required")
	}

	if err := c.PortRange.Validate(); err != nil {
		return fmt.Errorf("portRange: %w", err)
	}

	return nil
}

// Validate checks that the PortRange is valid.
func (p *PortRange) Validate() error {
	if p.From <= 0 {
		return fmt.Errorf("from must be positive (got %d)", p.From)
	}
	if p.To <= 0 {
		return fmt.Errorf("to must be positive (got %d)", p.To)
	}
	if p.From > p.To {
		return fmt.Errorf("from (%d) must be <= to (%d)", p.From, p.To)
	}
	if p.From > 65535 || p.To > 65535 {
		return fmt.Errorf("ports must be <= 65535")
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
	PackagePath string `json:"packagePath"`
	SecretName  string `json:"secretName"`
	AuthEnvVar  string `json:"authEnvVar"`
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
	if a.SecretName == "" {
		return fmt.Errorf("secretName is required")
	}
	if a.AuthEnvVar == "" {
		return fmt.Errorf("authEnvVar is required")
	}
	return nil
}

// SandboxMetadata represents the metadata for a running sandbox
type SandboxMetadata struct {
	Name            string `json:"name"`
	Template        string `json:"template"`
	Port            int    `json:"port"`
	Workspace       string `json:"workspace"`
	NetworkSlot     int    `json:"networkSlot"`
	CreatedAt       string `json:"createdAt"`
	WorkspaceMode   string `json:"workspaceMode,omitempty"`   // "direct", "jj", or "git-worktree"
	SourceRepo      string `json:"sourceRepo,omitempty"`      // Source repo path for jj/git-worktree
	JJWorkspaceName string `json:"jjWorkspaceName,omitempty"` // JJ workspace name
	GitBranch       string `json:"gitBranch,omitempty"`       // Git branch name for worktree
}

// Validate checks that the SandboxMetadata is valid.
func (m *SandboxMetadata) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}
	if m.Template == "" {
		return fmt.Errorf("template is required")
	}
	if m.Port <= 0 || m.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535 (got %d)", m.Port)
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

	return &config, nil
}

// LoadTemplate loads a template configuration
func LoadTemplate(templatesDir, name string) (*Template, error) {
	templatePath := filepath.Join(templatesDir, name+".json")
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
	metaPath := filepath.Join(sandboxesDir, name+".json")
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

	metaPath := filepath.Join(sandboxesDir, metadata.Name+".json")
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
	metaPath := filepath.Join(sandboxesDir, name+".json")
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
	metaPath := filepath.Join(sandboxesDir, name+".json")
	_, err := os.Stat(metaPath)
	return err == nil
}

// ContainerName returns the container name for a sandbox
func ContainerName(sandboxName string) string {
	return ContainerPrefix + sandboxName
}

package sandbox

import (
	"context"
	"path/filepath"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/injection"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/skills"
)

// SkillsContributor generates system prompts and skill files.
// This wraps the skills package and implements GeneratedFileContributor.
type SkillsContributor struct {
	HomeDir  string // Container home directory (e.g., "/home/agent")
	Template *config.Template
	Metadata *config.SandboxMetadata
}

// NewSkillsContributor creates a new SkillsContributor.
func NewSkillsContributor(homeDir string, template *config.Template, metadata *config.SandboxMetadata) *SkillsContributor {
	if homeDir == "" {
		homeDir = "/home/agent"
	}
	return &SkillsContributor{
		HomeDir:  homeDir,
		Template: template,
		Metadata: metadata,
	}
}

// ContributeGeneratedFiles generates the system prompt and skill files.
func (s *SkillsContributor) ContributeGeneratedFiles(ctx context.Context, req *injection.GeneratedFileRequest) ([]injection.GeneratedFile, error) {
	if req == nil || s.Template == nil || s.Metadata == nil {
		return nil, nil
	}

	var files []injection.GeneratedFile

	// Analyze the project for context-aware skills
	analyzer := skills.NewAnalyzer(req.WorkspacePath)
	projectInfo := analyzer.Analyze()

	// Generate system prompt using existing skills package
	promptContent := skills.GenerateSystemPrompt(s.Metadata, s.Template)
	files = append(files, injection.GeneratedFile{
		ContainerPath: filepath.Join(s.HomeDir, ".config", "forage", "system-prompt.md"),
		Content:       []byte(promptContent),
		Mode:          0644,
		ReadOnly:      true,
	})

	// Generate skill files using existing skills package
	skillFiles := skills.GenerateSkillFiles(s.Metadata, s.Template, projectInfo)
	claudeSkillsDir := filepath.Join(s.HomeDir, ".claude", "skills")
	for skillName, content := range skillFiles {
		files = append(files, injection.GeneratedFile{
			ContainerPath: filepath.Join(claudeSkillsDir, skillName, "SKILL.md"),
			Content:       []byte(content),
			Mode:          0644,
			ReadOnly:      true,
		})
	}

	return files, nil
}

// ContributeTmpfilesRules returns tmpfiles rules for skill directories.
func (s *SkillsContributor) ContributeTmpfilesRules(ctx context.Context, req *injection.TmpfilesRequest) ([]string, error) {
	username := "agent"
	if req != nil && req.Username != "" {
		username = req.Username
	}
	homeDir := s.HomeDir
	if req != nil && req.HomeDir != "" {
		homeDir = req.HomeDir
	}

	return []string{
		"d " + filepath.Join(homeDir, ".config", "forage") + " 0755 " + username + " users -",
	}, nil
}

// Ensure SkillsContributor implements interfaces
var (
	_ injection.GeneratedFileContributor = (*SkillsContributor)(nil)
	_ injection.TmpfilesContributor      = (*SkillsContributor)(nil)
)

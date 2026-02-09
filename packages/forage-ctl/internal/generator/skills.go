package generator

import (
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/skills"
)

// GenerateSystemPrompt generates the compact system prompt content for --append-system-prompt.
func GenerateSystemPrompt(metadata *config.SandboxMetadata, template *config.Template) string {
	return skills.GenerateSystemPrompt(metadata, template)
}

// GenerateSkillFiles generates skill file contents based on project analysis.
// Returns a map of skill name to SKILL.md content. May return an empty map.
func GenerateSkillFiles(metadata *config.SandboxMetadata, template *config.Template, info *skills.ProjectInfo) map[string]string {
	return skills.GenerateSkillFiles(metadata, template, info)
}

// GenerateSkills generates the combined skills content.
// Deprecated: Use GenerateSystemPrompt and GenerateSkillFiles instead.
func GenerateSkills(metadata *config.SandboxMetadata, template *config.Template) string {
	return skills.GenerateSystemPrompt(metadata, template)
}

package generator

import (
	"fmt"
	"strings"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
)

// GenerateSkills generates the CLAUDE.md skills file content
func GenerateSkills(metadata *config.SandboxMetadata, template *config.Template) string {
	var sb strings.Builder

	sb.WriteString("# Agent Instructions\n\n")
	sb.WriteString("You are running in a sandboxed environment managed by Firefly Forage.\n\n")

	sb.WriteString("## Environment\n\n")
	sb.WriteString(fmt.Sprintf("- **Sandbox**: %s\n", metadata.Name))
	sb.WriteString(fmt.Sprintf("- **Template**: %s\n", metadata.Template))
	sb.WriteString(fmt.Sprintf("- **Workspace**: /workspace\n"))

	if metadata.WorkspaceMode == "jj" {
		sb.WriteString(fmt.Sprintf("- **Mode**: jj workspace (isolated from source)\n"))
		sb.WriteString(fmt.Sprintf("- **Source Repo**: %s\n", metadata.SourceRepo))
	}

	sb.WriteString("\n## Workspace\n\n")
	sb.WriteString("Your workspace is mounted at `/workspace`. All your work should be done there.\n")
	sb.WriteString("The workspace persists across container restarts.\n\n")

	// JJ instructions if applicable
	if metadata.WorkspaceMode == "jj" {
		sb.WriteString("## Version Control: JJ (Jujutsu)\n\n")
		sb.WriteString("This workspace uses `jj` for version control:\n\n")
		sb.WriteString("```bash\n")
		sb.WriteString("jj status         # Show working copy status\n")
		sb.WriteString("jj diff           # Show changes\n")
		sb.WriteString("jj new            # Create new change\n")
		sb.WriteString("jj describe -m \"\" # Set commit message\n")
		sb.WriteString("jj bookmark set   # Update bookmark\n")
		sb.WriteString("```\n\n")
		sb.WriteString("This is an isolated jj workspace - changes don't affect other workspaces.\n\n")
	} else {
		sb.WriteString("## Version Control\n\n")
		sb.WriteString("Use `git` or `jj` for version control as needed.\n\n")
	}

	// Network info
	sb.WriteString("## Network\n\n")
	switch template.Network {
	case "none":
		sb.WriteString("**No network access** - This sandbox has no external network connectivity.\n")
	case "restricted":
		sb.WriteString("**Restricted network** - Only specific hosts are accessible:\n")
		for _, host := range template.AllowedHosts {
			sb.WriteString(fmt.Sprintf("- %s\n", host))
		}
	default:
		sb.WriteString("Full network access is available.\n")
	}
	sb.WriteString("\n")

	// Agents
	if len(template.Agents) > 0 {
		sb.WriteString("## Available Agents\n\n")
		for name, agent := range template.Agents {
			sb.WriteString(fmt.Sprintf("- **%s**", name))
			if agent.AuthEnvVar != "" {
				sb.WriteString(fmt.Sprintf(" (auth via $%s)", agent.AuthEnvVar))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Guidelines\n\n")
	sb.WriteString("- Work within the `/workspace` directory\n")
	sb.WriteString("- The container filesystem (except /workspace) is ephemeral\n")
	sb.WriteString("- Use tmux for persistent sessions (`tmux attach -t forage`)\n")

	return sb.String()
}

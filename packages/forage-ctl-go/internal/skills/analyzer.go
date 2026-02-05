// Package skills provides advanced skill injection based on project analysis
package skills

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/logging"
)

// ProjectType represents the detected project type
type ProjectType string

const (
	ProjectTypeUnknown    ProjectType = "unknown"
	ProjectTypeGo         ProjectType = "go"
	ProjectTypeRust       ProjectType = "rust"
	ProjectTypePython     ProjectType = "python"
	ProjectTypeNode       ProjectType = "node"
	ProjectTypeNix        ProjectType = "nix"
	ProjectTypeTypescript ProjectType = "typescript"
)

// ProjectInfo holds analyzed project information
type ProjectInfo struct {
	Type         ProjectType
	HasGit       bool
	HasJJ        bool
	HasNixFlake  bool
	HasTests     bool
	HasCI        bool
	BuildSystem  string
	TestCommand  string
	BuildCommand string
	Frameworks   []string
}

// Analyzer analyzes projects to generate context-aware skills
type Analyzer struct {
	workspacePath string
}

// NewAnalyzer creates a new project analyzer
func NewAnalyzer(workspacePath string) *Analyzer {
	return &Analyzer{workspacePath: workspacePath}
}

// Analyze analyzes the project and returns project info
func (a *Analyzer) Analyze() *ProjectInfo {
	info := &ProjectInfo{
		Type: ProjectTypeUnknown,
	}

	logging.Debug("analyzing project", "path", a.workspacePath)

	// Check for version control
	info.HasGit = a.fileExists(".git")
	info.HasJJ = a.fileExists(".jj")

	// Check for nix
	info.HasNixFlake = a.fileExists("flake.nix")

	// Check for CI
	info.HasCI = a.fileExists(".github/workflows") ||
		a.fileExists(".gitlab-ci.yml") ||
		a.fileExists(".circleci")

	// Detect project type
	info.Type = a.detectProjectType()

	// Detect build system and commands based on type
	a.detectBuildSystem(info)

	// Detect frameworks
	a.detectFrameworks(info)

	logging.Debug("project analysis complete",
		"type", info.Type,
		"hasGit", info.HasGit,
		"hasJJ", info.HasJJ,
		"hasNixFlake", info.HasNixFlake,
	)

	return info
}

func (a *Analyzer) fileExists(name string) bool {
	path := filepath.Join(a.workspacePath, name)
	_, err := os.Stat(path)
	return err == nil
}

func (a *Analyzer) readFile(name string) string {
	path := filepath.Join(a.workspacePath, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func (a *Analyzer) detectProjectType() ProjectType {
	// Check for Go
	if a.fileExists("go.mod") {
		return ProjectTypeGo
	}

	// Check for Rust
	if a.fileExists("Cargo.toml") {
		return ProjectTypeRust
	}

	// Check for Python
	if a.fileExists("pyproject.toml") || a.fileExists("setup.py") || a.fileExists("requirements.txt") {
		return ProjectTypePython
	}

	// Check for Node/TypeScript
	if a.fileExists("package.json") {
		pkgJson := a.readFile("package.json")
		if strings.Contains(pkgJson, "typescript") || a.fileExists("tsconfig.json") {
			return ProjectTypeTypescript
		}
		return ProjectTypeNode
	}

	// Check for Nix
	if a.fileExists("flake.nix") || a.fileExists("default.nix") {
		return ProjectTypeNix
	}

	return ProjectTypeUnknown
}

func (a *Analyzer) detectBuildSystem(info *ProjectInfo) {
	switch info.Type {
	case ProjectTypeGo:
		info.BuildSystem = "go"
		info.BuildCommand = "go build ./..."
		info.TestCommand = "go test ./..."
		info.HasTests = a.fileExists("*_test.go") || a.hasFilesMatching("**/*_test.go")

	case ProjectTypeRust:
		info.BuildSystem = "cargo"
		info.BuildCommand = "cargo build"
		info.TestCommand = "cargo test"
		info.HasTests = true // Rust tests are inline

	case ProjectTypePython:
		if a.fileExists("pyproject.toml") {
			content := a.readFile("pyproject.toml")
			if strings.Contains(content, "poetry") {
				info.BuildSystem = "poetry"
				info.TestCommand = "poetry run pytest"
			} else if strings.Contains(content, "hatch") {
				info.BuildSystem = "hatch"
				info.TestCommand = "hatch run test"
			} else {
				info.BuildSystem = "pip"
				info.TestCommand = "pytest"
			}
		} else {
			info.BuildSystem = "pip"
			info.TestCommand = "pytest"
		}
		info.HasTests = a.fileExists("tests") || a.fileExists("test")

	case ProjectTypeNode, ProjectTypeTypescript:
		info.BuildSystem = "npm"
		pkgJson := a.readFile("package.json")

		if a.fileExists("pnpm-lock.yaml") {
			info.BuildSystem = "pnpm"
		} else if a.fileExists("yarn.lock") {
			info.BuildSystem = "yarn"
		} else if a.fileExists("bun.lockb") {
			info.BuildSystem = "bun"
		}

		if strings.Contains(pkgJson, `"build"`) {
			info.BuildCommand = info.BuildSystem + " run build"
		}
		if strings.Contains(pkgJson, `"test"`) {
			info.TestCommand = info.BuildSystem + " run test"
			info.HasTests = true
		}

	case ProjectTypeNix:
		info.BuildSystem = "nix"
		info.BuildCommand = "nix build"
		if a.fileExists("flake.nix") {
			content := a.readFile("flake.nix")
			if strings.Contains(content, "checks") {
				info.TestCommand = "nix flake check"
				info.HasTests = true
			}
		}
	}
}

func (a *Analyzer) detectFrameworks(info *ProjectInfo) {
	switch info.Type {
	case ProjectTypeGo:
		goMod := a.readFile("go.mod")
		if strings.Contains(goMod, "github.com/gin-gonic/gin") {
			info.Frameworks = append(info.Frameworks, "gin")
		}
		if strings.Contains(goMod, "github.com/labstack/echo") {
			info.Frameworks = append(info.Frameworks, "echo")
		}
		if strings.Contains(goMod, "github.com/spf13/cobra") {
			info.Frameworks = append(info.Frameworks, "cobra")
		}

	case ProjectTypeTypescript, ProjectTypeNode:
		pkgJson := a.readFile("package.json")
		if strings.Contains(pkgJson, "react") {
			info.Frameworks = append(info.Frameworks, "react")
		}
		if strings.Contains(pkgJson, "next") {
			info.Frameworks = append(info.Frameworks, "nextjs")
		}
		if strings.Contains(pkgJson, "express") {
			info.Frameworks = append(info.Frameworks, "express")
		}
		if strings.Contains(pkgJson, "nestjs") {
			info.Frameworks = append(info.Frameworks, "nestjs")
		}

	case ProjectTypePython:
		requirements := a.readFile("requirements.txt") + a.readFile("pyproject.toml")
		if strings.Contains(requirements, "django") {
			info.Frameworks = append(info.Frameworks, "django")
		}
		if strings.Contains(requirements, "flask") {
			info.Frameworks = append(info.Frameworks, "flask")
		}
		if strings.Contains(requirements, "fastapi") {
			info.Frameworks = append(info.Frameworks, "fastapi")
		}

	case ProjectTypeRust:
		cargoToml := a.readFile("Cargo.toml")
		if strings.Contains(cargoToml, "actix") {
			info.Frameworks = append(info.Frameworks, "actix")
		}
		if strings.Contains(cargoToml, "axum") {
			info.Frameworks = append(info.Frameworks, "axum")
		}
		if strings.Contains(cargoToml, "tokio") {
			info.Frameworks = append(info.Frameworks, "tokio")
		}
	}
}

func (a *Analyzer) hasFilesMatching(pattern string) bool {
	matches, _ := filepath.Glob(filepath.Join(a.workspacePath, pattern))
	return len(matches) > 0
}

// GenerateSkills generates skill content based on project analysis
func GenerateSkills(metadata *config.SandboxMetadata, template *config.Template, info *ProjectInfo) string {
	var sb strings.Builder

	sb.WriteString("# Agent Instructions\n\n")
	sb.WriteString("You are running in a sandboxed environment managed by Firefly Forage.\n\n")

	// Environment section
	sb.WriteString("## Environment\n\n")
	sb.WriteString("- **Sandbox**: " + metadata.Name + "\n")
	sb.WriteString("- **Template**: " + metadata.Template + "\n")
	sb.WriteString("- **Workspace**: /workspace\n")

	if metadata.WorkspaceMode == "jj" {
		sb.WriteString("- **Mode**: jj workspace (isolated from source)\n")
		sb.WriteString("- **Source Repo**: " + metadata.SourceRepo + "\n")
	}

	sb.WriteString("\n")

	// Project-specific section
	if info != nil && info.Type != ProjectTypeUnknown {
		sb.WriteString("## Project\n\n")
		sb.WriteString("- **Type**: " + string(info.Type) + "\n")

		if info.BuildSystem != "" {
			sb.WriteString("- **Build System**: " + info.BuildSystem + "\n")
		}

		if len(info.Frameworks) > 0 {
			sb.WriteString("- **Frameworks**: " + strings.Join(info.Frameworks, ", ") + "\n")
		}

		sb.WriteString("\n### Common Commands\n\n")
		sb.WriteString("```bash\n")

		if info.BuildCommand != "" {
			sb.WriteString("# Build\n")
			sb.WriteString(info.BuildCommand + "\n\n")
		}

		if info.TestCommand != "" {
			sb.WriteString("# Test\n")
			sb.WriteString(info.TestCommand + "\n")
		}

		sb.WriteString("```\n\n")
	}

	// Version control section
	if metadata.WorkspaceMode == "jj" || (info != nil && info.HasJJ) {
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
	} else if info != nil && info.HasGit {
		sb.WriteString("## Version Control: Git\n\n")
		sb.WriteString("Standard git workflow is available.\n\n")
	}

	// Nix section
	if info != nil && info.HasNixFlake {
		sb.WriteString("## Nix\n\n")
		sb.WriteString("This project uses Nix flakes:\n\n")
		sb.WriteString("```bash\n")
		sb.WriteString("nix build         # Build the project\n")
		sb.WriteString("nix develop       # Enter dev shell\n")
		sb.WriteString("nix flake check   # Run checks\n")
		sb.WriteString("```\n\n")
		sb.WriteString("The nix store is shared read-only from the host.\n\n")
	}

	// Network section
	sb.WriteString("## Network\n\n")
	switch template.Network {
	case "none":
		sb.WriteString("**No network access** - This sandbox has no external network connectivity.\n")
	case "restricted":
		sb.WriteString("**Restricted network** - Only specific hosts are accessible:\n")
		for _, host := range template.AllowedHosts {
			sb.WriteString("- " + host + "\n")
		}
	default:
		sb.WriteString("Full network access is available.\n")
	}
	sb.WriteString("\n")

	// Agents section
	if len(template.Agents) > 0 {
		sb.WriteString("## Available Agents\n\n")
		for name, agent := range template.Agents {
			sb.WriteString("- **" + name + "**")
			if agent.AuthEnvVar != "" {
				sb.WriteString(" (auth via $" + agent.AuthEnvVar + ")")
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Guidelines
	sb.WriteString("## Guidelines\n\n")
	sb.WriteString("- Work within the `/workspace` directory\n")
	sb.WriteString("- The container filesystem (except /workspace) is ephemeral\n")
	sb.WriteString("- Use tmux for persistent sessions (`tmux attach -t forage`)\n")

	return sb.String()
}

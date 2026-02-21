package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
)

func TestAnalyzer_DetectGoProject(t *testing.T) {
	tmpDir := t.TempDir()

	// Create go.mod
	goMod := `module example.com/test

go 1.21

require github.com/spf13/cobra v1.8.0
`
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}

	// Create .git directory
	if err := os.MkdirAll(filepath.Join(tmpDir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create test file
	if err := os.WriteFile(filepath.Join(tmpDir, "main_test.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}

	analyzer := NewAnalyzer(tmpDir)
	info := analyzer.Analyze()

	if info.Type != ProjectTypeGo {
		t.Errorf("expected ProjectTypeGo, got %v", info.Type)
	}
	if info.BuildSystem != "go" {
		t.Errorf("expected build system 'go', got %v", info.BuildSystem)
	}
	if info.BuildCommand != "go build ./..." {
		t.Errorf("expected build command 'go build ./...', got %v", info.BuildCommand)
	}
	if info.TestCommand != "go test ./..." {
		t.Errorf("expected test command 'go test ./...', got %v", info.TestCommand)
	}
	if !info.HasGit {
		t.Error("expected HasGit to be true")
	}
	if !contains(info.Frameworks, "cobra") {
		t.Errorf("expected frameworks to contain 'cobra', got %v", info.Frameworks)
	}
}

func TestAnalyzer_DetectRustProject(t *testing.T) {
	tmpDir := t.TempDir()

	cargoToml := `[package]
name = "test"
version = "0.1.0"

[dependencies]
tokio = "1.0"
axum = "0.7"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "Cargo.toml"), []byte(cargoToml), 0644); err != nil {
		t.Fatal(err)
	}

	analyzer := NewAnalyzer(tmpDir)
	info := analyzer.Analyze()

	if info.Type != ProjectTypeRust {
		t.Errorf("expected ProjectTypeRust, got %v", info.Type)
	}
	if info.BuildSystem != "cargo" {
		t.Errorf("expected build system 'cargo', got %v", info.BuildSystem)
	}
	if !contains(info.Frameworks, "tokio") {
		t.Errorf("expected frameworks to contain 'tokio', got %v", info.Frameworks)
	}
	if !contains(info.Frameworks, "axum") {
		t.Errorf("expected frameworks to contain 'axum', got %v", info.Frameworks)
	}
}

func TestAnalyzer_DetectPythonProject(t *testing.T) {
	tmpDir := t.TempDir()

	pyproject := `[tool.poetry]
name = "test"
version = "0.1.0"

[tool.poetry.dependencies]
fastapi = "^0.100.0"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "pyproject.toml"), []byte(pyproject), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(tmpDir, "tests"), 0755); err != nil {
		t.Fatal(err)
	}

	analyzer := NewAnalyzer(tmpDir)
	info := analyzer.Analyze()

	if info.Type != ProjectTypePython {
		t.Errorf("expected ProjectTypePython, got %v", info.Type)
	}
	if info.BuildSystem != "poetry" {
		t.Errorf("expected build system 'poetry', got %v", info.BuildSystem)
	}
	if info.TestCommand != "poetry run pytest" {
		t.Errorf("expected test command 'poetry run pytest', got %v", info.TestCommand)
	}
	if !info.HasTests {
		t.Error("expected HasTests to be true")
	}
	if !contains(info.Frameworks, "fastapi") {
		t.Errorf("expected frameworks to contain 'fastapi', got %v", info.Frameworks)
	}
}

func TestAnalyzer_DetectTypescriptProject(t *testing.T) {
	tmpDir := t.TempDir()

	pkgJson := `{
  "name": "test",
  "dependencies": {
    "react": "^18.0.0",
    "next": "^14.0.0"
  },
  "devDependencies": {
    "typescript": "^5.0.0"
  },
  "scripts": {
    "build": "next build",
    "test": "jest"
  }
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(pkgJson), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "pnpm-lock.yaml"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	analyzer := NewAnalyzer(tmpDir)
	info := analyzer.Analyze()

	if info.Type != ProjectTypeTypescript {
		t.Errorf("expected ProjectTypeTypescript, got %v", info.Type)
	}
	if info.BuildSystem != "pnpm" {
		t.Errorf("expected build system 'pnpm', got %v", info.BuildSystem)
	}
	if info.BuildCommand != "pnpm run build" {
		t.Errorf("expected build command 'pnpm run build', got %v", info.BuildCommand)
	}
	if !contains(info.Frameworks, "react") {
		t.Errorf("expected frameworks to contain 'react', got %v", info.Frameworks)
	}
	if !contains(info.Frameworks, "nextjs") {
		t.Errorf("expected frameworks to contain 'nextjs', got %v", info.Frameworks)
	}
}

func TestAnalyzer_DetectNixProject(t *testing.T) {
	tmpDir := t.TempDir()

	flakeNix := `{
  outputs = { self, nixpkgs }: {
    packages.default = ...;
    checks.default = ...;
  };
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "flake.nix"), []byte(flakeNix), 0644); err != nil {
		t.Fatal(err)
	}

	analyzer := NewAnalyzer(tmpDir)
	info := analyzer.Analyze()

	if info.Type != ProjectTypeNix {
		t.Errorf("expected ProjectTypeNix, got %v", info.Type)
	}
	if info.BuildSystem != "nix" {
		t.Errorf("expected build system 'nix', got %v", info.BuildSystem)
	}
	if info.BuildCommand != "nix build" {
		t.Errorf("expected build command 'nix build', got %v", info.BuildCommand)
	}
	if info.TestCommand != "nix flake check" {
		t.Errorf("expected test command 'nix flake check', got %v", info.TestCommand)
	}
	if !info.HasNixFlake {
		t.Error("expected HasNixFlake to be true")
	}
}

func TestAnalyzer_DetectJJ(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tmpDir, ".jj"), 0755); err != nil {
		t.Fatal(err)
	}

	analyzer := NewAnalyzer(tmpDir)
	info := analyzer.Analyze()

	if !info.HasJJ {
		t.Error("expected HasJJ to be true")
	}
}

func TestAnalyzer_DetectCI(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tmpDir, ".github", "workflows"), 0755); err != nil {
		t.Fatal(err)
	}

	analyzer := NewAnalyzer(tmpDir)
	info := analyzer.Analyze()

	if !info.HasCI {
		t.Error("expected HasCI to be true")
	}
}

func TestGenerateSkills(t *testing.T) {
	metadata := &config.SandboxMetadata{
		Name:          "test-sandbox",
		Template:      "claude",
		WorkspaceMode: "jj",
		SourceRepo:    "/home/user/project",
	}

	template := &config.Template{
		Name:         "claude",
		Network:      "restricted",
		AllowedHosts: []string{"api.anthropic.com", "github.com"},
		Agents: map[string]config.AgentConfig{
			"claude": {AuthEnvVar: "ANTHROPIC_API_KEY"},
		},
	}

	projectInfo := &ProjectInfo{
		Type:         ProjectTypeGo,
		HasGit:       true,
		HasJJ:        true,
		HasNixFlake:  true,
		HasTests:     true,
		BuildSystem:  "go",
		BuildCommand: "go build ./...",
		TestCommand:  "go test ./...",
		Frameworks:   []string{"cobra"},
	}

	content := GenerateSkills(metadata, template, projectInfo)

	// Check for expected sections
	expectedStrings := []string{
		"# Agent Instructions",
		"test-sandbox",
		"claude",
		"jj workspace",
		"/home/user/project",
		"## Project",
		"go",
		"cobra",
		"go build ./...",
		"go test ./...",
		"## Version Control: JJ",
		"jj status",
		"## Nix",
		"nix build",
		"## Network",
		"Restricted network",
		"api.anthropic.com",
		"github.com",
		"## Available Agents",
		"ANTHROPIC_API_KEY",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(content, expected) {
			t.Errorf("expected content to contain %q", expected)
		}
	}
}

func TestGenerateSkills_DirectMode(t *testing.T) {
	metadata := &config.SandboxMetadata{
		Name:          "test-sandbox",
		Template:      "claude",
		WorkspaceMode: "direct",
	}

	template := &config.Template{
		Name:    "claude",
		Network: "full",
	}

	projectInfo := &ProjectInfo{
		Type:   ProjectTypeUnknown,
		HasGit: true,
		HasJJ:  false,
	}

	content := GenerateSkills(metadata, template, projectInfo)

	// Should have git section, not jj section
	if !strings.Contains(content, "## Version Control: Git") {
		t.Error("expected content to contain git section")
	}
	if strings.Contains(content, "jj workspace") {
		t.Error("did not expect jj workspace content")
	}
	// Should have full network access
	if !strings.Contains(content, "Full network access") {
		t.Error("expected content to contain full network access")
	}
}

func TestGenerateSkills_NoNetwork(t *testing.T) {
	metadata := &config.SandboxMetadata{
		Name:     "test-sandbox",
		Template: "isolated",
	}

	template := &config.Template{
		Name:    "isolated",
		Network: "none",
	}

	content := GenerateSkills(metadata, template, nil)

	if !strings.Contains(content, "No network access") {
		t.Error("expected content to contain no network message")
	}
}

func TestGenerateSkills_IdentityGitOnly(t *testing.T) {
	metadata := &config.SandboxMetadata{
		Name:          "test-sandbox",
		Template:      "claude",
		WorkspaceMode: "jj",
		SourceRepo:    "/home/user/project",
		AgentIdentity: &config.AgentIdentity{
			GitUser:  "Agent Bot",
			GitEmail: "agent@example.com",
		},
	}

	template := &config.Template{
		Name:    "claude",
		Network: "full",
	}

	content := GenerateSkills(metadata, template, nil)

	if !strings.Contains(content, "## Identity") {
		t.Error("expected content to contain Identity section")
	}
	if !strings.Contains(content, "Agent Bot") {
		t.Error("expected content to contain git user name")
	}
	if !strings.Contains(content, "agent@example.com") {
		t.Error("expected content to contain git email")
	}
	if strings.Contains(content, "SSH key") {
		t.Error("should not mention SSH key when not configured")
	}
}

func TestGenerateSkills_IdentityWithSSH(t *testing.T) {
	metadata := &config.SandboxMetadata{
		Name:     "test-sandbox",
		Template: "claude",
		AgentIdentity: &config.AgentIdentity{
			GitUser:    "Agent Bot",
			GitEmail:   "agent@example.com",
			SSHKeyPath: "/run/secrets/agent-key",
		},
	}

	template := &config.Template{
		Name:    "claude",
		Network: "full",
	}

	content := GenerateSkills(metadata, template, nil)

	if !strings.Contains(content, "## Identity") {
		t.Error("expected content to contain Identity section")
	}
	if !strings.Contains(content, "SSH key is available") {
		t.Error("expected content to mention SSH key availability")
	}
}

func TestGenerateSkills_NoIdentity(t *testing.T) {
	metadata := &config.SandboxMetadata{
		Name:     "test-sandbox",
		Template: "claude",
	}

	template := &config.Template{
		Name:    "claude",
		Network: "full",
	}

	content := GenerateSkills(metadata, template, nil)

	if strings.Contains(content, "## Identity") {
		t.Error("should not contain Identity section when no identity")
	}
}

func TestGenerateSystemPrompt(t *testing.T) {
	metadata := &config.SandboxMetadata{
		Name:          "test-sandbox",
		Template:      "claude",
		WorkspaceMode: "jj",
		SourceRepo:    "/home/user/project",
		AgentIdentity: &config.AgentIdentity{
			GitUser:    "Bot",
			GitEmail:   "bot@test.com",
			SSHKeyPath: "/key",
		},
	}

	template := &config.Template{
		Name:         "claude",
		Network:      "restricted",
		AllowedHosts: []string{"api.anthropic.com", "github.com"},
		Agents: map[string]config.AgentConfig{
			"claude": {AuthEnvVar: "ANTHROPIC_API_KEY"},
		},
	}

	result := GenerateSystemPrompt(metadata, template)

	expected := []string{
		"test-sandbox",
		"claude",
		"/workspace",
		"jj workspace",
		"/home/user/project",
		"Restricted network",
		"api.anthropic.com",
		"github.com",
		"Identity",
		"Bot",
		"bot@test.com",
		"SSH key available",
		"tmux",
	}

	for _, s := range expected {
		if !strings.Contains(result, s) {
			t.Errorf("system prompt should contain %q\nGot:\n%s", s, result)
		}
	}
}

func TestGenerateSystemPrompt_GitWorktreeMode(t *testing.T) {
	metadata := &config.SandboxMetadata{
		Name:          "test-sandbox",
		Template:      "claude",
		WorkspaceMode: "git-worktree",
		SourceRepo:    "/home/user/project",
		GitBranch:     "feature-branch",
	}

	template := &config.Template{
		Name:    "claude",
		Network: "full",
	}

	result := GenerateSystemPrompt(metadata, template)

	if !strings.Contains(result, "git worktree") {
		t.Errorf("system prompt should mention git worktree mode\nGot:\n%s", result)
	}
	if !strings.Contains(result, "feature-branch") {
		t.Errorf("system prompt should contain branch name\nGot:\n%s", result)
	}
}

func TestGenerateSkillFiles_AllSkills(t *testing.T) {
	metadata := &config.SandboxMetadata{
		Name:          "test-sandbox",
		Template:      "claude",
		WorkspaceMode: "jj",
	}

	template := &config.Template{
		Name:    "claude",
		Network: "full",
	}

	info := &ProjectInfo{
		Type:         ProjectTypeGo,
		HasGit:       true,
		HasJJ:        true,
		HasNixFlake:  true,
		BuildSystem:  "go",
		BuildCommand: "go build ./...",
		TestCommand:  "go test ./...",
		Frameworks:   []string{"cobra"},
	}

	result := GenerateSkillFiles(metadata, template, info)

	if _, ok := result["forage-vcs"]; !ok {
		t.Error("expected forage-vcs skill")
	}
	if _, ok := result["forage-nix"]; !ok {
		t.Error("expected forage-nix skill")
	}

	// VCS skill should contain jj content
	vcs := result["forage-vcs"]
	if !strings.Contains(vcs, "jj status") {
		t.Error("VCS skill should contain jj commands")
	}
	if !strings.Contains(vcs, "user-invocable: false") {
		t.Error("VCS skill should have frontmatter")
	}

	// Nix skill should contain nix content
	nix := result["forage-nix"]
	if !strings.Contains(nix, "nix build") {
		t.Error("Nix skill should contain nix build command")
	}
}

func TestGenerateSkillFiles_GitWorktree(t *testing.T) {
	metadata := &config.SandboxMetadata{
		Name:          "test-sandbox",
		Template:      "claude",
		WorkspaceMode: "git-worktree",
		GitBranch:     "feature-x",
	}

	template := &config.Template{
		Name:    "claude",
		Network: "full",
	}

	result := GenerateSkillFiles(metadata, template, nil)

	vcs, ok := result["forage-vcs"]
	if !ok {
		t.Fatal("expected forage-vcs skill for git-worktree mode")
	}

	if !strings.Contains(vcs, "feature-x") {
		t.Error("VCS skill should contain branch name")
	}
	if !strings.Contains(vcs, "Git Worktree") {
		t.Error("VCS skill should mention Git Worktree")
	}
}

func TestGenerateSkillFiles_NoSkills(t *testing.T) {
	metadata := &config.SandboxMetadata{
		Name:          "test-sandbox",
		Template:      "test",
		WorkspaceMode: "direct",
	}

	template := &config.Template{
		Name:    "test",
		Network: "full",
	}

	result := GenerateSkillFiles(metadata, template, nil)

	if len(result) != 0 {
		t.Errorf("expected no skill files for direct mode with no project info, got %d", len(result))
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

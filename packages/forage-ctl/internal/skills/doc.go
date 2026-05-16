// Package skills provides project analysis and skills file generation.
//
// This package analyzes project workspaces to detect languages, frameworks,
// and build systems, then generates context-aware instructions for AI agents
// running in sandboxes.
//
// # Project Analysis
//
// The Analyzer examines a workspace to detect:
//   - Project type (Go, Rust, Python, Node, TypeScript, Nix)
//   - Build system (go, cargo, npm, pnpm, yarn, bun, poetry, hatch)
//   - Frameworks (gin, echo, react, nextjs, django, flask, etc.)
//   - Version control (git, jj)
//   - CI configuration
//
// Usage:
//
//	analyzer := skills.NewAnalyzer("/path/to/workspace")
//	info := analyzer.Analyze()
//
// # Skills Generation
//
// GenerateSkills creates a markdown document (CLAUDE.md) with:
//   - Environment information (sandbox name, template, workspace mode)
//   - Project-specific build/test commands
//   - Version control instructions (git or jj)
//   - Network access documentation
//   - Available agents and their authentication
//   - General guidelines for working in the sandbox
//
// Usage:
//
//	content := skills.GenerateSkills(metadata, template, projectInfo)
//
// The generated skills file is injected into the container at /workspace/CLAUDE.md
// after the sandbox starts.
package skills

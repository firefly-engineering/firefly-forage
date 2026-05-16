# ADR 003: Skill Injection Strategy

## Status

Accepted

## Context

AI coding agents (like Claude Code) can be customized via "skills" files that provide context about the project and environment. We need to inform agents about:

1. The sandbox environment (network restrictions, available tools)
2. Version control system in use (jj vs git)
3. Project-specific information (language, build system, frameworks)
4. Workspace constraints (ephemeral container, persistent workspace)

The challenge is injecting this information without conflicting with existing project documentation.

## Decision

### Two-tier skill injection

1. **Project analysis**: Detect project type, build system, and frameworks
2. **Generated skills file**: Write a combined `CLAUDE.md` to the workspace

The `skills` package:
- Analyzes the workspace to detect project characteristics
- Generates context-aware instructions
- Includes sandbox-specific information

```go
analyzer := skills.NewAnalyzer(workspacePath)
projectInfo := analyzer.Analyze()
content := skills.GenerateSkills(metadata, template, projectInfo)
```

### Content structure

The generated file includes:
- Environment section (sandbox name, template, workspace mode)
- Project section (type, build system, frameworks, common commands)
- Version control section (jj or git instructions)
- Network section (access level and restrictions)
- Available agents section
- Guidelines section

### File location

Skills are written to `/workspace/CLAUDE.md` inside the container. This is a simple approach that works well in practice.

## Consequences

### Positive

- **Context-aware**: Agents know about project-specific tools and conventions
- **Consistent**: All sandboxes get appropriate environment documentation
- **Discoverable**: Agents find skills in expected location
- **Dynamic**: Content adapts based on project analysis

### Negative

- **May conflict**: If workspace has existing CLAUDE.md, it gets overwritten
- **Maintenance**: Need to keep framework/tool detection up to date
- **Not perfect**: Detection heuristics may miss some project types

## Alternatives Considered

### 1. Separate skills directory

Write to `.claude/forage-skills.md` to avoid conflicts.

**Not implemented because**: Claude Code loads instructions from `CLAUDE.md` primarily. A separate file would need explicit configuration to be discovered.

### 2. No injection

Let users manually configure agent skills.

**Rejected because**: Defeats the purpose of automation. Users would need to repeat configuration for every sandbox.

### 3. Template-only skills

Only include skills defined in the template, no project analysis.

**Rejected because**: Misses valuable context about the actual project being worked on.

### 4. Append to existing CLAUDE.md

If a CLAUDE.md exists, append forage skills to it.

**Considered but not implemented**: Could lead to messy documents with duplicate information. The current approach prioritizes clean, consistent output.

## Future Considerations

- Add option to preserve existing CLAUDE.md content
- Support for other agent configuration formats
- User-defined skill templates

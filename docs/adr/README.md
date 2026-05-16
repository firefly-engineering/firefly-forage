# Architecture Decision Records

This directory contains Architecture Decision Records (ADRs) for Firefly Forage.

ADRs document significant technical decisions, including the context, decision, and consequences. They complement the [DESIGN.md](/DESIGN.md) document which provides a comprehensive overview of the system architecture.

## Index

| ADR | Title | Status |
|-----|-------|--------|
| [001](001-container-runtime-abstraction.md) | Container Runtime Abstraction | Accepted |
| [002](002-ssh-based-sandbox-access.md) | SSH-Based Sandbox Access | Accepted |
| [003](003-skill-injection-strategy.md) | Skill Injection Strategy | Accepted |
| [004](004-workspace-modes.md) | Workspace Modes (Direct, JJ, Git Worktree) | Accepted |

## ADR Format

Each ADR follows this structure:

1. **Title**: Short descriptive name
2. **Status**: Proposed, Accepted, Deprecated, Superseded
3. **Context**: The circumstances and constraints
4. **Decision**: What we decided to do
5. **Consequences**: The results, both positive and negative
6. **Alternatives Considered**: Other options we evaluated

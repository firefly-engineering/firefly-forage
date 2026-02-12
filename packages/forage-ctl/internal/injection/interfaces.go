package injection

import (
	"context"
)

// MountContributor can contribute filesystem mounts to a container.
type MountContributor interface {
	ContributeMounts(ctx context.Context, req *MountRequest) ([]Mount, error)
}

// PackageContributor can contribute packages to install in the container.
type PackageContributor interface {
	ContributePackages(ctx context.Context) ([]Package, error)
}

// EnvVarContributor can contribute environment variables to the container.
type EnvVarContributor interface {
	ContributeEnvVars(ctx context.Context, req *EnvVarRequest) ([]EnvVar, error)
}

// PromptContributor can contribute to agent system prompts.
type PromptContributor interface {
	ContributePromptFragments(ctx context.Context) ([]PromptFragment, error)
}

// GeneratedFileContributor can contribute dynamically generated files
// (e.g., permissions policy, skills, system prompts).
type GeneratedFileContributor interface {
	ContributeGeneratedFiles(ctx context.Context, req *GeneratedFileRequest) ([]GeneratedFile, error)
}

// TmpfilesContributor can contribute systemd tmpfiles rules.
type TmpfilesContributor interface {
	ContributeTmpfilesRules(ctx context.Context, req *TmpfilesRequest) ([]string, error)
}

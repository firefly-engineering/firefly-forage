package injection

import (
	"context"
	"sort"
)

// Collector gathers contributions from various backends.
type Collector struct{}

// NewCollector creates a new Collector.
func NewCollector() *Collector {
	return &Collector{}
}

// Contributions is the aggregated result from all contributors.
type Contributions struct {
	Mounts           []Mount
	EnvVars          []EnvVar
	Packages         []Package
	InitCommands     []string
	TmpfilesRules    []string
	PromptFragments  []PromptFragment
}

// CollectionSources holds all the backends that might contribute.
type CollectionSources struct {
	// Contributors is the list of all potential contributors.
	// Each will be checked via interface assertions.
	Contributors []any

	// Request contexts for different contribution types
	MountRequest         *MountRequest
	EnvVarRequest        *EnvVarRequest
	InitCommandRequest   *InitCommandRequest
	GeneratedFileRequest *GeneratedFileRequest
	TmpfilesRequest      *TmpfilesRequest

	// GeneratedFileMounter handles converting generated files to mounts.
	// If nil, generated files will be skipped.
	GeneratedFileMounter interface {
		MountGeneratedFile(ctx context.Context, sandboxName string, file GeneratedFile) (Mount, error)
	}
	SandboxName string
}

// Collect queries all sources for their contributions.
func (c *Collector) Collect(ctx context.Context, sources CollectionSources) (*Contributions, error) {
	result := &Contributions{}

	for _, src := range sources.Contributors {
		// Mounts
		if mc, ok := src.(MountContributor); ok {
			mounts, err := mc.ContributeMounts(ctx, sources.MountRequest)
			if err != nil {
				return nil, err
			}
			result.Mounts = append(result.Mounts, mounts...)
		}

		// Packages
		if pc, ok := src.(PackageContributor); ok {
			pkgs, err := pc.ContributePackages(ctx)
			if err != nil {
				return nil, err
			}
			result.Packages = append(result.Packages, pkgs...)
		}

		// Environment variables
		if ec, ok := src.(EnvVarContributor); ok {
			envVars, err := ec.ContributeEnvVars(ctx, sources.EnvVarRequest)
			if err != nil {
				return nil, err
			}
			result.EnvVars = append(result.EnvVars, envVars...)
		}

		// Init commands
		if ic, ok := src.(InitCommandContributor); ok {
			cmds, err := ic.ContributeInitCommands(ctx, sources.InitCommandRequest)
			if err != nil {
				return nil, err
			}
			result.InitCommands = append(result.InitCommands, cmds...)
		}

		// Tmpfiles rules
		if tc, ok := src.(TmpfilesContributor); ok {
			rules, err := tc.ContributeTmpfilesRules(ctx, sources.TmpfilesRequest)
			if err != nil {
				return nil, err
			}
			result.TmpfilesRules = append(result.TmpfilesRules, rules...)
		}

		// Prompt fragments
		if pc, ok := src.(PromptContributor); ok {
			fragments, err := pc.ContributePromptFragments(ctx)
			if err != nil {
				return nil, err
			}
			result.PromptFragments = append(result.PromptFragments, fragments...)
		}

		// Generated files -> converted to mounts
		if gfc, ok := src.(GeneratedFileContributor); ok && sources.GeneratedFileMounter != nil {
			files, err := gfc.ContributeGeneratedFiles(ctx, sources.GeneratedFileRequest)
			if err != nil {
				return nil, err
			}
			for _, file := range files {
				mount, err := sources.GeneratedFileMounter.MountGeneratedFile(ctx, sources.SandboxName, file)
				if err != nil {
					return nil, err
				}
				result.Mounts = append(result.Mounts, mount)
			}
		}
	}

	// Sort prompt fragments by section and priority
	sort.Slice(result.PromptFragments, func(i, j int) bool {
		if result.PromptFragments[i].Section != result.PromptFragments[j].Section {
			return result.PromptFragments[i].Section < result.PromptFragments[j].Section
		}
		return result.PromptFragments[i].Priority < result.PromptFragments[j].Priority
	})

	// Deduplicate tmpfiles rules (keep first occurrence)
	result.TmpfilesRules = dedupeStrings(result.TmpfilesRules)

	return result, nil
}

// dedupeStrings removes duplicates while preserving order.
func dedupeStrings(items []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(items))
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

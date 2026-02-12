package runtime

import (
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
)

// buildContainerReverseMap builds a mapping from container name to sandbox name
// by loading all sandbox metadata files. This is used by List operations to
// identify which running containers belong to forage sandboxes.
func buildContainerReverseMap(sandboxesDir string) map[string]string {
	result := make(map[string]string)
	if sandboxesDir == "" {
		return result
	}

	sandboxes, err := config.ListSandboxes(sandboxesDir)
	if err != nil {
		logging.Debug("failed to list sandboxes for reverse map", "error", err)
		return result
	}

	for _, meta := range sandboxes {
		result[meta.ResolvedContainerName()] = meta.Name
	}
	return result
}

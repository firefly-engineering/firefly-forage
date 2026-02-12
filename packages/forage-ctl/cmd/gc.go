package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/sandbox"
)

var gcForce bool

var gcCmd = &cobra.Command{
	Use:   "gc",
	Short: "Garbage collect orphaned sandbox resources",
	Long: `Reconciles disk state with runtime state and removes orphaned resources.

Without --force, prints what would be cleaned (dry run).
With --force, actually removes orphaned files and destroys orphaned containers.

Detects:
  - Orphaned files: sandbox files on disk with no matching container
  - Orphaned containers: containers with no matching metadata on disk
  - Stale metadata: metadata files for sandboxes whose container no longer exists`,
	RunE: runGC,
}

func init() {
	gcCmd.Flags().BoolVar(&gcForce, "force", false, "Actually remove orphaned resources (default is dry run)")
	rootCmd.AddCommand(gcCmd)
}

// orphanedContainer represents a container in the runtime with no metadata on disk.
type orphanedContainer struct {
	name        string // sandbox name from runtime
	recoveredBy string // how the sandbox name was identified (empty if from runtime directly)
}

// gcResult tracks what gc found and would/did clean up.
type gcResult struct {
	orphanedSandboxNames []string            // sandbox names with files on disk but no container
	orphanedContainers   []orphanedContainer // containers in runtime but no metadata on disk
}

func (r *gcResult) empty() bool {
	return len(r.orphanedSandboxNames) == 0 && len(r.orphanedContainers) == 0
}

func runGC(cmd *cobra.Command, args []string) error {
	p := paths()
	rt := getRuntime()
	ctx := context.Background()

	// 1. Collect sandbox names from disk files
	diskNames, err := sandboxNamesFromDisk(p.SandboxesDir)
	if err != nil {
		return fmt.Errorf("failed to scan sandboxes directory: %w", err)
	}

	// 2. Collect container names from runtime
	containers, err := rt.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	containerSet := make(map[string]bool)
	for _, c := range containers {
		containerSet[c.Name] = true
	}

	// 3. Collect valid metadata names
	metadataList, err := config.ListSandboxes(p.SandboxesDir)
	if err != nil {
		return fmt.Errorf("failed to list sandbox metadata: %w", err)
	}

	metadataSet := make(map[string]*config.SandboxMetadata)
	for _, m := range metadataList {
		metadataSet[m.Name] = m
	}

	// 4. Find orphans
	result := &gcResult{}

	// Orphaned disk files: sandbox name found on disk but no matching container
	for name := range diskNames {
		if !containerSet[name] {
			result.orphanedSandboxNames = append(result.orphanedSandboxNames, name)
		}
	}

	// Orphaned containers: in runtime but no metadata on disk
	for name := range containerSet {
		if _, ok := metadataSet[name]; !ok {
			result.orphanedContainers = append(result.orphanedContainers, orphanedContainer{name: name})
		}
	}

	// 5. Report or act
	if result.empty() {
		logInfo("No orphaned resources found")
		return nil
	}

	if !gcForce {
		printGCDryRun(result)
		return nil
	}

	return executeGC(ctx, result, p, rt, metadataSet)
}

// sandboxNamesFromDisk scans the sandboxes directory and returns a set of
// sandbox names extracted from filenames. It recognizes:
//   - <name>.json (metadata)
//   - <name>.nix (config)
//   - <name>.skills.md (skills)
//   - <name>.*-permissions.json (permissions)
func sandboxNamesFromDisk(sandboxesDir string) (map[string]bool, error) {
	entries, err := os.ReadDir(sandboxesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	names := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := extractSandboxName(entry.Name())
		if name == "" {
			continue
		}

		if config.ValidateSandboxName(name) == nil {
			names[name] = true
		}
	}

	return names, nil
}

// extractSandboxName extracts the sandbox name from a known file pattern.
// Returns empty string if the filename doesn't match any known pattern.
func extractSandboxName(filename string) string {
	// <name>.*-permissions.json (e.g. "test.claude-permissions.json")
	if strings.HasSuffix(filename, "-permissions.json") {
		// Find the first dot to split sandbox name from agent-permissions suffix
		idx := strings.Index(filename, ".")
		if idx > 0 {
			return filename[:idx]
		}
		return ""
	}

	// <name>.skills.md
	if name, ok := strings.CutSuffix(filename, ".skills.md"); ok {
		return name
	}

	// <name>.nix
	if name, ok := strings.CutSuffix(filename, ".nix"); ok {
		return name
	}

	// <name>.json (metadata - must not contain dots before .json)
	if name, ok := strings.CutSuffix(filename, ".json"); ok {
		if !strings.Contains(name, ".") {
			return name
		}
	}

	return ""
}

func printGCDryRun(result *gcResult) {
	fmt.Println("Dry run (use --force to actually clean up):")
	fmt.Println()

	if len(result.orphanedSandboxNames) > 0 {
		fmt.Println("Orphaned sandbox files (no matching container):")
		for _, name := range result.orphanedSandboxNames {
			fmt.Printf("  %s\n", name)
		}
		fmt.Println()
	}

	if len(result.orphanedContainers) > 0 {
		fmt.Println("Orphaned containers (no matching metadata):")
		for _, oc := range result.orphanedContainers {
			if oc.recoveredBy != "" {
				fmt.Printf("  %s (identified via %s)\n", oc.name, oc.recoveredBy)
			} else {
				fmt.Printf("  %s\n", oc.name)
			}
		}
		fmt.Println()
	}
}

func executeGC(ctx context.Context, result *gcResult, p *config.Paths, rt interface {
	Destroy(ctx context.Context, name string) error
}, metadataSet map[string]*config.SandboxMetadata) error {
	// Clean up orphaned sandbox files
	for _, name := range result.orphanedSandboxNames {
		logInfo("Cleaning up orphaned sandbox: %s", name)

		// If we have valid metadata, use Cleanup() for proper VCS unwinding
		if meta, ok := metadataSet[name]; ok {
			opts := sandbox.DefaultCleanupOptions()
			opts.DestroyContainer = false // container doesn't exist
			sandbox.Cleanup(meta, p, opts, nil)
		} else {
			// No valid metadata -- remove files manually
			removeOrphanedFiles(name, p)
		}

		logging.Debug("cleaned up orphaned sandbox", "name", name)
	}

	// Destroy orphaned containers
	for _, oc := range result.orphanedContainers {
		if oc.recoveredBy != "" {
			logInfo("Destroying orphaned container: %s (identified via %s)", oc.name, oc.recoveredBy)
		} else {
			logInfo("Destroying orphaned container: %s", oc.name)
		}
		if err := rt.Destroy(ctx, oc.name); err != nil {
			logWarning("Failed to destroy container %s: %v", oc.name, err)
		} else {
			logging.Debug("destroyed orphaned container", "name", oc.name)
		}
	}

	logSuccess("Garbage collection complete")
	return nil
}

// removeOrphanedFiles removes all files associated with a sandbox name
// when no valid metadata is available.
func removeOrphanedFiles(name string, p *config.Paths) {
	// Remove known file patterns
	patterns := []string{
		filepath.Join(p.SandboxesDir, name+".json"),
		filepath.Join(p.SandboxesDir, name+".nix"),
		filepath.Join(p.SandboxesDir, name+".skills.md"),
	}

	for _, path := range patterns {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			logging.Warn("failed to remove file", "path", path, "error", err)
		}
	}

	// Glob for permissions files
	permPattern := filepath.Join(p.SandboxesDir, name+".*-permissions.json")
	matches, _ := filepath.Glob(permPattern)
	for _, match := range matches {
		if err := os.Remove(match); err != nil && !os.IsNotExist(err) {
			logging.Warn("failed to remove permissions file", "path", match, "error", err)
		}
	}

	// Remove secrets directory
	secretsPath := filepath.Join(p.SecretsDir, name)
	if err := os.RemoveAll(secretsPath); err != nil {
		logging.Warn("failed to remove secrets directory", "path", secretsPath, "error", err)
	}

	// Remove workspace directory
	workspacePath := filepath.Join(p.WorkspacesDir, name)
	if err := os.RemoveAll(workspacePath); err != nil {
		logging.Warn("failed to remove workspace directory", "path", workspacePath, "error", err)
	}
}

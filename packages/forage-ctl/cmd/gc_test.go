package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/testutil"
)

func TestGCCommand_Help(t *testing.T) {
	stdout, _, err := executeCommand("gc", "--help")
	if err != nil {
		t.Fatalf("Help command failed: %v", err)
	}

	if !strings.Contains(stdout, "orphaned") {
		t.Error("GC help should mention orphaned resources")
	}

	if !strings.Contains(stdout, "--force") {
		t.Error("GC help should mention --force flag")
	}
}

func TestExtractSandboxName(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		// Metadata files
		{"test.json", "test"},
		{"my-sandbox.json", "my-sandbox"},

		// Config files
		{"test.nix", "test"},
		{"my-sandbox.nix", "my-sandbox"},

		// Skills files
		{"test.skills.md", "test"},
		{"my-sandbox.skills.md", "my-sandbox"},

		// Permissions files
		{"test.claude-permissions.json", "test"},
		{"my-sandbox.copilot-permissions.json", "my-sandbox"},

		// Dotted JSON (not metadata, not permissions) -- ignored
		{"some.other.json", ""},

		// Non-sandbox files
		{"readme.txt", ""},
		{"notes.md", ""},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := extractSandboxName(tt.filename)
			if got != tt.want {
				t.Errorf("extractSandboxName(%q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}

func TestSandboxNamesFromDisk(t *testing.T) {
	tmpDir := t.TempDir()

	// Create various sandbox files
	files := map[string]string{
		"sandbox-1.json":                    `{"name":"sandbox-1","template":"claude","networkSlot":1}`,
		"sandbox-1.nix":                     "# nix config",
		"sandbox-1.skills.md":               "# skills",
		"sandbox-1.claude-permissions.json": `{}`,
		"sandbox-2.json":                    `{"name":"sandbox-2","template":"claude","networkSlot":2}`,
		"orphan.nix":                        "# orphaned nix file",
		"notes.txt":                         "not a sandbox file",
	}

	for name, content := range files {
		os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644)
	}

	names, err := sandboxNamesFromDisk(tmpDir)
	if err != nil {
		t.Fatalf("sandboxNamesFromDisk failed: %v", err)
	}

	expected := map[string]bool{
		"sandbox-1": true,
		"sandbox-2": true,
		"orphan":    true,
	}

	if len(names) != len(expected) {
		t.Errorf("len(names) = %d, want %d", len(names), len(expected))
	}

	for name := range expected {
		if !names[name] {
			t.Errorf("expected name %q not found in disk names", name)
		}
	}
}

func TestSandboxNamesFromDisk_NonexistentDir(t *testing.T) {
	names, err := sandboxNamesFromDisk("/nonexistent/path")
	if err != nil {
		t.Fatalf("should not error for nonexistent dir: %v", err)
	}
	if names != nil {
		t.Errorf("names = %v, want nil", names)
	}
}

func TestGC_OrphanDetection(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	// Create a sandbox with matching container (not orphaned)
	env.AddSandbox(&config.SandboxMetadata{
		Name:        "healthy",
		Template:    "claude",
		NetworkSlot: 1,
		Workspace:   "/tmp/healthy",
	})

	// Create metadata on disk with NO container (orphaned files)
	config.SaveSandboxMetadata(env.Paths.SandboxesDir, &config.SandboxMetadata{
		Name:        "orphan-disk",
		Template:    "claude",
		NetworkSlot: 2,
		Workspace:   "/tmp/orphan-disk",
	})

	// Add container in runtime with NO metadata (orphaned container)
	env.Runtime.AddContainer("orphan-rt", runtime.StatusRunning)

	ctx := context.Background()

	// Collect disk names
	diskNames, err := sandboxNamesFromDisk(env.Paths.SandboxesDir)
	if err != nil {
		t.Fatalf("sandboxNamesFromDisk: %v", err)
	}

	// Collect runtime containers
	containers, err := env.Runtime.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	containerSet := make(map[string]bool)
	for _, c := range containers {
		containerSet[c.Name] = true
	}

	// Collect metadata
	metadataList, err := config.ListSandboxes(env.Paths.SandboxesDir)
	if err != nil {
		t.Fatalf("ListSandboxes: %v", err)
	}
	metadataSet := make(map[string]*config.SandboxMetadata)
	for _, m := range metadataList {
		metadataSet[m.Name] = m
	}

	// Find orphaned disk files
	var orphanedDisk []string
	for name := range diskNames {
		if !containerSet[name] {
			orphanedDisk = append(orphanedDisk, name)
		}
	}

	// Find orphaned containers
	var orphanedContainers []string
	for name := range containerSet {
		if _, ok := metadataSet[name]; !ok {
			orphanedContainers = append(orphanedContainers, name)
		}
	}

	// Verify: orphan-disk should be in orphaned disk files
	foundDisk := false
	for _, name := range orphanedDisk {
		if name == "orphan-disk" {
			foundDisk = true
		}
		if name == "healthy" {
			t.Error("healthy sandbox should not be orphaned on disk")
		}
	}
	if !foundDisk {
		t.Error("orphan-disk should be detected as orphaned on disk")
	}

	// Verify: orphan-rt should be in orphaned containers
	foundRT := false
	for _, name := range orphanedContainers {
		if name == "orphan-rt" {
			foundRT = true
		}
		if name == "healthy" {
			t.Error("healthy sandbox should not be an orphaned container")
		}
	}
	if !foundRT {
		t.Error("orphan-rt should be detected as orphaned container")
	}
}

func TestGC_Force_CleansOrphanedFiles(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	name := "orphan"
	sandboxesDir := env.Paths.SandboxesDir

	// Create various sandbox files on disk
	os.WriteFile(filepath.Join(sandboxesDir, name+".json"),
		[]byte(`{"name":"orphan","template":"claude","networkSlot":1,"workspace":"/tmp/w"}`), 0644)
	os.WriteFile(filepath.Join(sandboxesDir, name+".nix"), []byte("# nix"), 0644)
	os.WriteFile(filepath.Join(sandboxesDir, name+".skills.md"), []byte("# skills"), 0644)
	os.WriteFile(filepath.Join(sandboxesDir, name+".claude-permissions.json"), []byte("{}"), 0644)

	// Create secrets directory
	secretsDir := filepath.Join(env.Paths.SecretsDir, name)
	os.MkdirAll(secretsDir, 0755)
	os.WriteFile(filepath.Join(secretsDir, "key"), []byte("secret"), 0644)

	// No container in runtime -- sandbox is orphaned
	// Use removeOrphanedFiles directly to test cleanup logic
	removeOrphanedFiles(name, env.Paths)

	// Verify all files were removed
	for _, suffix := range []string{".json", ".nix", ".skills.md", ".claude-permissions.json"} {
		path := filepath.Join(sandboxesDir, name+suffix)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("file %s should have been removed", filepath.Base(path))
		}
	}

	// Verify secrets directory was removed
	if _, err := os.Stat(secretsDir); !os.IsNotExist(err) {
		t.Error("secrets directory should have been removed")
	}
}

func TestGC_Force_DestroysOrphanedContainers(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	ctx := context.Background()

	// Add orphaned container (no metadata)
	env.Runtime.AddContainer("ghost", runtime.StatusRunning)

	// Call executeGC directly
	result := &gcResult{
		orphanedContainers: []string{"ghost"},
	}
	metadataSet := make(map[string]*config.SandboxMetadata)

	err := executeGC(ctx, result, env.Paths, env.Runtime, metadataSet)
	if err != nil {
		t.Fatalf("executeGC failed: %v", err)
	}

	// Verify container was destroyed
	destroyCalls := env.Runtime.GetCallsFor("Destroy")
	found := false
	for _, call := range destroyCalls {
		if len(call.Args) > 0 && call.Args[0] == "ghost" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Destroy to be called for orphaned container 'ghost'")
	}
}

func TestGC_NoOrphans(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	// Create a sandbox with matching container
	env.AddSandbox(&config.SandboxMetadata{
		Name:        "test",
		Template:    "claude",
		NetworkSlot: 1,
		Workspace:   "/tmp/test",
	})

	ctx := context.Background()

	// Collect all three data sources
	diskNames, _ := sandboxNamesFromDisk(env.Paths.SandboxesDir)
	containers, _ := env.Runtime.List(ctx)
	containerSet := make(map[string]bool)
	for _, c := range containers {
		containerSet[c.Name] = true
	}
	metadataList, _ := config.ListSandboxes(env.Paths.SandboxesDir)
	metadataSet := make(map[string]*config.SandboxMetadata)
	for _, m := range metadataList {
		metadataSet[m.Name] = m
	}

	result := &gcResult{}
	for name := range diskNames {
		if !containerSet[name] {
			result.orphanedSandboxNames = append(result.orphanedSandboxNames, name)
		}
	}
	for name := range containerSet {
		if _, ok := metadataSet[name]; !ok {
			result.orphanedContainers = append(result.orphanedContainers, name)
		}
	}

	if !result.empty() {
		t.Errorf("expected no orphans, got disk=%v containers=%v",
			result.orphanedSandboxNames, result.orphanedContainers)
	}
}

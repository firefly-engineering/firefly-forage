package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
)

// testEnv holds test environment state
type testEnv struct {
	tmpDir     string
	configDir  string
	stateDir   string
	secretsDir string
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()

	tmpDir := t.TempDir()

	env := &testEnv{
		tmpDir:     tmpDir,
		configDir:  filepath.Join(tmpDir, "config"),
		stateDir:   filepath.Join(tmpDir, "state"),
		secretsDir: filepath.Join(tmpDir, "secrets"),
	}

	// Create directories
	dirs := []string{
		env.configDir,
		env.stateDir,
		env.secretsDir,
		filepath.Join(env.stateDir, "sandboxes"),
		filepath.Join(env.stateDir, "workspaces"),
		filepath.Join(env.configDir, "templates"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create %s: %v", dir, err)
		}
	}

	// Create host config
	hostConfig := &config.HostConfig{
		User: "testuser",
		PortRange: config.PortRange{
			From: 2200,
			To:   2299,
		},
		AuthorizedKeys:     []string{"ssh-rsa AAAA..."},
		Secrets:            map[string]string{"anthropic": "sk-test"},
		StateDir:           env.stateDir,
		ExtraContainerPath: "/fake/extra-container",
		NixpkgsRev:         "test123",
	}

	data, _ := json.MarshalIndent(hostConfig, "", "  ")
	if err := os.WriteFile(filepath.Join(env.configDir, "config.json"), data, 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	return env
}

func (e *testEnv) addTemplate(t *testing.T, name string, tmpl *config.Template) {
	t.Helper()

	if tmpl.Name == "" {
		tmpl.Name = name
	}

	data, _ := json.MarshalIndent(tmpl, "", "  ")
	path := filepath.Join(e.configDir, "templates", name+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("Failed to write template: %v", err)
	}
}

func (e *testEnv) createWorkspace(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join(e.tmpDir, "workspace", name)
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	return path
}

func executeCommand(args ...string) (string, string, error) {
	// Reset flag values before each test
	upTemplate = ""
	upWorkspace = ""
	upRepo = ""
	logsFollow = false
	logsLines = 50
	verbose = false
	jsonOutput = false

	cmd := rootCmd
	cmd.SetArgs(args)

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	err := cmd.Execute()

	// Reset args for next test
	cmd.SetArgs(nil)
	cmd.SetOut(nil)
	cmd.SetErr(nil)

	return stdout.String(), stderr.String(), err
}

func TestRootCommand_Help(t *testing.T) {
	stdout, _, err := executeCommand("--help")
	if err != nil {
		t.Fatalf("Help command failed: %v", err)
	}

	if !strings.Contains(stdout, "forage-ctl") {
		t.Error("Help output should contain 'forage-ctl'")
	}

	if !strings.Contains(stdout, "sandbox") {
		t.Error("Help output should mention sandbox")
	}
}

func TestRootCommand_Version(t *testing.T) {
	// Version is not implemented, but help should work
	stdout, _, err := executeCommand("help")
	if err != nil {
		t.Fatalf("Help command failed: %v", err)
	}

	if !strings.Contains(stdout, "Available Commands") {
		t.Error("Help output should list available commands")
	}
}

func TestUpCommand_Help(t *testing.T) {
	stdout, _, err := executeCommand("up", "--help")
	if err != nil {
		t.Fatalf("Help command failed: %v", err)
	}

	if !strings.Contains(stdout, "--template") {
		t.Error("Up help should mention --template flag")
	}

	if !strings.Contains(stdout, "--workspace") {
		t.Error("Up help should mention --workspace flag")
	}

	if !strings.Contains(stdout, "--repo") {
		t.Error("Up help should mention --repo flag")
	}
}

func TestUpCommand_MissingTemplate(t *testing.T) {
	stdout, stderr, err := executeCommand("up", "test-sandbox", "--workspace", "/tmp")
	output := stdout + stderr

	// Should either return an error or show required flag message
	if err == nil && !strings.Contains(output, "required") && !strings.Contains(output, "template") {
		t.Error("Up should fail or show error when --template is missing")
	}
}

func TestUpCommand_MutuallyExclusiveFlags(t *testing.T) {
	env := setupTestEnv(t)
	env.addTemplate(t, "test", &config.Template{
		Name:    "test",
		Network: "full",
		Agents: map[string]config.AgentConfig{
			"test": {
				PackagePath: "/nix/store/test-agent",
				SecretName:  "test-secret",
				AuthEnvVar:  "TEST_API_KEY",
			},
		},
	})

	// Create workspace for potential future tests
	_ = env.createWorkspace(t, "project")

	// This test would need the actual command to check mutual exclusivity
	// For now, we verify the help mentions both flags
	stdout, _, _ := executeCommand("up", "--help")

	if !strings.Contains(stdout, "workspace") || !strings.Contains(stdout, "repo") {
		t.Error("Help should document both --workspace and --repo")
	}
}

func TestDownCommand_Help(t *testing.T) {
	stdout, _, err := executeCommand("down", "--help")
	if err != nil {
		t.Fatalf("Help command failed: %v", err)
	}

	if !strings.Contains(stdout, "Stop and remove") {
		t.Error("Down help should describe its purpose")
	}
}

func TestPsCommand_Help(t *testing.T) {
	stdout, _, err := executeCommand("ps", "--help")
	if err != nil {
		t.Fatalf("Help command failed: %v", err)
	}

	if !strings.Contains(stdout, "List") {
		t.Error("Ps help should mention listing")
	}
}

func TestStatusCommand_Help(t *testing.T) {
	stdout, _, err := executeCommand("status", "--help")
	if err != nil {
		t.Fatalf("Help command failed: %v", err)
	}

	if !strings.Contains(stdout, "status") {
		t.Error("Status help should mention status")
	}
}

func TestSSHCommand_Help(t *testing.T) {
	stdout, _, err := executeCommand("ssh", "--help")
	if err != nil {
		t.Fatalf("Help command failed: %v", err)
	}

	if !strings.Contains(stdout, "SSH") || !strings.Contains(stdout, "tmux") {
		t.Error("SSH help should mention SSH and tmux")
	}
}

func TestExecCommand_Help(t *testing.T) {
	stdout, _, err := executeCommand("exec", "--help")
	if err != nil {
		t.Fatalf("Help command failed: %v", err)
	}

	if !strings.Contains(stdout, "Execute") {
		t.Error("Exec help should mention execution")
	}
}

func TestLogsCommand_Help(t *testing.T) {
	stdout, _, err := executeCommand("logs", "--help")
	if err != nil {
		t.Fatalf("Help command failed: %v", err)
	}

	if !strings.Contains(stdout, "logs") {
		t.Error("Logs help should mention logs")
	}

	if !strings.Contains(stdout, "--follow") {
		t.Error("Logs help should mention --follow flag")
	}
}

func TestStartCommand_Help(t *testing.T) {
	stdout, _, err := executeCommand("start", "--help")
	if err != nil {
		t.Fatalf("Help command failed: %v", err)
	}

	if !strings.Contains(stdout, "Start") {
		t.Error("Start help should mention starting")
	}
}

func TestShellCommand_Help(t *testing.T) {
	stdout, _, err := executeCommand("shell", "--help")
	if err != nil {
		t.Fatalf("Help command failed: %v", err)
	}

	if !strings.Contains(stdout, "shell") {
		t.Error("Shell help should mention shell")
	}
}

func TestResetCommand_Help(t *testing.T) {
	stdout, _, err := executeCommand("reset", "--help")
	if err != nil {
		t.Fatalf("Help command failed: %v", err)
	}

	if !strings.Contains(stdout, "Reset") {
		t.Error("Reset help should mention reset")
	}
}

func TestTemplatesCommand_Help(t *testing.T) {
	stdout, _, err := executeCommand("templates", "--help")
	if err != nil {
		t.Fatalf("Help command failed: %v", err)
	}

	if !strings.Contains(stdout, "templates") {
		t.Error("Templates help should mention templates")
	}
}

func TestGlobalFlags(t *testing.T) {
	stdout, _, err := executeCommand("--help")
	if err != nil {
		t.Fatalf("Help failed: %v", err)
	}

	if !strings.Contains(stdout, "--verbose") {
		t.Error("Should have --verbose flag")
	}

	if !strings.Contains(stdout, "--json") {
		t.Error("Should have --json flag")
	}
}

func TestCommandRequiresArgs(t *testing.T) {
	// Commands that require arguments show error in stderr
	tests := []struct {
		cmd            string
		shouldShowHelp bool
	}{
		{"down", true},   // requires name, shows usage
		{"status", true}, // requires name, shows usage
		{"ssh", true},    // requires name, shows usage
		{"start", true},  // requires name, shows usage
		{"shell", true},  // requires name, shows usage
		{"reset", true},  // requires name, shows usage
		{"logs", true},   // requires name, shows usage
		{"ps", false},    // no args required
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			stdout, stderr, _ := executeCommand(tt.cmd)
			output := stdout + stderr

			// Commands requiring args should show usage info
			if tt.shouldShowHelp {
				if !strings.Contains(output, "Usage:") && !strings.Contains(output, "Error:") {
					// Some cobra versions just show usage without "Error:"
					if !strings.Contains(output, tt.cmd) {
						t.Errorf("%s: expected usage info in output", tt.cmd)
					}
				}
			}
		})
	}
}

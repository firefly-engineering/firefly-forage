package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/testutil"
)

func TestCreator_Create_InvalidName(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	env.AddTemplate("test", testutil.DefaultTemplate())

	creator := &Creator{
		paths:      env.Paths,
		hostConfig: env.HostConfig,
		rt:         env.Runtime,
	}

	// Test invalid sandbox names
	invalidNames := []string{
		"",                  // empty
		"../escape",         // path traversal
		"My-Project",        // uppercase
		"has spaces",        // spaces
		"-starts-with-dash", // starts with dash
		"has;semicolon",     // special characters
	}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			_, err := creator.Create(context.Background(), CreateOptions{
				Name:     name,
				Template: "test",
				RepoPath: env.TmpDir,
				Direct:   true,
			})
			if err == nil {
				t.Errorf("Create(%q) should have failed with invalid name", name)
			}
		})
	}
}

func TestCreator_Create_ValidName(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	// IMPORTANT: Set mock runtime as global runtime so runtime.Create() uses it
	runtime.SetGlobal(env.Runtime)
	defer runtime.SetGlobal(nil)

	env.AddTemplate("test", testutil.DefaultTemplate())

	// Create a workspace directory
	workspacePath := env.CreateWorkspace("myproject")

	creator := &Creator{
		paths:      env.Paths,
		hostConfig: env.HostConfig,
		rt:         env.Runtime,
	}

	// Test valid sandbox name
	result, err := creator.Create(context.Background(), CreateOptions{
		Name:     "myproject",
		Template: "test",
		RepoPath: workspacePath,
		Direct:   true,
	})

	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	if result.Name != "myproject" {
		t.Errorf("Name = %q, want %q", result.Name, "myproject")
	}

	// Verify ContainerIP is derived from NetworkSlot
	if result.ContainerIP == "" {
		t.Error("ContainerIP should not be empty")
	}
	if result.Metadata.NetworkSlot < 1 || result.Metadata.NetworkSlot > 254 {
		t.Errorf("NetworkSlot %d not in valid range [1, 254]",
			result.Metadata.NetworkSlot)
	}

	// Verify sandbox metadata was saved
	if !env.SandboxExists("myproject") {
		t.Error("Sandbox metadata was not saved")
	}

	// Verify runtime.Create was called
	if _, exists := env.Runtime.Containers["myproject"]; !exists {
		t.Error("Container was not created via runtime")
	}
}

func TestCreator_Create_DuplicateName(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	env.AddTemplate("test", testutil.DefaultTemplate())

	// Create an existing sandbox
	env.AddSandbox(&config.SandboxMetadata{
		Name:        "existing",
		Template:    "test",
		NetworkSlot: 1,
	})

	workspacePath := env.CreateWorkspace("existing")

	creator := &Creator{
		paths:      env.Paths,
		hostConfig: env.HostConfig,
		rt:         env.Runtime,
	}

	_, err := creator.Create(context.Background(), CreateOptions{
		Name:     "existing",
		Template: "test",
		RepoPath: workspacePath,
		Direct:   true,
	})

	if err == nil {
		t.Error("Create() should have failed for duplicate name")
	}
}

func TestCreator_Create_MissingTemplate(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	workspacePath := env.CreateWorkspace("myproject")

	creator := &Creator{
		paths:      env.Paths,
		hostConfig: env.HostConfig,
		rt:         env.Runtime,
	}

	_, err := creator.Create(context.Background(), CreateOptions{
		Name:     "myproject",
		Template: "nonexistent",
		RepoPath: workspacePath,
		Direct:   true,
	})

	if err == nil {
		t.Error("Create() should have failed for missing template")
	}
}

func TestCreator_Create_MissingWorkspace(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	env.AddTemplate("test", testutil.DefaultTemplate())

	creator := &Creator{
		paths:      env.Paths,
		hostConfig: env.HostConfig,
		rt:         env.Runtime,
	}

	_, err := creator.Create(context.Background(), CreateOptions{
		Name:     "myproject",
		Template: "test",
		RepoPath: "/nonexistent/workspace",
		Direct:   true,
	})

	if err == nil {
		t.Error("Create() should have failed for missing workspace")
	}
}

func TestCreator_setupSecrets(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	creator := &Creator{
		paths:      env.Paths,
		hostConfig: env.HostConfig,
		rt:         env.Runtime,
	}

	template := testutil.DefaultTemplate()
	secretsPath := filepath.Join(env.TmpDir, "test-secrets")

	err := creator.setupSecrets(secretsPath, template)
	if err != nil {
		t.Fatalf("setupSecrets() failed: %v", err)
	}

	// Verify secrets directory was created
	if _, statErr := os.Stat(secretsPath); os.IsNotExist(statErr) {
		t.Error("Secrets directory was not created")
	}

	// Verify secret file was created with correct permissions
	secretFile := filepath.Join(secretsPath, "anthropic")
	info, err := os.Stat(secretFile)
	if os.IsNotExist(err) {
		t.Error("Secret file was not created")
	} else if info.Mode().Perm() != 0600 {
		t.Errorf("Secret file permissions = %o, want %o", info.Mode().Perm(), 0600)
	}

	// Verify secret content
	content, err := os.ReadFile(secretFile)
	if err != nil {
		t.Fatalf("Failed to read secret file: %v", err)
	}
	if string(content) != "sk-test-key" {
		t.Errorf("Secret content = %q, want %q", string(content), "sk-test-key")
	}
}

func TestCreator_setupSecrets_MissingSecret(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	// Remove the secret from host config
	env.HostConfig.Secrets = map[string]string{}

	creator := &Creator{
		paths:      env.Paths,
		hostConfig: env.HostConfig,
		rt:         env.Runtime,
	}

	template := testutil.DefaultTemplate()
	secretsPath := filepath.Join(env.TmpDir, "test-secrets")

	// Should not fail, just skip the missing secret
	err := creator.setupSecrets(secretsPath, template)
	if err != nil {
		t.Fatalf("setupSecrets() should not fail for missing secret: %v", err)
	}

	// Secret file should not exist
	secretFile := filepath.Join(secretsPath, "anthropic")
	if _, err := os.Stat(secretFile); !os.IsNotExist(err) {
		t.Error("Secret file should not exist when secret is missing from config")
	}
}

func TestCreator_cleanup(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	env.AddTemplate("test", testutil.DefaultTemplate())

	creator := &Creator{
		paths:      env.Paths,
		hostConfig: env.HostConfig,
		rt:         env.Runtime,
	}

	// Create some resources that cleanup should remove
	metadata := &config.SandboxMetadata{
		Name:        "cleanup-test",
		Template:    "test",
		NetworkSlot: 1,
		Workspace:   filepath.Join(env.TmpDir, "workspace"),
	}

	// Save metadata
	config.SaveSandboxMetadata(env.Paths.SandboxesDir, metadata)

	// Create secrets directory
	secretsPath := filepath.Join(env.Paths.SecretsDir, "cleanup-test")
	os.MkdirAll(secretsPath, 0700)
	os.WriteFile(filepath.Join(secretsPath, "test-secret"), []byte("secret"), 0600)

	// Create config file
	configPath := filepath.Join(env.Paths.SandboxesDir, "cleanup-test.nix")
	os.WriteFile(configPath, []byte("# nix config"), 0644)

	// Add container to mock runtime
	env.Runtime.AddContainer("cleanup-test", runtime.StatusRunning)

	// Run cleanup
	creator.cleanup(metadata)

	// Verify resources were cleaned up
	if env.SandboxExists("cleanup-test") {
		t.Error("Sandbox metadata was not cleaned up")
	}

	if _, err := os.Stat(secretsPath); !os.IsNotExist(err) {
		t.Error("Secrets directory was not cleaned up")
	}

	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Error("Config file was not cleaned up")
	}
}

func TestCreator_resolveIdentity(t *testing.T) {
	tests := []struct {
		name       string
		hostID     *config.AgentIdentity
		tmplID     *config.AgentIdentity
		opts       CreateOptions
		wantNil    bool
		wantUser   string
		wantEmail  string
		wantSSHKey string
	}{
		{
			name:    "no identity anywhere",
			hostID:  nil,
			tmplID:  nil,
			opts:    CreateOptions{},
			wantNil: true,
		},
		{
			name: "host defaults only",
			hostID: &config.AgentIdentity{
				GitUser:  "Host Agent",
				GitEmail: "host@example.com",
			},
			opts:      CreateOptions{},
			wantUser:  "Host Agent",
			wantEmail: "host@example.com",
		},
		{
			name: "template defaults only",
			tmplID: &config.AgentIdentity{
				GitUser:  "Template Agent",
				GitEmail: "template@example.com",
			},
			opts:      CreateOptions{},
			wantUser:  "Template Agent",
			wantEmail: "template@example.com",
		},
		{
			name:   "opts only",
			hostID: nil,
			opts: CreateOptions{
				GitUser:  "Opts Agent",
				GitEmail: "opts@example.com",
			},
			wantUser:  "Opts Agent",
			wantEmail: "opts@example.com",
		},
		{
			name: "template overrides host",
			hostID: &config.AgentIdentity{
				GitUser:  "Host Agent",
				GitEmail: "host@example.com",
			},
			tmplID: &config.AgentIdentity{
				GitUser: "Template Agent",
			},
			opts:      CreateOptions{},
			wantUser:  "Template Agent",
			wantEmail: "host@example.com",
		},
		{
			name: "opts override template and host",
			hostID: &config.AgentIdentity{
				GitUser:    "Host Agent",
				GitEmail:   "host@example.com",
				SSHKeyPath: "/host/key",
			},
			tmplID: &config.AgentIdentity{
				GitUser:  "Template Agent",
				GitEmail: "template@example.com",
			},
			opts: CreateOptions{
				GitUser: "Override Agent",
			},
			wantUser:   "Override Agent",
			wantEmail:  "template@example.com",
			wantSSHKey: "/host/key",
		},
		{
			name: "template SSH key overrides host SSH key",
			hostID: &config.AgentIdentity{
				SSHKeyPath: "/host/key",
			},
			tmplID: &config.AgentIdentity{
				SSHKeyPath: "/template/key",
			},
			opts:       CreateOptions{},
			wantSSHKey: "/template/key",
		},
		{
			name: "opts override SSH key",
			hostID: &config.AgentIdentity{
				SSHKeyPath: "/host/key",
			},
			opts: CreateOptions{
				SSHKeyPath: "/opts/key",
			},
			wantSSHKey: "/opts/key",
		},
		{
			name:   "opts SSH key only",
			hostID: nil,
			opts: CreateOptions{
				SSHKeyPath: "/my/key",
			},
			wantSSHKey: "/my/key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creator := &Creator{
				hostConfig: &config.HostConfig{
					User:          "nonexistent-user-for-test",
					AgentIdentity: tt.hostID,
				},
			}

			tmpl := &config.Template{
				AgentIdentity: tt.tmplID,
			}

			result := creator.resolveIdentity(tt.opts, tmpl)

			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Fatal("expected non-nil identity")
			}
			if result.GitUser != tt.wantUser {
				t.Errorf("GitUser = %q, want %q", result.GitUser, tt.wantUser)
			}
			if result.GitEmail != tt.wantEmail {
				t.Errorf("GitEmail = %q, want %q", result.GitEmail, tt.wantEmail)
			}
			if result.SSHKeyPath != tt.wantSSHKey {
				t.Errorf("SSHKeyPath = %q, want %q", result.SSHKeyPath, tt.wantSSHKey)
			}
		})
	}
}

func TestCreator_runInitCommands_NoCommands(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	creator := &Creator{
		paths:      env.Paths,
		hostConfig: env.HostConfig,
		rt:         env.Runtime,
	}

	metadata := &config.SandboxMetadata{
		Name:          "test-init",
		Template:      "test",
		NetworkSlot:   1,
		ContainerName: "f1",
	}
	template := &config.Template{
		Name: "test",
	}

	// Add container so exec can find it
	env.Runtime.AddContainer("f1", runtime.StatusRunning)

	result := creator.runInitCommands(context.Background(), metadata, template)

	if result.TemplateCommandsRun != 0 {
		t.Errorf("TemplateCommandsRun = %d, want 0", result.TemplateCommandsRun)
	}
	if len(result.TemplateWarnings) != 0 {
		t.Errorf("TemplateWarnings = %v, want empty", result.TemplateWarnings)
	}

	// Should have .forage/init check (test -f) + execution (sh) since mock returns success
	execCalls := env.Runtime.GetCallsFor("Exec")
	if len(execCalls) != 2 {
		t.Errorf("Expected 2 Exec calls (forage/init check + run), got %d", len(execCalls))
	}
}

func TestCreator_runInitCommands_TemplateCommands(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	creator := &Creator{
		paths:      env.Paths,
		hostConfig: env.HostConfig,
		rt:         env.Runtime,
	}

	metadata := &config.SandboxMetadata{
		Name:          "test-init",
		Template:      "test",
		NetworkSlot:   1,
		ContainerName: "f1",
	}
	template := &config.Template{
		Name:         "test",
		InitCommands: []string{"echo hello", "echo world"},
	}

	env.Runtime.AddContainer("f1", runtime.StatusRunning)

	result := creator.runInitCommands(context.Background(), metadata, template)

	if result.TemplateCommandsRun != 2 {
		t.Errorf("TemplateCommandsRun = %d, want 2", result.TemplateCommandsRun)
	}
	if len(result.TemplateWarnings) != 0 {
		t.Errorf("TemplateWarnings = %v, want empty", result.TemplateWarnings)
	}

	// Verify exec calls: 2 init commands + 1 .forage/init check + 1 .forage/init run (default returns 0)
	execCalls := env.Runtime.GetCallsFor("Exec")
	if len(execCalls) < 2 {
		t.Fatalf("Expected at least 2 Exec calls, got %d", len(execCalls))
	}

	// Check first command args
	cmd1 := execCalls[0].Args[1].([]string)
	if len(cmd1) != 3 || cmd1[0] != "sh" || cmd1[1] != "-c" || cmd1[2] != "echo hello" {
		t.Errorf("First command = %v, want [sh -c echo hello]", cmd1)
	}

	// Check second command args
	cmd2 := execCalls[1].Args[1].([]string)
	if len(cmd2) != 3 || cmd2[0] != "sh" || cmd2[1] != "-c" || cmd2[2] != "echo world" {
		t.Errorf("Second command = %v, want [sh -c echo world]", cmd2)
	}

	// Verify exec options (user and workdir)
	opts := execCalls[0].Args[2].(runtime.ExecOptions)
	if opts.User != "agent" {
		t.Errorf("Exec User = %q, want %q", opts.User, "agent")
	}
	if opts.WorkingDir != "/workspace" {
		t.Errorf("Exec WorkingDir = %q, want %q", opts.WorkingDir, "/workspace")
	}
}

func TestCreator_runInitCommands_FailedCommandContinues(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	creator := &Creator{
		paths:      env.Paths,
		hostConfig: env.HostConfig,
		rt:         env.Runtime,
	}

	metadata := &config.SandboxMetadata{
		Name:          "test-init",
		Template:      "test",
		NetworkSlot:   1,
		ContainerName: "f1",
	}
	template := &config.Template{
		Name:         "test",
		InitCommands: []string{"failing-cmd", "second-cmd"},
	}

	env.Runtime.AddContainer("f1", runtime.StatusRunning)
	// Set exec result to non-zero exit code for this container
	env.Runtime.SetExecResult("f1", &runtime.ExecResult{ExitCode: 1, Stderr: "command failed"})

	result := creator.runInitCommands(context.Background(), metadata, template)

	// Both commands should have been attempted
	if result.TemplateCommandsRun != 2 {
		t.Errorf("TemplateCommandsRun = %d, want 2", result.TemplateCommandsRun)
	}

	// Both should have warnings
	if len(result.TemplateWarnings) != 2 {
		t.Errorf("len(TemplateWarnings) = %d, want 2", len(result.TemplateWarnings))
	}

	// Verify all exec calls were made (2 commands + 1 .forage/init check; no init run since check fails)
	execCalls := env.Runtime.GetCallsFor("Exec")
	if len(execCalls) != 3 {
		t.Errorf("Expected 3 Exec calls, got %d", len(execCalls))
	}
}

func TestCreator_runInitCommands_ProjectInit(t *testing.T) {
	env := testutil.NewTestEnv(t)
	defer env.Cleanup()

	creator := &Creator{
		paths:      env.Paths,
		hostConfig: env.HostConfig,
		rt:         env.Runtime,
	}

	metadata := &config.SandboxMetadata{
		Name:          "test-init",
		Template:      "test",
		NetworkSlot:   1,
		ContainerName: "f1",
	}
	template := &config.Template{
		Name: "test",
	}

	env.Runtime.AddContainer("f1", runtime.StatusRunning)
	// Default mock returns ExitCode 0, so test -f will "find" the file
	// and then sh will "run" it

	result := creator.runInitCommands(context.Background(), metadata, template)

	if !result.ProjectInitRun {
		t.Error("ProjectInitRun should be true when .forage/init check succeeds")
	}
	if result.ProjectInitWarning != "" {
		t.Errorf("ProjectInitWarning = %q, want empty", result.ProjectInitWarning)
	}

	// Verify exec calls: 1 test -f check + 1 sh run
	execCalls := env.Runtime.GetCallsFor("Exec")
	if len(execCalls) != 2 {
		t.Fatalf("Expected 2 Exec calls, got %d", len(execCalls))
	}

	// Check the test -f call
	testCmd := execCalls[0].Args[1].([]string)
	if testCmd[0] != "test" || testCmd[1] != "-f" {
		t.Errorf("First command = %v, want [test -f ...]", testCmd)
	}

	// Check the sh call
	shCmd := execCalls[1].Args[1].([]string)
	if shCmd[0] != "sh" {
		t.Errorf("Second command = %v, want [sh ...]", shCmd)
	}
}

func TestWorkspaceBackendFor(t *testing.T) {
	tests := []struct {
		mode     WorkspaceMode
		wantName string
		wantNil  bool
	}{
		{WorkspaceModeJJ, "jj", false},
		{WorkspaceModeGitWorktree, "git-worktree", false},
		{WorkspaceModeDirect, "", true},
		{"", "", true},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			backend := workspaceBackendFor(tt.mode)
			if tt.wantNil {
				if backend != nil {
					t.Errorf("workspaceBackendFor(%q) = %v, want nil", tt.mode, backend)
				}
			} else {
				if backend == nil {
					t.Errorf("workspaceBackendFor(%q) = nil, want non-nil", tt.mode)
				} else if backend.Name() != tt.wantName {
					t.Errorf("workspaceBackendFor(%q).Name() = %q, want %q",
						tt.mode, backend.Name(), tt.wantName)
				}
			}
		})
	}
}

//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	os.Exit(SetupSharedEnv(m))
}

func TestModuleSetup(t *testing.T) {
	env := GetSharedEnv(t)

	t.Run("forage-ctl installed", func(t *testing.T) {
		AssertSuccess(t, env.System, "forage-ctl is available", "which forage-ctl")
	})

	t.Run("directories", func(t *testing.T) {
		dirs := []string{
			"/var/lib/firefly-forage",
			"/var/lib/firefly-forage/sandboxes",
			"/var/lib/firefly-forage/workspaces",
			"/etc/firefly-forage/templates",
		}
		for _, dir := range dirs {
			t.Run(dir, func(t *testing.T) {
				AssertSuccess(t, env.System, dir+" exists", "test -d "+dir)
			})
		}
	})

	t.Run("host config", func(t *testing.T) {
		AssertSuccess(t, env.System, "host config exists",
			"test -f /etc/firefly-forage/config.json")
		AssertOutputContains(t, env.System, "host config has correct user",
			`"user"`, "cat /etc/firefly-forage/config.json")
	})

	t.Run("template", func(t *testing.T) {
		AssertSuccess(t, env.System, "template JSON exists",
			"test -f /etc/firefly-forage/templates/test.json")
		AssertOutputContains(t, env.System, "template has network config",
			`"network"`, "cat /etc/firefly-forage/templates/test.json")
	})

	t.Run("extra-container available", func(t *testing.T) {
		AssertSuccess(t, env.System, "extra-container is available", "which extra-container")
	})

	t.Run("secrets directory", func(t *testing.T) {
		AssertSuccess(t, env.System, "secrets directory exists", "test -d /run/forage-secrets")
	})

	t.Run("templates command", func(t *testing.T) {
		AssertOutputContains(t, env.System, "templates lists test template",
			"test", "forage-ctl templates")
		AssertOutputContains(t, env.System, "templates shows agent name",
			"test-agent", "forage-ctl templates")
	})
}

func TestSandboxLifecycle(t *testing.T) {
	env := GetSharedEnv(t)
	ctx := context.Background()

	// Clean up any stale sandboxes
	env.System.ForageCtl(ctx, "down", "e2e-test")

	// Create test repository
	env.InitGitRepo(t, "/tmp/e2e-project", map[string]string{
		"README.md": "# E2E Test Project",
	})

	// === forage-ctl up ===
	t.Log("running forage-ctl up...")
	env.MustRun(t, "forage-ctl up e2e-test -t test --repo /tmp/e2e-project --direct > /tmp/forage-up.log 2>&1")
	t.Cleanup(func() {
		env.System.ForageCtl(context.Background(), "down", "e2e-test")
	})

	// Wait for sandbox
	containerIP := "10.100.1.2"
	env.WaitForSandbox(t, containerIP, 60*time.Second)

	// Connect to sandbox
	sb := env.ConnectSandbox(t, "e2e-test", containerIP)

	t.Run("exec/basic connectivity", func(t *testing.T) {
		AssertSandboxSuccess(t, sb, "can run commands in sandbox", "true")
	})

	t.Run("exec/workspace", func(t *testing.T) {
		AssertSandboxOutputContains(t, sb, "workspace has README",
			"E2E Test Project", "cat /workspace/README.md")
	})

	t.Run("exec/forage metadata", func(t *testing.T) {
		AssertSandboxOutputContains(t, sb, "forage.json has sandbox name",
			"e2e-test", "cat /etc/forage.json")
	})

	t.Run("exec/packages", func(t *testing.T) {
		AssertSandboxSuccess(t, sb, "git is available", "which git")
		AssertSandboxSuccess(t, sb, "jj is available", "which jj")
		AssertSandboxSuccess(t, sb, "tmux is available", "which tmux")
	})

	t.Run("exec/vcs", func(t *testing.T) {
		AssertSandboxOutputContains(t, sb, "git log works",
			"Initial commit", "cd /workspace && git log --oneline -1")
		AssertSandboxSuccess(t, sb, "jj init works",
			"cd /workspace && jj git init --colocate 2>&1")
		AssertSandboxSuccess(t, sb, "jj log works after init",
			"cd /workspace && jj log --no-graph -r @ -T description 2>&1")
	})

	t.Run("exec/secrets", func(t *testing.T) {
		// The test template configures test-secret with authEnvVar=TEST_KEY.
		// Verify the secret file is mounted and the env var is set.
		AssertSandboxOutputContains(t, sb, "secret file mounted",
			"test-api-key-e2e", "cat /run/secrets/test-secret")
		AssertSandboxOutputContains(t, sb, "auth env var set",
			"test-api-key-e2e", "printenv TEST_KEY")
	})

	t.Run("exec/network-none", func(t *testing.T) {
		// The test template uses network=none. Outbound traffic should
		// be blocked, but SSH management access works (already proven
		// by all the exec tests above).
		AssertSandboxFailure(t, sb, "outbound ping blocked",
			"ping -c 1 -W 2 8.8.8.8")
	})

	t.Run("audit-log", func(t *testing.T) {
		// After forage-ctl up, the audit log should contain a create event
		AssertOutputContains(t, env.System, "audit log has create event",
			"create", "forage-ctl audit-log e2e-test")
	})

	t.Run("exec/file sync", func(t *testing.T) {
		AssertSandboxSuccess(t, sb, "can create files in workspace",
			"echo hello-from-sandbox > /workspace/sandbox-created.txt")

		// Verify file visible on host
		AssertSuccess(t, env.System, "file visible on host",
			"test -f /tmp/e2e-project/sandbox-created.txt")
		AssertOutputContains(t, env.System, "content matches",
			"hello-from-sandbox", "cat /tmp/e2e-project/sandbox-created.txt")
	})

	t.Run("status", func(t *testing.T) {
		AssertOutputContains(t, env.System, "status shows container healthy",
			"Container:", "forage-ctl status e2e-test")
	})

	t.Run("ps", func(t *testing.T) {
		AssertOutputContains(t, env.System, "ps shows sandbox",
			"e2e-test", "forage-ctl ps")
	})

	// === Destructive operations (must be sequential, after all reads) ===

	t.Run("stop-start", func(t *testing.T) {
		// Close old sandbox connection before stopping
		sb.Close()

		t.Log("stopping sandbox...")
		AssertSuccess(t, env.System, "forage-ctl stop succeeds",
			"forage-ctl stop e2e-test")

		// Verify status shows stopped
		AssertOutputContains(t, env.System, "status shows stopped after stop",
			"stopped", "forage-ctl status e2e-test 2>&1 || true")

		t.Log("starting sandbox...")
		AssertSuccess(t, env.System, "forage-ctl start succeeds",
			"forage-ctl start e2e-test")

		// Wait for sandbox to be reachable again
		env.WaitForSandbox(t, containerIP, 60*time.Second)

		// Reconnect and verify workspace survived the stop/start cycle
		sbAfter := env.ConnectSandbox(t, "e2e-test", containerIP)
		AssertSandboxOutputContains(t, sbAfter, "workspace intact after stop/start",
			"E2E Test Project", "cat /workspace/README.md")
		sbAfter.Close()
	})

	t.Run("reset", func(t *testing.T) {
		// Create a file outside /workspace (ephemeral container state)
		sbPre := env.ConnectSandbox(t, "e2e-test", containerIP)
		AssertSandboxSuccess(t, sbPre, "create ephemeral file",
			"touch /tmp/ephemeral-marker")
		sbPre.Close()

		t.Log("resetting sandbox...")
		AssertSuccess(t, env.System, "forage-ctl reset succeeds",
			"forage-ctl reset e2e-test")

		// Wait for sandbox to be reachable after reset
		env.WaitForSandbox(t, containerIP, 60*time.Second)

		// Verify ephemeral state is gone but workspace persists
		sbPost := env.ConnectSandbox(t, "e2e-test", containerIP)
		AssertSandboxFailure(t, sbPost, "ephemeral file gone after reset",
			"test -f /tmp/ephemeral-marker")
		AssertSandboxOutputContains(t, sbPost, "workspace intact after reset",
			"E2E Test Project", "cat /workspace/README.md")
		sbPost.Close()
	})

	t.Run("down", func(t *testing.T) {
		t.Log("running forage-ctl down...")
		AssertSuccess(t, env.System, "forage-ctl down succeeds",
			"forage-ctl down e2e-test")
		AssertFailure(t, env.System, "sandbox no longer exists after down",
			"forage-ctl status e2e-test")

		// Verify cleanup: metadata and secrets removed
		AssertFailure(t, env.System, "metadata file removed",
			"test -f /var/lib/firefly-forage/sandboxes/e2e-test.json")
		AssertFailure(t, env.System, "nix config file removed",
			"test -f /var/lib/firefly-forage/sandboxes/e2e-test.nix")
		AssertFailure(t, env.System, "secrets directory removed",
			"test -d /run/forage-secrets/e2e-test")
	})
}

func TestMultipleSandboxes(t *testing.T) {
	env := GetSharedEnv(t)
	ctx := context.Background()

	// Clean up any stale sandboxes
	env.System.ForageCtl(ctx, "down", "e2e-multi-a")
	env.System.ForageCtl(ctx, "down", "e2e-multi-b")

	// Create two separate project directories
	env.InitGitRepo(t, "/tmp/e2e-project-a", map[string]string{
		"README.md": "# Project A",
	})
	env.InitGitRepo(t, "/tmp/e2e-project-b", map[string]string{
		"README.md": "# Project B",
	})

	// Start sandbox A
	t.Log("starting sandbox A...")
	env.MustRun(t, "forage-ctl up e2e-multi-a -t test --repo /tmp/e2e-project-a --direct > /tmp/forage-multi-a.log 2>&1")
	t.Cleanup(func() {
		env.System.ForageCtl(context.Background(), "down", "e2e-multi-a")
	})

	// Start sandbox B
	t.Log("starting sandbox B...")
	env.MustRun(t, "forage-ctl up e2e-multi-b -t test --repo /tmp/e2e-project-b --direct > /tmp/forage-multi-b.log 2>&1")
	t.Cleanup(func() {
		env.System.ForageCtl(context.Background(), "down", "e2e-multi-b")
	})

	ipA := "10.100.1.2"
	ipB := "10.100.2.2"

	env.WaitForSandbox(t, ipA, 60*time.Second)
	env.WaitForSandbox(t, ipB, 60*time.Second)

	sbA := env.ConnectSandbox(t, "e2e-multi-a", ipA)
	sbB := env.ConnectSandbox(t, "e2e-multi-b", ipB)

	t.Run("sandbox A has correct project", func(t *testing.T) {
		AssertSandboxOutputContains(t, sbA, "sandbox A has project-a README",
			"Project A", "cat /workspace/README.md")
	})

	t.Run("sandbox B has correct project", func(t *testing.T) {
		AssertSandboxOutputContains(t, sbB, "sandbox B has project-b README",
			"Project B", "cat /workspace/README.md")
	})

	t.Run("ps shows both", func(t *testing.T) {
		AssertOutputContains(t, env.System, "ps shows sandbox A",
			"e2e-multi-a", "forage-ctl ps")
		AssertOutputContains(t, env.System, "ps shows sandbox B",
			"e2e-multi-b", "forage-ctl ps")
	})
}

func TestGarbageCollection(t *testing.T) {
	env := GetSharedEnv(t)
	ctx := context.Background()

	// Clean up any stale sandbox
	env.System.ForageCtl(ctx, "down", "e2e-gc")

	// Create a sandbox
	env.InitGitRepo(t, "/tmp/e2e-gc-project", map[string]string{
		"README.md": "# GC Test",
	})

	t.Log("creating sandbox for gc test...")
	env.MustRun(t, "forage-ctl up e2e-gc -t test --repo /tmp/e2e-gc-project --direct > /tmp/forage-gc.log 2>&1")

	// Dry run: should report no orphans (sandbox is running)
	t.Run("dry-run-clean", func(t *testing.T) {
		AssertOutputContains(t, env.System, "gc dry run reports clean",
			"No orphaned resources", "forage-ctl gc")
	})

	// Tear down the sandbox
	t.Log("tearing down sandbox for gc test...")
	env.MustRun(t, "forage-ctl down e2e-gc")

	// Create an orphaned metadata file (simulates incomplete cleanup)
	env.MustRun(t, `echo '{"name":"e2e-orphan"}' > /var/lib/firefly-forage/sandboxes/e2e-orphan.json`)

	// Dry run: should detect the orphan
	t.Run("dry-run-detects-orphan", func(t *testing.T) {
		AssertOutputContains(t, env.System, "gc dry run detects orphaned file",
			"e2e-orphan", "forage-ctl gc")
	})

	// Force: should clean up the orphan
	t.Run("force-cleans-orphan", func(t *testing.T) {
		AssertSuccess(t, env.System, "gc force succeeds",
			"forage-ctl gc --force")
		AssertFailure(t, env.System, "orphaned file removed",
			"test -f /var/lib/firefly-forage/sandboxes/e2e-orphan.json")
	})
}

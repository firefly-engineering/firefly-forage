//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	os.Exit(SetupSharedEnv(m))
}

func TestModuleSetup(t *testing.T) {
	env := GetSharedEnv(t)

	t.Run("forage-ctl installed", func(t *testing.T) {
		t.Parallel()
		AssertSuccess(t, env.Ctx(t), env.System, "forage-ctl is available", "which forage-ctl")
	})

	t.Run("directories", func(t *testing.T) {
		t.Parallel()
		dirs := []string{
			"/var/lib/firefly-forage",
			"/var/lib/firefly-forage/sandboxes",
			"/var/lib/firefly-forage/workspaces",
			"/etc/firefly-forage/templates",
		}
		for _, dir := range dirs {
			AssertSuccess(t, env.Ctx(t), env.System, dir+" exists", "test -d "+dir)
		}
	})

	t.Run("host config", func(t *testing.T) {
		t.Parallel()
		AssertSuccess(t, env.Ctx(t), env.System, "host config exists",
			"test -f /etc/firefly-forage/config.json")
		AssertOutputContains(t, env.Ctx(t), env.System, "host config has correct user",
			`"user"`, "cat /etc/firefly-forage/config.json")
	})

	t.Run("template", func(t *testing.T) {
		t.Parallel()
		AssertSuccess(t, env.Ctx(t), env.System, "template JSON exists",
			"test -f /etc/firefly-forage/templates/test.json")
		AssertOutputContains(t, env.Ctx(t), env.System, "template has network config",
			`"network"`, "cat /etc/firefly-forage/templates/test.json")
	})

	t.Run("extra-container available", func(t *testing.T) {
		t.Parallel()
		AssertSuccess(t, env.Ctx(t), env.System, "extra-container is available", "which extra-container")
	})

	t.Run("secrets directory", func(t *testing.T) {
		t.Parallel()
		AssertSuccess(t, env.Ctx(t), env.System, "secrets directory exists", "test -d /run/forage-secrets")
	})

	t.Run("templates command", func(t *testing.T) {
		t.Parallel()
		AssertOutputContains(t, env.Ctx(t), env.System, "templates lists test template",
			"test", "forage-ctl templates")
		AssertOutputContains(t, env.Ctx(t), env.System, "templates shows agent name",
			"test-agent", "forage-ctl templates")
	})
}

func TestSandboxLifecycle(t *testing.T) {
	env := GetSharedEnv(t)

	// Clean up any stale sandboxes
	env.System.ForageCtl(env.Ctx(t), "down", "e2e-test")

	// Create test repository
	env.InitGitRepo(t, "/tmp/e2e-project", map[string]string{
		"README.md": "# E2E Test Project",
	})

	// === forage-ctl up ===
	t.Log("running forage-ctl up...")
	env.MustRun(t, "forage-ctl up e2e-test -t test --repo /tmp/e2e-project --direct > /tmp/forage-up.log 2>&1")
	t.Cleanup(func() {
		env.System.ForageCtl(env.Ctx(t), "down", "e2e-test")
	})

	// Wait for sandbox
	containerIP := "10.100.1.2"
	env.WaitForSandbox(t, containerIP, 60*time.Second)

	// Connect to sandbox (ssh.Client is safe for concurrent use)
	sb := env.ConnectSandbox(t, "e2e-test", containerIP)

	// Phase 1: All read-only checks run in parallel.
	// The "verify" subtest blocks until every parallel child finishes,
	// ensuring destructive operations below don't start early.
	t.Run("verify", func(t *testing.T) {
		t.Run("connectivity", func(t *testing.T) {
			t.Parallel()
			AssertSandboxSuccess(t, env.Ctx(t), sb, "can run commands in sandbox", "true")
		})

		t.Run("workspace", func(t *testing.T) {
			t.Parallel()
			AssertSandboxOutputContains(t, env.Ctx(t), sb, "workspace has README",
				"E2E Test Project", "cat /workspace/README.md")
		})

		t.Run("forage metadata", func(t *testing.T) {
			t.Parallel()
			AssertSandboxOutputContains(t, env.Ctx(t), sb, "forage.json has sandbox name",
				"e2e-test", "cat /etc/forage.json")
		})

		t.Run("packages", func(t *testing.T) {
			t.Parallel()
			AssertSandboxSuccess(t, env.Ctx(t), sb, "git is available", "which git")
			AssertSandboxSuccess(t, env.Ctx(t), sb, "jj is available", "which jj")
			AssertSandboxSuccess(t, env.Ctx(t), sb, "tmux is available", "which tmux")
		})

		t.Run("vcs", func(t *testing.T) {
			t.Parallel()
			AssertSandboxOutputContains(t, env.Ctx(t), sb, "git log works",
				"Initial commit", "cd /workspace && git log --oneline -1")
			// Note: jj init is skipped here because it mutates workspace state.
			// It was tested in the original sequential flow but is not safe to
			// run concurrently with other read-only workspace checks.
		})

		t.Run("secrets", func(t *testing.T) {
			t.Parallel()
			AssertSandboxOutputContains(t, env.Ctx(t), sb, "secret file mounted",
				"test-api-key-e2e", "cat /run/secrets/test-secret")
			// Note: auth env var (TEST_KEY) is only set for recognized agent
			// types (e.g. "claude"). The generic "test-agent" doesn't get
			// env var injection, so we only verify file-level access here.
		})

		t.Run("network-none", func(t *testing.T) {
			t.Parallel()
			AssertSandboxFailure(t, env.Ctx(t), sb, "outbound ping blocked",
				"ping -c 1 -W 2 8.8.8.8")
		})

		t.Run("audit-log", func(t *testing.T) {
			t.Parallel()
			AssertOutputContains(t, env.Ctx(t), env.System, "audit log has create event",
				"create", "forage-ctl audit-log e2e-test")
		})

		t.Run("status", func(t *testing.T) {
			t.Parallel()
			AssertOutputContains(t, env.Ctx(t), env.System, "status shows container healthy",
				"Container:", "forage-ctl status e2e-test")
		})

		t.Run("ps", func(t *testing.T) {
			t.Parallel()
			AssertOutputContains(t, env.Ctx(t), env.System, "ps shows sandbox",
				"e2e-test", "forage-ctl ps")
		})
	})

	// Phase 2: Sequential operations that mutate state

	t.Run("file sync", func(t *testing.T) {
		AssertSandboxSuccess(t, env.Ctx(t), sb, "can create files in workspace",
			"echo hello-from-sandbox > /workspace/sandbox-created.txt")

		// Verify file visible on host
		AssertSuccess(t, env.Ctx(t), env.System, "file visible on host",
			"test -f /tmp/e2e-project/sandbox-created.txt")
		AssertOutputContains(t, env.Ctx(t), env.System, "content matches",
			"hello-from-sandbox", "cat /tmp/e2e-project/sandbox-created.txt")
	})

	t.Run("jj init", func(t *testing.T) {
		AssertSandboxSuccess(t, env.Ctx(t), sb, "jj init works",
			"cd /workspace && jj git init --colocate 2>&1")
		AssertSandboxSuccess(t, env.Ctx(t), sb, "jj log works after init",
			"cd /workspace && jj log --no-graph -r @ -T description 2>&1")
	})

	// Phase 3: Destructive operations (stop/start, reset, down)

	t.Run("stop-start", func(t *testing.T) {
		// Close old sandbox connection before stopping
		sb.Close()

		t.Log("stopping sandbox...")
		AssertSuccess(t, env.Ctx(t), env.System, "forage-ctl stop succeeds",
			"forage-ctl stop e2e-test")

		// Verify status shows container not running (✗ = not running)
		AssertOutputContains(t, env.Ctx(t), env.System, "status shows container down after stop",
			"Container: ✗", "forage-ctl status e2e-test 2>&1 || true")

		t.Log("starting sandbox...")
		AssertSuccess(t, env.Ctx(t), env.System, "forage-ctl start succeeds",
			"forage-ctl start e2e-test")

		// Wait for sandbox to be reachable again
		env.WaitForSandbox(t, containerIP, 60*time.Second)

		// Reconnect and verify workspace survived the stop/start cycle
		sbAfter := env.ConnectSandbox(t, "e2e-test", containerIP)
		AssertSandboxOutputContains(t, env.Ctx(t), sbAfter, "workspace intact after stop/start",
			"E2E Test Project", "cat /workspace/README.md")
		sbAfter.Close()
	})

	t.Run("reset", func(t *testing.T) {
		// Create a file outside /workspace (ephemeral container state)
		sbPre := env.ConnectSandbox(t, "e2e-test", containerIP)
		AssertSandboxSuccess(t, env.Ctx(t), sbPre, "create ephemeral file",
			"touch /tmp/ephemeral-marker")
		sbPre.Close()

		t.Log("resetting sandbox...")
		AssertSuccess(t, env.Ctx(t), env.System, "forage-ctl reset succeeds",
			"forage-ctl reset e2e-test")

		// Wait for sandbox to be reachable after reset
		env.WaitForSandbox(t, containerIP, 60*time.Second)

		// Verify ephemeral state is gone but workspace persists
		sbPost := env.ConnectSandbox(t, "e2e-test", containerIP)
		AssertSandboxFailure(t, env.Ctx(t), sbPost, "ephemeral file gone after reset",
			"test -f /tmp/ephemeral-marker")
		AssertSandboxOutputContains(t, env.Ctx(t), sbPost, "workspace intact after reset",
			"E2E Test Project", "cat /workspace/README.md")
		sbPost.Close()
	})

	t.Run("down", func(t *testing.T) {
		t.Log("running forage-ctl down...")
		AssertSuccess(t, env.Ctx(t), env.System, "forage-ctl down succeeds",
			"forage-ctl down e2e-test")
		AssertFailure(t, env.Ctx(t), env.System, "sandbox no longer exists after down",
			"forage-ctl status e2e-test")

		// Verify cleanup: metadata and secrets removed
		AssertFailure(t, env.Ctx(t), env.System, "metadata file removed",
			"test -f /var/lib/firefly-forage/sandboxes/e2e-test.json")
		AssertFailure(t, env.Ctx(t), env.System, "nix config file removed",
			"test -f /var/lib/firefly-forage/sandboxes/e2e-test.nix")
		AssertFailure(t, env.Ctx(t), env.System, "secrets directory removed",
			"test -d /run/forage-secrets/e2e-test")
	})
}

func TestMultipleSandboxes(t *testing.T) {
	env := GetSharedEnv(t)

	// Clean up any stale sandboxes
	env.System.ForageCtl(env.Ctx(t), "down", "e2e-multi-a")
	env.System.ForageCtl(env.Ctx(t), "down", "e2e-multi-b")

	// Create two separate project directories
	env.InitGitRepo(t, "/tmp/e2e-project-a", map[string]string{
		"README.md": "# Project A",
	})
	env.InitGitRepo(t, "/tmp/e2e-project-b", map[string]string{
		"README.md": "# Project B",
	})

	// Start both sandboxes in parallel (slot allocation is serialized by the sandbox lock)
	t.Cleanup(func() {
		env.System.ForageCtl(env.Ctx(t), "down", "e2e-multi-a")
		env.System.ForageCtl(env.Ctx(t), "down", "e2e-multi-b")
	})

	var wg sync.WaitGroup
	var errA, errB error
	wg.Add(2)
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(env.Ctx(t), 2*time.Minute)
		defer cancel()
		_, errA = env.System.Run(ctx, "forage-ctl up e2e-multi-a -t test --repo /tmp/e2e-project-a --direct > /tmp/forage-multi-a.log 2>&1")
	}()
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(env.Ctx(t), 2*time.Minute)
		defer cancel()
		_, errB = env.System.Run(ctx, "forage-ctl up e2e-multi-b -t test --repo /tmp/e2e-project-b --direct > /tmp/forage-multi-b.log 2>&1")
	}()
	wg.Wait()
	if errA != nil {
		t.Fatalf("sandbox A creation failed: %v", errA)
	}
	if errB != nil {
		t.Fatalf("sandbox B creation failed: %v", errB)
	}

	// Look up IPs from metadata (slot assignment is non-deterministic with parallel creation)
	ipA := sandboxIP(t, env, "e2e-multi-a")
	ipB := sandboxIP(t, env, "e2e-multi-b")

	env.WaitForSandbox(t, ipA, 60*time.Second)
	env.WaitForSandbox(t, ipB, 60*time.Second)

	sbA := env.ConnectSandbox(t, "e2e-multi-a", ipA)
	sbB := env.ConnectSandbox(t, "e2e-multi-b", ipB)

	// All verification subtests run in parallel
	t.Run("verify", func(t *testing.T) {
		t.Run("sandbox A has correct project", func(t *testing.T) {
			t.Parallel()
			AssertSandboxOutputContains(t, env.Ctx(t), sbA, "sandbox A has project-a README",
				"Project A", "cat /workspace/README.md")
		})

		t.Run("sandbox B has correct project", func(t *testing.T) {
			t.Parallel()
			AssertSandboxOutputContains(t, env.Ctx(t), sbB, "sandbox B has project-b README",
				"Project B", "cat /workspace/README.md")
		})

		t.Run("ps shows both", func(t *testing.T) {
			t.Parallel()
			AssertOutputContains(t, env.Ctx(t), env.System, "ps shows sandbox A",
				"e2e-multi-a", "forage-ctl ps")
			AssertOutputContains(t, env.Ctx(t), env.System, "ps shows sandbox B",
				"e2e-multi-b", "forage-ctl ps")
		})
	})
}

func TestGarbageCollection(t *testing.T) {
	env := GetSharedEnv(t)

	// Clean up any stale sandbox
	env.System.ForageCtl(env.Ctx(t), "down", "e2e-gc")

	// Create a sandbox
	env.InitGitRepo(t, "/tmp/e2e-gc-project", map[string]string{
		"README.md": "# GC Test",
	})

	t.Log("creating sandbox for gc test...")
	env.MustRun(t, "forage-ctl up e2e-gc -t test --repo /tmp/e2e-gc-project --direct > /tmp/forage-gc.log 2>&1")

	// Dry run: should report no orphans (sandbox is running)
	t.Run("dry-run-clean", func(t *testing.T) {
		AssertOutputContains(t, env.Ctx(t), env.System, "gc dry run reports clean",
			"No orphaned resources", "forage-ctl gc")
	})

	// Tear down the sandbox
	t.Log("tearing down sandbox for gc test...")
	env.MustRun(t, "forage-ctl down e2e-gc")

	// Create an orphaned metadata file (simulates incomplete cleanup)
	env.MustRun(t, `echo '{"name":"e2e-orphan"}' > /var/lib/firefly-forage/sandboxes/e2e-orphan.json`)

	// Dry run: should detect the orphan
	t.Run("dry-run-detects-orphan", func(t *testing.T) {
		AssertOutputContains(t, env.Ctx(t), env.System, "gc dry run detects orphaned file",
			"e2e-orphan", "forage-ctl gc")
	})

	// Force: should clean up the orphan
	t.Run("force-cleans-orphan", func(t *testing.T) {
		AssertSuccess(t, env.Ctx(t), env.System, "gc force succeeds",
			"forage-ctl gc --force")
		AssertFailure(t, env.Ctx(t), env.System, "orphaned file removed",
			"test -f /var/lib/firefly-forage/sandboxes/e2e-orphan.json")
	})
}

// sandboxIP reads the networkSlot from sandbox metadata and returns the container IP.
func sandboxIP(t *testing.T, env *TestEnv, name string) string {
	t.Helper()
	// Use grep+sed instead of jq since the VM may not have jq installed
	cmd := fmt.Sprintf(`grep -o '"networkSlot": *[0-9]*' /var/lib/firefly-forage/sandboxes/%s.json | grep -o '[0-9]*$'`, name)
	output, err := env.System.Run(env.Ctx(t), cmd)
	if err != nil {
		t.Fatalf("failed to read networkSlot for %s: %v", name, err)
	}
	slot := strings.TrimSpace(output)
	return fmt.Sprintf("10.100.%s.2", slot)
}

//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/telemetry"
)

// sharedEnv holds the singleton test environment shared across all tests.
var sharedEnv *TestEnv

// TestEnv ties a System to testing.T for convenient test helpers.
type TestEnv struct {
	System  System
	rootCtx context.Context

	mu       sync.Mutex
	testCtxs map[*testing.T]context.Context
}

// Ctx returns a context for the given test, creating a per-test span
// on first call. The span is ended automatically via t.Cleanup.
// All operations within a test should derive from this context so
// that spans form a single trace: root → test → operations.
func (e *TestEnv) Ctx(t *testing.T) context.Context {
	t.Helper()
	e.mu.Lock()
	defer e.mu.Unlock()

	if ctx, ok := e.testCtxs[t]; ok {
		return ctx
	}

	ctx, span := telemetry.Start(e.rootCtx, "test."+t.Name())
	t.Cleanup(func() {
		if t.Failed() {
			span.SetStatus(codes.Error, "test failed")
		}
		span.End()

		e.mu.Lock()
		delete(e.testCtxs, t)
		e.mu.Unlock()
	})

	e.testCtxs[t] = ctx
	return ctx
}

// SetupSharedEnv initializes the shared test environment based on env vars:
//   - E2E_VM set → VM mode (boot QEMU, run tests via SSH)
//   - E2E_LOCAL=1 → Local mode (run tests against current machine)
//   - Neither set → skip all E2E tests gracefully (return 0)
func SetupSharedEnv(m *testing.M) int {
	ctx := context.Background()
	shutdown, err := telemetry.Init(ctx, "forage-e2e")
	if err != nil {
		log.Printf("telemetry init: %v", err)
	}

	ctx, rootSpan := telemetry.Start(ctx, "e2e.run")

	// run executes the test suite and returns the exit code.
	// Separated so we can explicitly end the root span and flush
	// the exporter before os.Exit kills the process.
	code := setupAndRun(ctx, m)

	rootSpan.End()
	shutdown()
	return code
}

func setupAndRun(ctx context.Context, m *testing.M) int {
	if vmScript := os.Getenv("E2E_VM"); vmScript != "" {
		// VM mode (existing behavior)
		sshKey := os.Getenv("E2E_SSH_KEY")
		if sshKey == "" {
			log.Fatal("E2E_SSH_KEY not set.")
		}

		sys, err := NewVMSystem(ctx, VMConfig{
			VMScript:    vmScript,
			SSHKeyPath:  sshKey,
			BootTimeout: 5 * time.Minute,
		})
		if err != nil {
			log.Fatalf("failed to boot VM: %v", err)
		}

		sharedEnv = &TestEnv{
			System:   sys,
			rootCtx:  ctx,
			testCtxs: make(map[*testing.T]context.Context),
		}
		code := m.Run()
		sys.Close()
		return code
	}

	if os.Getenv("E2E_LOCAL") != "" {
		// Local mode: run against current machine
		sshKey := os.Getenv("E2E_SSH_KEY")
		if sshKey == "" {
			sshKey = "/etc/firefly-forage/ssh-key"
		}

		sharedEnv = &TestEnv{
			System:   NewLocalSystem(sshKey),
			rootCtx:  ctx,
			testCtxs: make(map[*testing.T]context.Context),
		}
		code := m.Run()
		sharedEnv.System.Close()
		return code
	}

	// No mode set: skip gracefully
	log.Println("E2E_VM and E2E_LOCAL not set; skipping E2E tests")
	return 0
}

// GetSharedEnv returns the shared TestEnv, or skips the test if no
// environment was configured (neither E2E_VM nor E2E_LOCAL set).
func GetSharedEnv(t *testing.T) *TestEnv {
	t.Helper()
	if sharedEnv == nil {
		t.Skip("E2E environment not configured; set E2E_VM or E2E_LOCAL")
	}
	return sharedEnv
}

// MustRun executes a command and fails the test if it errors.
func (e *TestEnv) MustRun(t *testing.T, cmd string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(e.Ctx(t), 2*time.Minute)
	defer cancel()

	output, err := e.System.Run(ctx, cmd)
	if err != nil {
		t.Fatalf("command failed: %s\nerror: %v", cmd, err)
	}
	return output
}

// MustForageCtl runs forage-ctl and fails the test if it errors.
func (e *TestEnv) MustForageCtl(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(e.Ctx(t), 5*time.Minute)
	defer cancel()

	output, err := e.System.ForageCtl(ctx, args...)
	if err != nil {
		t.Fatalf("forage-ctl %s failed: %v", strings.Join(args, " "), err)
	}
	return output
}

// InitGitRepo creates a git repository in the VM with the given files.
func (e *TestEnv) InitGitRepo(t *testing.T, path string, files map[string]string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(e.Ctx(t), 30*time.Second)
	defer cancel()

	// Remove any existing directory and create fresh
	cmd := fmt.Sprintf("rm -rf %s && mkdir -p %s && cd %s && git init -q", path, path, path)
	if _, err := e.System.Run(ctx, cmd); err != nil {
		t.Fatalf("init git repo: %v", err)
	}

	// Write files
	for name, content := range files {
		dir := fmt.Sprintf("%s/%s", path, name)
		// Ensure parent directory exists
		if strings.Contains(name, "/") {
			parentDir := dir[:strings.LastIndex(dir, "/")]
			e.System.Run(ctx, fmt.Sprintf("mkdir -p %s", parentDir))
		}
		// Use printf to handle special characters safely
		writeCmd := fmt.Sprintf("printf '%%s' %q > %s/%s", content, path, name)
		if _, err := e.System.Run(ctx, writeCmd); err != nil {
			t.Fatalf("write file %s: %v", name, err)
		}
	}

	// Commit
	commitCmd := fmt.Sprintf("cd %s && git add . && git commit -q -m 'Initial commit' && chown -R 1000:100 %s", path, path)
	if _, err := e.System.Run(ctx, commitCmd); err != nil {
		t.Fatalf("git commit: %v", err)
	}
}

// WaitForSandbox waits for a sandbox to become SSH-reachable.
func (e *TestEnv) WaitForSandbox(t *testing.T, ip string, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(e.Ctx(t), timeout)
	defer cancel()
	ctx, span := telemetry.Start(ctx, "e2e.wait-for-sandbox",
		telemetry.WithAttr(attribute.String("sandbox.ip", ip)))
	defer span.End()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := e.System.DialSandbox(ctx, ip)
		if err == nil {
			// Try running a command
			_, err = conn.Run(ctx, "true")
			conn.Close()
			if err == nil {
				t.Logf("sandbox %s ready", ip)
				return
			}
		}
		time.Sleep(time.Second)
	}

	// Diagnostics on failure
	t.Logf("sandbox at %s not ready after %v, running diagnostics...", ip, timeout)
	diagCtx, diagCancel := context.WithTimeout(e.Ctx(t), 10*time.Second)
	defer diagCancel()
	if out, err := e.System.Run(diagCtx, "machinectl list"); err == nil {
		t.Logf("machinectl list:\n%s", out)
	}
	if out, err := e.System.Run(diagCtx, "forage-ctl ps"); err == nil {
		t.Logf("forage-ctl ps:\n%s", out)
	}
	if out, err := e.System.Run(diagCtx, fmt.Sprintf("ping -c 1 -W 2 %s", ip)); err == nil {
		t.Logf("ping:\n%s", out)
	}

	t.Fatalf("sandbox %s did not become ready within %v", ip, timeout)
}

// ConnectSandbox connects to a sandbox and registers cleanup.
func (e *TestEnv) ConnectSandbox(t *testing.T, name, ip string) *SandboxConn {
	t.Helper()
	ctx, cancel := context.WithTimeout(e.Ctx(t), 10*time.Second)
	defer cancel()

	conn, err := e.System.DialSandbox(ctx, ip)
	if err != nil {
		t.Fatalf("connect to sandbox %s (%s): %v", name, ip, err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// Assertion helpers that use testing.T for proper failure reporting.
// All accept a context.Context to propagate trace context.

// AssertSuccess asserts that a command succeeds in the system.
func AssertSuccess(t *testing.T, ctx context.Context, sys System, desc, cmd string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	if _, err := sys.Run(ctx, cmd); err != nil {
		t.Errorf("%s: %v", desc, err)
	}
}

// AssertFailure asserts that a command fails in the system.
func AssertFailure(t *testing.T, ctx context.Context, sys System, desc, cmd string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if _, err := sys.Run(ctx, cmd); err == nil {
		t.Errorf("%s: expected failure but succeeded", desc)
	}
}

// AssertOutputContains asserts that a command's output contains expected string.
func AssertOutputContains(t *testing.T, ctx context.Context, sys System, desc, expected, cmd string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	output, err := sys.Run(ctx, cmd)
	// Allow non-zero exit codes; we only care about output content
	if err != nil {
		// Extract output from error if the command itself produced output
		output = extractOutput(output, err)
	}
	if !strings.Contains(output, expected) {
		t.Errorf("%s: output does not contain %q\nactual: %s", desc, expected, output)
	}
}

// AssertSandboxSuccess asserts that a command succeeds in a sandbox.
func AssertSandboxSuccess(t *testing.T, ctx context.Context, sb *SandboxConn, desc, cmd string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if _, err := sb.Run(ctx, cmd); err != nil {
		t.Errorf("%s: %v", desc, err)
	}
}

// AssertSandboxFailure asserts that a command fails in a sandbox.
func AssertSandboxFailure(t *testing.T, ctx context.Context, sb *SandboxConn, desc, cmd string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if _, err := sb.Run(ctx, cmd); err == nil {
		t.Errorf("%s: expected failure but succeeded", desc)
	}
}

// AssertSandboxOutputContains asserts sandbox command output contains expected string.
func AssertSandboxOutputContains(t *testing.T, ctx context.Context, sb *SandboxConn, desc, expected, cmd string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	output, err := sb.Run(ctx, cmd)
	if err != nil {
		output = extractOutput(output, err)
	}
	if !strings.Contains(output, expected) {
		t.Errorf("%s: output does not contain %q\nactual: %s", desc, expected, output)
	}
}

// extractOutput extracts any output from an error message or returns the
// original output. Commands may exit non-zero but still produce useful output.
func extractOutput(output string, err error) string {
	if output != "" {
		return output
	}
	// Try to extract output from the error string
	errStr := err.Error()
	if idx := strings.Index(errStr, "\noutput: "); idx >= 0 {
		return errStr[idx+len("\noutput: "):]
	}
	return errStr
}

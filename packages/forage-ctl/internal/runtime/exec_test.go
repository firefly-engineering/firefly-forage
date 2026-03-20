package runtime

import (
	"context"
	"strings"
	"testing"

	shellquote "github.com/kballard/go-shellquote"
)

func TestExecShell(t *testing.T) {
	ctx := context.Background()
	mock := NewMockRuntime()
	mock.AddContainer("test", StatusRunning)

	t.Run("passes script as sh -c to Exec", func(t *testing.T) {
		mock.Reset()
		mock.AddContainer("test", StatusRunning)

		script := "echo hello && echo world"
		result, err := ExecShell(ctx, mock, "test", script, ExecOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.ExitCode != 0 {
			t.Fatalf("unexpected exit code: %d", result.ExitCode)
		}

		calls := mock.GetCallsFor("Exec")
		if len(calls) != 1 {
			t.Fatalf("expected 1 Exec call, got %d", len(calls))
		}
		cmd := calls[0].Args[1].([]string)
		if len(cmd) != 3 || cmd[0] != "sh" || cmd[1] != "-c" || cmd[2] != script {
			t.Errorf("expected [sh -c %q], got %v", script, cmd)
		}
	})

	t.Run("forwards exec options", func(t *testing.T) {
		mock.Reset()
		mock.AddContainer("test", StatusRunning)

		opts := ExecOptions{User: "root", WorkingDir: "/tmp"}
		ExecShell(ctx, mock, "test", "ls", opts)

		calls := mock.GetCallsFor("Exec")
		if len(calls) != 1 {
			t.Fatalf("expected 1 Exec call, got %d", len(calls))
		}
		gotOpts := calls[0].Args[2].(ExecOptions)
		if gotOpts.User != "root" {
			t.Errorf("expected User=root, got %q", gotOpts.User)
		}
		if gotOpts.WorkingDir != "/tmp" {
			t.Errorf("expected WorkingDir=/tmp, got %q", gotOpts.WorkingDir)
		}
	})

	t.Run("propagates errors", func(t *testing.T) {
		mock.Reset()
		mock.AddContainer("test", StatusRunning)
		mock.SetError("Exec", context.DeadlineExceeded)

		_, err := ExecShell(ctx, mock, "test", "sleep 999", ExecOptions{})
		if err != context.DeadlineExceeded {
			t.Errorf("expected DeadlineExceeded, got %v", err)
		}
	})

	t.Run("handles complex shell expressions", func(t *testing.T) {
		mock.Reset()
		mock.AddContainer("test", StatusRunning)

		script := `if tmux has-session -t forage 2>/dev/null; then tmux -CC attach-session -t forage; else tmux -CC new-session -s forage -c /workspace; fi`
		ExecShell(ctx, mock, "test", script, ExecOptions{})

		calls := mock.GetCallsFor("Exec")
		cmd := calls[0].Args[1].([]string)
		if cmd[2] != script {
			t.Errorf("script was modified:\n  got:  %s\n  want: %s", cmd[2], script)
		}
	})

	t.Run("handles multiline scripts", func(t *testing.T) {
		mock.Reset()
		mock.AddContainer("test", StatusRunning)

		script := "set -e\ntmux new-session -d -s forage\ntrue"
		ExecShell(ctx, mock, "test", script, ExecOptions{})

		calls := mock.GetCallsFor("Exec")
		cmd := calls[0].Args[1].([]string)
		if cmd[2] != script {
			t.Errorf("multiline script was modified:\n  got:  %q\n  want: %q", cmd[2], script)
		}
	})
}

func TestExecShellInteractive(t *testing.T) {
	ctx := context.Background()
	mock := NewMockRuntime()

	t.Run("passes script as sh -c to ExecInteractive", func(t *testing.T) {
		mock.Reset()
		mock.AddContainer("test", StatusRunning)

		script := "tmux attach-session -t forage || tmux new-session -s forage"
		err := ExecShellInteractive(ctx, mock, "test", script, ExecOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		calls := mock.GetCallsFor("ExecInteractive")
		if len(calls) != 1 {
			t.Fatalf("expected 1 ExecInteractive call, got %d", len(calls))
		}
		cmd := calls[0].Args[1].([]string)
		if len(cmd) != 3 || cmd[0] != "sh" || cmd[1] != "-c" || cmd[2] != script {
			t.Errorf("expected [sh -c %q], got %v", script, cmd)
		}
	})

	t.Run("propagates errors", func(t *testing.T) {
		mock.Reset()
		mock.AddContainer("test", StatusRunning)
		mock.SetError("ExecInteractive", context.Canceled)

		err := ExecShellInteractive(ctx, mock, "test", "tmux attach", ExecOptions{})
		if err != context.Canceled {
			t.Errorf("expected Canceled, got %v", err)
		}
	})
}

// buildExecArgs extracts the arg-building logic from AppleRuntime.Exec for
// testability. It returns the args slice that would be passed to the container
// CLI binary (excluding the binary path itself).
func (r *AppleRuntime) buildExecArgs(sandboxName string, command []string, opts ExecOptions) []string {
	containerName := r.containerName(sandboxName)

	args := []string{"exec"}

	if opts.Interactive {
		args = append(args, "-i", "-t")
	}

	if opts.User != "" {
		args = append(args, "-u", opts.User)
	}

	if opts.WorkingDir != "" {
		args = append(args, "-w", opts.WorkingDir)
	}

	for _, env := range opts.Env {
		args = append(args, "-e", env)
	}

	args = append(args, containerName)
	args = append(args, "/bin/sh", "-c", shellquote.Join(command...))

	return args
}

func TestAppleRuntime_ExecCommandConstruction(t *testing.T) {
	rt := &AppleRuntime{
		BinaryPath:      "/usr/bin/container",
		ContainerPrefix: "forage-",
	}

	t.Run("token commands are shellquoted", func(t *testing.T) {
		// ["ls", "/nix/store"] → shellquote.Join → "ls /nix/store"
		args := rt.buildExecArgs("mybox", []string{"ls", "/nix/store"}, ExecOptions{})
		assertArgsSuffix(t, args, "/bin/sh", "-c", "ls /nix/store")
	})

	t.Run("shell expressions via ExecShell are double-wrapped correctly", func(t *testing.T) {
		// ExecShell passes ["sh", "-c", script] to Exec.
		// shellquote.Join produces: sh -c '<script>'
		// The outer /bin/sh -c runs: sh -c '<script>'
		// The inner sh interprets: <script>
		script := `echo "hello world"`
		args := rt.buildExecArgs("mybox", []string{"sh", "-c", script}, ExecOptions{})
		assertArgsSuffix(t, args, "/bin/sh", "-c", `sh -c 'echo "hello world"'`)
	})

	t.Run("complex tmux attach command survives quoting", func(t *testing.T) {
		script := `if tmux has-session -t forage 2>/dev/null; then tmux -CC attach-session -t forage; else tmux -CC new-session -s forage -c /workspace; fi`
		args := rt.buildExecArgs("mybox", []string{"sh", "-c", script}, ExecOptions{})
		shC := findShellArg(args)
		if shC == "" {
			t.Fatal("no /bin/sh -c found in args")
		}
		// The outer shell command must contain the full script text
		// (wrapped inside the inner sh -c)
		if !strings.Contains(shC, "if tmux has-session") {
			t.Errorf("shell command doesn't contain tmux expression:\n  %s", shC)
		}
		if !strings.Contains(shC, "tmux -CC new-session") {
			t.Errorf("shell command doesn't contain new-session fallback:\n  %s", shC)
		}
	})

	t.Run("multiline init script survives quoting", func(t *testing.T) {
		script := "              tmux new-session -d -s forage -c /workspace -n main\n              tmux set-option -w -t forage:main automatic-rename off\n              true"
		args := rt.buildExecArgs("mybox", []string{"sh", "-c", script}, ExecOptions{})
		shC := findShellArg(args)
		if shC == "" {
			t.Fatal("no /bin/sh -c found in args")
		}
		if !strings.Contains(shC, "tmux new-session") {
			t.Errorf("shell command doesn't contain tmux new-session:\n  %s", shC)
		}
	})

	t.Run("user option", func(t *testing.T) {
		args := rt.buildExecArgs("mybox", []string{"whoami"}, ExecOptions{User: "root"})
		assertArgsPair(t, args, "-u", "root")
	})

	t.Run("working directory option", func(t *testing.T) {
		args := rt.buildExecArgs("mybox", []string{"pwd"}, ExecOptions{WorkingDir: "/workspace"})
		assertArgsPair(t, args, "-w", "/workspace")
	})

	t.Run("environment variables", func(t *testing.T) {
		args := rt.buildExecArgs("mybox", []string{"env"}, ExecOptions{Env: []string{"FOO=bar", "BAZ=qux"}})
		assertArgsPair(t, args, "-e", "FOO=bar")
		assertArgsPair(t, args, "-e", "BAZ=qux")
	})

	t.Run("container name uses prefix", func(t *testing.T) {
		args := rt.buildExecArgs("mybox", []string{"echo"}, ExecOptions{})
		assertArgsHas(t, args, "forage-mybox")
	})

	t.Run("special characters in arguments are escaped", func(t *testing.T) {
		args := rt.buildExecArgs("mybox", []string{"echo", "hello world", "$HOME"}, ExecOptions{})
		shC := findShellArg(args)
		if shC == "" {
			t.Fatal("no /bin/sh -c found in args")
		}
		// shellquote.Join should quote the space-containing arg and escape $
		if !strings.Contains(shC, "hello world") {
			t.Errorf("expected 'hello world' in shell cmd: %s", shC)
		}
	})

	t.Run("empty command array", func(t *testing.T) {
		args := rt.buildExecArgs("mybox", []string{}, ExecOptions{})
		shC := findShellArg(args)
		// shellquote.Join of empty slice produces ""
		if shC != "" {
			t.Errorf("expected empty shell command for empty command array, got %q", shC)
		}
	})

	t.Run("interactive flag", func(t *testing.T) {
		args := rt.buildExecArgs("mybox", []string{"bash"}, ExecOptions{Interactive: true})
		assertArgsPair(t, args, "-i", "-t")
	})
}

// --- test helpers ---

// findShellArg returns the argument following "/bin/sh -c" in args.
func findShellArg(args []string) string {
	for i, a := range args {
		if a == "/bin/sh" && i+2 < len(args) && args[i+1] == "-c" {
			return args[i+2]
		}
	}
	return ""
}

// assertArgsSuffix checks that args ends with the given values.
func assertArgsSuffix(t *testing.T, args []string, suffix ...string) {
	t.Helper()
	if len(args) < len(suffix) {
		t.Fatalf("args too short: %v, expected suffix %v", args, suffix)
	}
	tail := args[len(args)-len(suffix):]
	for i, s := range suffix {
		if tail[i] != s {
			t.Errorf("args[%d] = %q, want %q\n  full args: %v", len(args)-len(suffix)+i, tail[i], s, args)
		}
	}
}

// assertArgsPair checks that the consecutive pair [key, value] appears in args.
func assertArgsPair(t *testing.T, args []string, key, value string) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key && args[i+1] == value {
			return
		}
	}
	t.Errorf("args %v does not contain [%q, %q]", args, key, value)
}

// assertArgsHas checks that value appears somewhere in args.
func assertArgsHas(t *testing.T, args []string, value string) {
	t.Helper()
	for _, a := range args {
		if a == value {
			return
		}
	}
	t.Errorf("args %v does not contain %q", args, value)
}

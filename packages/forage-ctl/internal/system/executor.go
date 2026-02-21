package system

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// safeEnvPrefixes lists environment variable prefixes that are safe to pass
// through to child processes. All others are filtered out to prevent
// accidental leakage of host secrets (API keys, cloud credentials, etc.).
var safeEnvPrefixes = []string{
	"PATH=", "HOME=", "USER=", "LOGNAME=", "SHELL=",
	"TERM=", "LANG=", "LC_", "LANGUAGE=",
	"XDG_", "DISPLAY=", "WAYLAND_DISPLAY=",
	"SSH_AUTH_SOCK=", "DBUS_SESSION_BUS_ADDRESS=",
	"TMPDIR=", "TMP=", "TEMP=",
	"COLORTERM=", "COLORFGBG=",
	"NO_COLOR=", "FORCE_COLOR=",
	"EDITOR=", "VISUAL=", "PAGER=",
	"HOSTNAME=", "HOSTTYPE=", "OSTYPE=",
	"NIX_", "IN_NIX_SHELL=",
}

// SafeEnviron returns a filtered copy of os.Environ() containing only
// safe environment variables. This prevents leaking host secrets like
// ANTHROPIC_API_KEY, AWS_SECRET_ACCESS_KEY, etc. to child processes.
func SafeEnviron() []string {
	var filtered []string
	for _, env := range os.Environ() {
		for _, prefix := range safeEnvPrefixes {
			if strings.HasPrefix(env, prefix) {
				filtered = append(filtered, env)
				break
			}
		}
	}
	return filtered
}

// osExecutor implements CommandExecutor using real OS operations.
type osExecutor struct{}

func (e *osExecutor) Execute(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

func (e *osExecutor) ExecuteWithStdin(ctx context.Context, stdin string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	return cmd.CombinedOutput()
}

func (e *osExecutor) ExecuteInteractive(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (e *osExecutor) ReplaceProcess(name string, args ...string) error {
	binary, err := exec.LookPath(name)
	if err != nil {
		return err
	}

	// Build argv with program name as first element
	argv := append([]string{name}, args...)

	return syscall.Exec(binary, argv, SafeEnviron())
}

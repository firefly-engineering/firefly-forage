package system

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

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

	return syscall.Exec(binary, argv, os.Environ())
}

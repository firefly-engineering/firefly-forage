package gateway

import (
	"context"
	"fmt"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/multiplexer"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/ssh"
)

// Connect loads sandbox metadata, verifies it is running, and replaces the
// current process with an SSH session to the sandbox (with the appropriate
// multiplexer attach command, if any).
func Connect(ctx context.Context, name, sandboxesDir string, rt runtime.Runtime) error {
	metadata, err := config.LoadSandboxMetadata(sandboxesDir, name)
	if err != nil {
		return fmt.Errorf("sandbox not found: %s", name)
	}

	if rt != nil {
		running, _ := rt.IsRunning(ctx, name)
		if !running {
			return fmt.Errorf("sandbox %s is not running", name)
		}
	}

	caps := runtime.GetCapabilities(rt)
	containerIP := metadata.ContainerIP()
	logging.Debug("connecting to sandbox", "name", name, "ip", containerIP, "sshAccess", caps.SSHAccess)

	mux := multiplexer.New(multiplexer.Type(metadata.Multiplexer))
	attachCmd := mux.AttachCommand()

	// For runtimes without SSH, use exec-based attach
	if !caps.SSHAccess && rt != nil {
		if attachCmd != "" {
			return runtime.ExecShellInteractive(ctx, rt, name, attachCmd, runtime.ExecOptions{})
		}
		return rt.ExecInteractive(ctx, name, []string{"sh"}, runtime.ExecOptions{})
	}

	if attachCmd != "" {
		return ssh.ReplaceWithSession(containerIP, attachCmd)
	}
	// For multiplexers without an attach command (e.g. wezterm in SSH context),
	// fall back to an interactive shell.
	return ssh.ReplaceWithSession(containerIP, "")
}

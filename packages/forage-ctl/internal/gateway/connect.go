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
func Connect(name, sandboxesDir string, rt runtime.Runtime) error {
	metadata, err := config.LoadSandboxMetadata(sandboxesDir, name)
	if err != nil {
		return fmt.Errorf("sandbox not found: %s", name)
	}

	if rt != nil {
		running, _ := rt.IsRunning(context.Background(), name)
		if !running {
			return fmt.Errorf("sandbox %s is not running", name)
		}
	}

	containerIP := metadata.ContainerIP()
	logging.Debug("connecting to sandbox", "name", name, "ip", containerIP)

	mux := multiplexer.New(multiplexer.Type(metadata.Multiplexer))
	if attachCmd := mux.AttachCommand(); attachCmd != "" {
		return ssh.ReplaceWithSession(containerIP, attachCmd)
	}
	// For multiplexers without an attach command (e.g. wezterm in SSH context),
	// fall back to an interactive shell.
	return ssh.ReplaceWithSession(containerIP, "")
}

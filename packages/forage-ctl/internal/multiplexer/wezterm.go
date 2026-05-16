package multiplexer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	shellquote "github.com/kballard/go-shellquote"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/injection"
)

// Wezterm implements Multiplexer for wezterm-mux-server.
type Wezterm struct{}

func (w *Wezterm) Type() Type { return TypeWezterm }

func (w *Wezterm) NixPackages() []string { return []string{"wezterm"} }

func (w *Wezterm) InitScript(windows []Window) string {
	var sb strings.Builder
	sb.WriteString("              wezterm-mux-server --daemonize\n")
	for i, win := range windows {
		if i == 0 {
			// The mux server creates a default tab; set its title.
			fmt.Fprintf(&sb, "              wezterm cli set-tab-title %s\n", shellquote.Join(win.Name))
		} else {
			fmt.Fprintf(&sb, "              wezterm cli spawn --cwd /workspace\n")
			fmt.Fprintf(&sb, "              wezterm cli set-tab-title %s\n", shellquote.Join(win.Name))
		}
		if win.Command != "" {
			fmt.Fprintf(&sb, "              wezterm cli send-text --no-paste %s\n", shellquote.Join(win.Command+"\n"))
		}
	}
	sb.WriteString("              true")
	return sb.String()
}

// AttachCommand returns empty — wezterm uses native `wezterm connect`, not SSH.
func (w *Wezterm) AttachCommand() string { return "" }

func (w *Wezterm) CheckSessionArgs() []string {
	return []string{"pgrep", "-x", "wezterm-mux-server"}
}

func (w *Wezterm) ListWindowsArgs() []string {
	return []string{"wezterm", "cli", "list", "--format", "json"}
}

func (w *Wezterm) ParseWindowList(output string) []string {
	// wezterm cli list outputs tab-separated rows; parse window titles.
	// With --format json we get JSON but for simplicity parse lines.
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var windows []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			windows = append(windows, line)
		}
	}
	return windows
}

// HostConfigMounts returns nil — wezterm config is client-side only.
func (w *Wezterm) HostConfigMounts(homeDir string) []ConfigMount { return nil }

func (w *Wezterm) PromptInstructions() string {
	return "Terminal multiplexing via wezterm. New tabs: `wezterm cli spawn --cwd /workspace`. List panes: `wezterm cli list`."
}

// ContributePackages returns the packages needed for wezterm.
func (w *Wezterm) ContributePackages(ctx context.Context) ([]injection.Package, error) {
	return []injection.Package{{Name: "wezterm"}}, nil
}

// ContributeMounts returns nil - wezterm config is client-side only.
func (w *Wezterm) ContributeMounts(ctx context.Context, req *injection.MountRequest) ([]injection.Mount, error) {
	return nil, nil
}

// ContributePromptFragments returns prompt instructions for wezterm.
func (w *Wezterm) ContributePromptFragments(ctx context.Context) ([]injection.PromptFragment, error) {
	return []injection.PromptFragment{{
		Section:  injection.PromptSectionEnvironment,
		Priority: 100,
		Content:  w.PromptInstructions(),
	}}, nil
}

// NativeConnect execs `wezterm connect` for the named container.
// This replaces the current process.
func (w *Wezterm) NativeConnect(containerName string) error {
	binary, err := exec.LookPath("wezterm")
	if err != nil {
		return fmt.Errorf("wezterm not found in PATH: %w", err)
	}
	argv := []string{"wezterm", "connect", containerName}
	return syscall.Exec(binary, argv, os.Environ())
}

// NativeConnector is an optional interface for multiplexers that support
// connecting via a native client rather than SSH.
type NativeConnector interface {
	NativeConnect(containerName string) error
}

// Ensure Wezterm implements contribution interfaces
var (
	_ injection.MountContributor   = (*Wezterm)(nil)
	_ injection.PackageContributor = (*Wezterm)(nil)
	_ injection.PromptContributor  = (*Wezterm)(nil)
	_ NativeConnector              = (*Wezterm)(nil)
)

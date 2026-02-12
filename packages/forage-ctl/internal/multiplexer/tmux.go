package multiplexer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/injection"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/terminal"
)

// SessionName is the tmux/mux session name used in all sandboxes.
const SessionName = "forage"

// Tmux implements Multiplexer for tmux.
type Tmux struct {
	// DisableControlMode prevents automatic use of tmux -CC even when
	// the host terminal supports it. Used by --no-tmux-cc flag.
	DisableControlMode bool
}

func (t *Tmux) Type() Type { return TypeTmux }

func (t *Tmux) NixPackages() []string { return []string{"tmux"} }

// shellQuote returns a single-quoted shell string, escaping any embedded
// single quotes using the '\” idiom.
func shellQuote(s string) string {
	return fmt.Sprintf("'%s'", strings.ReplaceAll(s, "'", `'\''`))
}

func (t *Tmux) InitScript(windows []Window) string {
	var sb strings.Builder
	for i, w := range windows {
		if i == 0 {
			fmt.Fprintf(&sb, "              tmux new-session -d -s %s -c /workspace -n %s\n", SessionName, w.Name)
		} else {
			fmt.Fprintf(&sb, "              tmux new-window -t %s -n %s -c /workspace\n", SessionName, w.Name)
		}
		if w.Command != "" {
			fmt.Fprintf(&sb, "              tmux send-keys -t %s:%s %s Enter\n", SessionName, w.Name, shellQuote(w.Command))
		}
	}
	sb.WriteString("              true")
	return sb.String()
}

func (t *Tmux) AttachCommand() string {
	// Use tmux control mode (-CC) when the host terminal supports it,
	// unless explicitly disabled.
	if !t.DisableControlMode && terminal.SupportsControlMode() {
		// Two constraints for control mode:
		// 1. Only invoke tmux -CC once. A failed -CC attach emits DCS
		//    protocol bytes that cause wezterm to enter and immediately
		//    exit control mode, tearing down the pane.
		// 2. Don't use exec. When tmux -CC exits, the %exit protocol
		//    message must be flushed before the process terminates;
		//    exec causes immediate exit which can leave wezterm hung.
		// Use if/then/else so exactly one -CC runs without exec.
		return fmt.Sprintf("if tmux has-session -t %s 2>/dev/null; then tmux -CC attach-session -t %s; else tmux -CC new-session -s %s -c /workspace; fi",
			SessionName, SessionName, SessionName)
	}
	return fmt.Sprintf("tmux attach-session -t %s || tmux new-session -s %s -c /workspace", SessionName, SessionName)
}

func (t *Tmux) CheckSessionArgs() []string {
	return []string{"tmux", "has-session", "-t", SessionName}
}

func (t *Tmux) ListWindowsArgs() []string {
	return []string{"tmux", "list-windows", "-t", SessionName, "-F", "#{window_index}:#{window_name}"}
}

func (t *Tmux) ParseWindowList(output string) []string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var windows []string
	for _, line := range lines {
		if line != "" {
			windows = append(windows, line)
		}
	}
	return windows
}

func (t *Tmux) HostConfigMounts(homeDir string) []ConfigMount {
	if homeDir == "" {
		return nil
	}
	// Prefer ~/.config/tmux dir, fall back to ~/.tmux.conf
	tmuxConfigDir := filepath.Join(homeDir, ".config", "tmux")
	if info, err := os.Stat(tmuxConfigDir); err == nil && info.IsDir() {
		return []ConfigMount{{
			ContainerPath: "/home/agent/.config/tmux",
			HostPath:      tmuxConfigDir,
			ReadOnly:      true,
		}}
	}
	tmuxConfFile := filepath.Join(homeDir, ".tmux.conf")
	if _, err := os.Stat(tmuxConfFile); err == nil {
		return []ConfigMount{{
			ContainerPath: "/home/agent/.tmux.conf",
			HostPath:      tmuxConfFile,
			ReadOnly:      true,
		}}
	}
	return nil
}

func (t *Tmux) PromptInstructions() string {
	return fmt.Sprintf("Use tmux (`tmux attach -t %s`).", SessionName)
}

// ContributePackages returns the packages needed for tmux.
func (t *Tmux) ContributePackages(ctx context.Context) ([]injection.Package, error) {
	return []injection.Package{{Name: "tmux"}}, nil
}

// ContributeMounts returns host config mounts for tmux.
func (t *Tmux) ContributeMounts(ctx context.Context, req *injection.MountRequest) ([]injection.Mount, error) {
	if req.HostHomeDir == "" {
		return nil, nil
	}

	// Prefer ~/.config/tmux dir, fall back to ~/.tmux.conf
	tmuxConfigDir := filepath.Join(req.HostHomeDir, ".config", "tmux")
	if info, err := os.Stat(tmuxConfigDir); err == nil && info.IsDir() {
		return []injection.Mount{{
			HostPath:      tmuxConfigDir,
			ContainerPath: "/home/agent/.config/tmux",
			ReadOnly:      true,
		}}, nil
	}

	tmuxConfFile := filepath.Join(req.HostHomeDir, ".tmux.conf")
	if _, err := os.Stat(tmuxConfFile); err == nil {
		return []injection.Mount{{
			HostPath:      tmuxConfFile,
			ContainerPath: "/home/agent/.tmux.conf",
			ReadOnly:      true,
		}}, nil
	}

	return nil, nil
}

// ContributePromptFragments returns prompt instructions for tmux.
func (t *Tmux) ContributePromptFragments(ctx context.Context) ([]injection.PromptFragment, error) {
	return []injection.PromptFragment{{
		Section:  injection.PromptSectionEnvironment,
		Priority: 100,
		Content:  t.PromptInstructions(),
	}}, nil
}

// Ensure Tmux implements contribution interfaces
var (
	_ injection.MountContributor   = (*Tmux)(nil)
	_ injection.PackageContributor = (*Tmux)(nil)
	_ injection.PromptContributor  = (*Tmux)(nil)
)

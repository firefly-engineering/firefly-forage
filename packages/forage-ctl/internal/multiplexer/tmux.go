package multiplexer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SessionName is the tmux/mux session name used in all sandboxes.
const SessionName = "forage"

// Tmux implements Multiplexer for tmux.
type Tmux struct{}

func (t *Tmux) Type() Type { return TypeTmux }

func (t *Tmux) NixPackages() []string { return []string{"tmux"} }

// shellQuote returns a single-quoted shell string, escaping any embedded
// single quotes using the '\'' idiom.
func shellQuote(s string) string {
	return fmt.Sprintf("'%s'", strings.ReplaceAll(s, "'", `'\''`))
}

func (t *Tmux) InitScript(windows []Window) string {
	var sb strings.Builder
	for i, w := range windows {
		if i == 0 {
			fmt.Fprintf(&sb, "            tmux new-session -d -s %s -c /workspace -n %s\n", SessionName, w.Name)
		} else {
			fmt.Fprintf(&sb, "            tmux new-window -t %s -n %s -c /workspace\n", SessionName, w.Name)
		}
		if w.Command != "" {
			fmt.Fprintf(&sb, "            tmux send-keys -t %s:%s %s Enter\n", SessionName, w.Name, shellQuote(w.Command))
		}
	}
	sb.WriteString("            true")
	return sb.String()
}

func (t *Tmux) AttachCommand() string {
	return fmt.Sprintf("tmux attach-session -t %s || tmux new-session -s %s -c /workspace", SessionName, SessionName)
}

// AttachCommandCC returns the remote command for tmux control mode (-CC).
// This is tmux-specific and not part of the Multiplexer interface.
func (t *Tmux) AttachCommandCC() string {
	return fmt.Sprintf("tmux -CC attach-session -t %s || tmux -CC new-session -s %s -c /workspace", SessionName, SessionName)
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

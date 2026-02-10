package multiplexer

import (
	"fmt"
	"strings"
)

// Wezterm implements Multiplexer for wezterm-mux-server.
type Wezterm struct{}

func (w *Wezterm) Type() Type { return TypeWezterm }

func (w *Wezterm) NixPackages() []string { return []string{"wezterm"} }

func (w *Wezterm) InitScript(windows []Window) string {
	var sb strings.Builder
	sb.WriteString("            wezterm-mux-server --daemonize\n")
	for i, win := range windows {
		if i == 0 {
			// The mux server creates a default tab; set its title.
			fmt.Fprintf(&sb, "            wezterm cli set-tab-title %s\n", shellQuote(win.Name))
		} else {
			fmt.Fprintf(&sb, "            wezterm cli spawn --cwd /workspace\n")
			fmt.Fprintf(&sb, "            wezterm cli set-tab-title %s\n", shellQuote(win.Name))
		}
		if win.Command != "" {
			fmt.Fprintf(&sb, "            wezterm cli send-text --no-paste %s\n", shellQuote(win.Command+"\n"))
		}
	}
	sb.WriteString("            true")
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

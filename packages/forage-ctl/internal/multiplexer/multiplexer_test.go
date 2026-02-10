package multiplexer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewDefault(t *testing.T) {
	mux := New("")
	if mux.Type() != TypeTmux {
		t.Errorf("New(\"\") type = %q, want %q", mux.Type(), TypeTmux)
	}
}

func TestNewTmux(t *testing.T) {
	mux := New(TypeTmux)
	if mux.Type() != TypeTmux {
		t.Errorf("New(TypeTmux) type = %q, want %q", mux.Type(), TypeTmux)
	}
}

func TestNewWezterm(t *testing.T) {
	mux := New(TypeWezterm)
	if mux.Type() != TypeWezterm {
		t.Errorf("New(TypeWezterm) type = %q, want %q", mux.Type(), TypeWezterm)
	}
}

func TestNewUnknown(t *testing.T) {
	mux := New("unknown")
	if mux.Type() != TypeTmux {
		t.Errorf("New(\"unknown\") type = %q, want %q (default)", mux.Type(), TypeTmux)
	}
}

// --- Tmux tests ---

func TestTmuxNixPackages(t *testing.T) {
	mux := &Tmux{}
	pkgs := mux.NixPackages()
	if len(pkgs) != 1 || pkgs[0] != "tmux" {
		t.Errorf("NixPackages() = %v, want [tmux]", pkgs)
	}
}

func TestTmuxInitScript(t *testing.T) {
	mux := &Tmux{}
	windows := []Window{
		{Name: "claude", Command: "claude"},
		{Name: "shell", Command: ""},
	}

	script := mux.InitScript(windows)

	if !strings.Contains(script, "tmux new-session -d -s forage -c /workspace -n claude") {
		t.Error("InitScript should create session with first window")
	}
	if !strings.Contains(script, "tmux send-keys -t forage:claude 'claude' Enter") {
		t.Error("InitScript should send-keys for first window command")
	}
	if !strings.Contains(script, "tmux new-window -t forage -n shell") {
		t.Error("InitScript should create second window")
	}
	if strings.Contains(script, "send-keys -t forage:shell") {
		t.Error("InitScript should not send-keys for empty command")
	}
	if !strings.HasSuffix(strings.TrimSpace(script), "true") {
		t.Error("InitScript should end with 'true'")
	}
}

func TestTmuxAttachCommand(t *testing.T) {
	mux := &Tmux{}
	cmd := mux.AttachCommand()
	if !strings.Contains(cmd, "tmux attach-session -t forage") {
		t.Errorf("AttachCommand() = %q, should contain tmux attach", cmd)
	}
}

func TestTmuxCheckSessionArgs(t *testing.T) {
	mux := &Tmux{}
	args := mux.CheckSessionArgs()
	if len(args) != 4 || args[0] != "tmux" || args[1] != "has-session" {
		t.Errorf("CheckSessionArgs() = %v, unexpected", args)
	}
}

func TestTmuxListWindowsArgs(t *testing.T) {
	mux := &Tmux{}
	args := mux.ListWindowsArgs()
	if len(args) < 3 || args[0] != "tmux" || args[1] != "list-windows" {
		t.Errorf("ListWindowsArgs() = %v, unexpected", args)
	}
}

func TestTmuxParseWindowList(t *testing.T) {
	mux := &Tmux{}
	output := "0:claude\n1:shell\n"
	windows := mux.ParseWindowList(output)
	if len(windows) != 2 {
		t.Fatalf("ParseWindowList() returned %d windows, want 2", len(windows))
	}
	if windows[0] != "0:claude" {
		t.Errorf("windows[0] = %q, want %q", windows[0], "0:claude")
	}
}

func TestTmuxParseWindowList_Empty(t *testing.T) {
	mux := &Tmux{}
	windows := mux.ParseWindowList("")
	if len(windows) != 0 {
		t.Errorf("ParseWindowList(\"\") returned %d windows, want 0", len(windows))
	}
}

func TestTmuxHostConfigMounts_ConfigDir(t *testing.T) {
	tmpDir := t.TempDir()
	tmuxDir := filepath.Join(tmpDir, ".config", "tmux")
	if err := os.MkdirAll(tmuxDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mux := &Tmux{}
	mounts := mux.HostConfigMounts(tmpDir)
	if len(mounts) != 1 {
		t.Fatalf("HostConfigMounts() returned %d mounts, want 1", len(mounts))
	}
	if mounts[0].ContainerPath != "/home/agent/.config/tmux" {
		t.Errorf("ContainerPath = %q", mounts[0].ContainerPath)
	}
	if !mounts[0].ReadOnly {
		t.Error("mount should be read-only")
	}
}

func TestTmuxHostConfigMounts_TmuxConf(t *testing.T) {
	tmpDir := t.TempDir()
	confFile := filepath.Join(tmpDir, ".tmux.conf")
	if err := os.WriteFile(confFile, []byte("# tmux config"), 0o644); err != nil {
		t.Fatal(err)
	}

	mux := &Tmux{}
	mounts := mux.HostConfigMounts(tmpDir)
	if len(mounts) != 1 {
		t.Fatalf("HostConfigMounts() returned %d mounts, want 1", len(mounts))
	}
	if mounts[0].ContainerPath != "/home/agent/.tmux.conf" {
		t.Errorf("ContainerPath = %q", mounts[0].ContainerPath)
	}
}

func TestTmuxHostConfigMounts_None(t *testing.T) {
	tmpDir := t.TempDir()
	mux := &Tmux{}
	mounts := mux.HostConfigMounts(tmpDir)
	if len(mounts) != 0 {
		t.Errorf("HostConfigMounts() returned %d mounts, want 0", len(mounts))
	}
}

func TestTmuxHostConfigMounts_EmptyHome(t *testing.T) {
	mux := &Tmux{}
	mounts := mux.HostConfigMounts("")
	if mounts != nil {
		t.Errorf("HostConfigMounts(\"\") = %v, want nil", mounts)
	}
}

func TestTmuxPromptInstructions(t *testing.T) {
	mux := &Tmux{}
	instructions := mux.PromptInstructions()
	if !strings.Contains(instructions, "tmux") {
		t.Errorf("PromptInstructions() = %q, should mention tmux", instructions)
	}
}

// --- Wezterm tests ---

func TestWeztermNixPackages(t *testing.T) {
	mux := &Wezterm{}
	pkgs := mux.NixPackages()
	if len(pkgs) != 1 || pkgs[0] != "wezterm" {
		t.Errorf("NixPackages() = %v, want [wezterm]", pkgs)
	}
}

func TestWeztermInitScript(t *testing.T) {
	mux := &Wezterm{}
	windows := []Window{
		{Name: "claude", Command: "claude"},
		{Name: "shell", Command: ""},
	}

	script := mux.InitScript(windows)

	if !strings.Contains(script, "wezterm-mux-server --daemonize") {
		t.Error("InitScript should start mux server")
	}
	if !strings.Contains(script, "wezterm cli set-tab-title") {
		t.Error("InitScript should set tab titles")
	}
	if !strings.Contains(script, "wezterm cli spawn --cwd /workspace") {
		t.Error("InitScript should spawn additional tabs")
	}
	if !strings.Contains(script, "wezterm cli send-text") {
		t.Error("InitScript should send-text for commands")
	}
	if !strings.HasSuffix(strings.TrimSpace(script), "true") {
		t.Error("InitScript should end with 'true'")
	}
}

func TestWeztermAttachCommand(t *testing.T) {
	mux := &Wezterm{}
	if cmd := mux.AttachCommand(); cmd != "" {
		t.Errorf("AttachCommand() = %q, want empty (native connect)", cmd)
	}
}

func TestWeztermCheckSessionArgs(t *testing.T) {
	mux := &Wezterm{}
	args := mux.CheckSessionArgs()
	if len(args) != 3 || args[0] != "pgrep" {
		t.Errorf("CheckSessionArgs() = %v, want pgrep command", args)
	}
}

func TestWeztermListWindowsArgs(t *testing.T) {
	mux := &Wezterm{}
	args := mux.ListWindowsArgs()
	if len(args) < 3 || args[0] != "wezterm" {
		t.Errorf("ListWindowsArgs() = %v, unexpected", args)
	}
}

func TestWeztermHostConfigMounts(t *testing.T) {
	mux := &Wezterm{}
	mounts := mux.HostConfigMounts("/home/user")
	if mounts != nil {
		t.Errorf("HostConfigMounts() = %v, want nil", mounts)
	}
}

func TestWeztermPromptInstructions(t *testing.T) {
	mux := &Wezterm{}
	instructions := mux.PromptInstructions()
	if !strings.Contains(instructions, "wezterm") {
		t.Errorf("PromptInstructions() = %q, should mention wezterm", instructions)
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "'hello'"},
		{"it's", `'it'\''s'`},
		{"", "''"},
	}
	for _, tt := range tests {
		got := shellQuote(tt.input)
		if got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

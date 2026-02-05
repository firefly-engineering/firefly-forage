package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/health"
)

func TestTruncatePath(t *testing.T) {
	tests := []struct {
		path   string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"/home/user/workspace", 20, "/home/user/workspace"},
		{"/home/user/very/long/path/to/workspace", 20, "...path/to/workspace"},
		{"", 10, ""},
		{"exactly10!", 10, "exactly10!"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := truncatePath(tt.path, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncatePath(%q, %d) = %q, want %q", tt.path, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestSandboxItemMethods(t *testing.T) {
	meta := &config.SandboxMetadata{
		Name:          "test-sandbox",
		Template:      "claude",
		Workspace:     "/home/user/workspace",
		WorkspaceMode: "jj",
	}

	item := sandboxItem{
		metadata: meta,
		status:   health.StatusHealthy,
		uptime:   "2h30m",
	}

	t.Run("Title", func(t *testing.T) {
		if got := item.Title(); got != "test-sandbox" {
			t.Errorf("Title() = %q, want %q", got, "test-sandbox")
		}
	})

	t.Run("FilterValue", func(t *testing.T) {
		if got := item.FilterValue(); got != "test-sandbox" {
			t.Errorf("FilterValue() = %q, want %q", got, "test-sandbox")
		}
	})

	t.Run("Description", func(t *testing.T) {
		desc := item.Description()
		if !strings.Contains(desc, "✓") {
			t.Error("Description should contain healthy status icon")
		}
		if !strings.Contains(desc, "claude") {
			t.Error("Description should contain template name")
		}
		if !strings.Contains(desc, "jj") {
			t.Error("Description should contain workspace mode")
		}
		if !strings.Contains(desc, "2h30m") {
			t.Error("Description should contain uptime")
		}
	})

	t.Run("Description with empty mode", func(t *testing.T) {
		meta := &config.SandboxMetadata{
			Name:          "test",
			WorkspaceMode: "",
		}
		item := sandboxItem{metadata: meta, status: health.StatusStopped}
		desc := item.Description()
		if !strings.Contains(desc, "dir") {
			t.Error("Description should default to 'dir' mode")
		}
	})
}

func TestSandboxItemStatusIcons(t *testing.T) {
	tests := []struct {
		status health.Status
		icon   string
	}{
		{health.StatusHealthy, "✓"},
		{health.StatusUnhealthy, "⚠"},
		{health.StatusNoTmux, "○"},
		{health.StatusStopped, "●"},
	}

	for _, tt := range tests {
		t.Run(tt.icon, func(t *testing.T) {
			item := sandboxItem{
				metadata: &config.SandboxMetadata{Name: "test"},
				status:   tt.status,
			}
			desc := item.Description()
			if !strings.Contains(desc, tt.icon) {
				t.Errorf("Description for status %v should contain %q", tt.status, tt.icon)
			}
		})
	}
}

func TestModelKeyHandling(t *testing.T) {
	meta := &config.SandboxMetadata{
		Name:     "test-sandbox",
		Template: "claude",
		Port:     2222,
	}

	paths := &config.Paths{
		ConfigDir:    "/etc/firefly-forage",
		StateDir:     "/var/lib/firefly-forage",
		SandboxesDir: "/var/lib/firefly-forage/sandboxes",
	}

	t.Run("quit with q", func(t *testing.T) {
		m := NewPicker([]*config.SandboxMetadata{meta}, paths)
		newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
		model := newModel.(Model)

		if model.result.Action != ActionQuit {
			t.Errorf("Action = %v, want ActionQuit", model.result.Action)
		}
		if !model.quitting {
			t.Error("Model should be quitting")
		}
		if cmd == nil {
			t.Error("Should return tea.Quit command")
		}
	})

	t.Run("quit with esc", func(t *testing.T) {
		m := NewPicker([]*config.SandboxMetadata{meta}, paths)
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		model := newModel.(Model)

		if model.result.Action != ActionQuit {
			t.Errorf("Action = %v, want ActionQuit", model.result.Action)
		}
	})

	t.Run("new sandbox with n", func(t *testing.T) {
		m := NewPicker([]*config.SandboxMetadata{meta}, paths)
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		model := newModel.(Model)

		if model.result.Action != ActionNew {
			t.Errorf("Action = %v, want ActionNew", model.result.Action)
		}
	})

	t.Run("window size update", func(t *testing.T) {
		m := NewPicker([]*config.SandboxMetadata{meta}, paths)
		newModel, cmd := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
		model := newModel.(Model)

		if model.width != 100 {
			t.Errorf("Width = %d, want 100", model.width)
		}
		if model.height != 50 {
			t.Errorf("Height = %d, want 50", model.height)
		}
		if cmd != nil {
			t.Error("Window size update should not return a command")
		}
	})
}

func TestModelInit(t *testing.T) {
	m := Model{}
	cmd := m.Init()
	if cmd != nil {
		t.Error("Init() should return nil")
	}
}

func TestModelView(t *testing.T) {
	meta := &config.SandboxMetadata{
		Name:     "test-sandbox",
		Template: "claude",
	}

	paths := &config.Paths{
		SandboxesDir: "/var/lib/firefly-forage/sandboxes",
	}

	t.Run("normal view contains help", func(t *testing.T) {
		m := NewPicker([]*config.SandboxMetadata{meta}, paths)
		view := m.View()

		if !strings.Contains(view, "[enter] Attach") {
			t.Error("View should contain attach help")
		}
		if !strings.Contains(view, "[n] New") {
			t.Error("View should contain new help")
		}
		if !strings.Contains(view, "[q] Quit") {
			t.Error("View should contain quit help")
		}
	})

	t.Run("quitting view is empty", func(t *testing.T) {
		m := NewPicker([]*config.SandboxMetadata{meta}, paths)
		m.quitting = true
		view := m.View()

		if view != "" {
			t.Errorf("Quitting view should be empty, got %q", view)
		}
	})
}

func TestModelResult(t *testing.T) {
	m := Model{
		result: PickerResult{
			Action: ActionAttach,
			Sandbox: &config.SandboxMetadata{
				Name: "test",
			},
		},
	}

	result := m.Result()
	if result.Action != ActionAttach {
		t.Errorf("Action = %v, want ActionAttach", result.Action)
	}
	if result.Sandbox.Name != "test" {
		t.Errorf("Sandbox.Name = %q, want %q", result.Sandbox.Name, "test")
	}
}

func TestRunPickerEmptySandboxes(t *testing.T) {
	paths := &config.Paths{
		SandboxesDir: "/var/lib/firefly-forage/sandboxes",
	}

	result, err := RunPicker([]*config.SandboxMetadata{}, paths)
	if err != nil {
		t.Fatalf("RunPicker with empty sandboxes failed: %v", err)
	}

	if result.Action != ActionNew {
		t.Errorf("Empty sandboxes should return ActionNew, got %v", result.Action)
	}
}

func TestSimplePicker(t *testing.T) {
	paths := &config.Paths{
		SandboxesDir: "/var/lib/firefly-forage/sandboxes",
	}

	t.Run("empty sandboxes", func(t *testing.T) {
		output := SimplePicker([]*config.SandboxMetadata{}, paths)

		if !strings.Contains(output, "No sandboxes found") {
			t.Error("Should indicate no sandboxes found")
		}
		if !strings.Contains(output, "forage-ctl up") {
			t.Error("Should show how to create sandbox")
		}
	})

	t.Run("with sandboxes", func(t *testing.T) {
		sandboxes := []*config.SandboxMetadata{
			{
				Name:      "sandbox1",
				Template:  "claude",
				Port:      2222,
				Workspace: "/home/user/project1",
			},
			{
				Name:      "sandbox2",
				Template:  "aider",
				Port:      2223,
				Workspace: "/home/user/project2",
			},
		}

		output := SimplePicker(sandboxes, paths)

		if !strings.Contains(output, "Firefly Forage") {
			t.Error("Should contain title")
		}
		if !strings.Contains(output, "sandbox1") {
			t.Error("Should contain first sandbox name")
		}
		if !strings.Contains(output, "sandbox2") {
			t.Error("Should contain second sandbox name")
		}
		if !strings.Contains(output, "claude") {
			t.Error("Should contain template name")
		}
		if !strings.Contains(output, "2222") {
			t.Error("Should contain port number")
		}
	})
}

func TestActionConstants(t *testing.T) {
	// Verify action constants have distinct values
	actions := []Action{ActionNone, ActionAttach, ActionNew, ActionDown, ActionQuit}
	seen := make(map[Action]bool)

	for _, a := range actions {
		if seen[a] {
			t.Errorf("Duplicate action value: %v", a)
		}
		seen[a] = true
	}
}

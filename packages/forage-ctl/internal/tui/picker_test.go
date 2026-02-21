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
		if !strings.Contains(desc, "/home/user/workspace") {
			t.Error("Description should contain workspace path")
		}
	})

	t.Run("Description with direct mode", func(t *testing.T) {
		meta := &config.SandboxMetadata{
			Name:          "test",
			WorkspaceMode: "direct",
		}
		item := sandboxItem{metadata: meta, status: health.StatusStopped}
		desc := item.Description()
		if !strings.Contains(desc, "direct") {
			t.Error("Description should contain 'direct' mode")
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
		{health.StatusNoMux, "○"},
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
		Name:          "test-sandbox",
		Template:      "claude",
		NetworkSlot:   1,
		WorkspaceMode: "direct",
		Workspace:     "/home/user/project",
	}

	paths := &config.Paths{
		ConfigDir:    "/etc/firefly-forage",
		StateDir:     "/var/lib/firefly-forage",
		SandboxesDir: "/var/lib/firefly-forage/sandboxes",
	}

	opts := PickerOptions{}

	t.Run("quit with q", func(t *testing.T) {
		m := NewPicker([]*config.SandboxMetadata{meta}, paths, nil, opts)
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
		m := NewPicker([]*config.SandboxMetadata{meta}, paths, nil, opts)
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		model := newModel.(Model)

		if model.result.Action != ActionQuit {
			t.Errorf("Action = %v, want ActionQuit", model.result.Action)
		}
	})

	t.Run("new sandbox with n (AllowCreate=false)", func(t *testing.T) {
		m := NewPicker([]*config.SandboxMetadata{meta}, paths, nil, opts)
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		model := newModel.(Model)

		if model.result.Action != ActionNew {
			t.Errorf("Action = %v, want ActionNew", model.result.Action)
		}
	})

	t.Run("new sandbox with n (AllowCreate=true) opens wizard", func(t *testing.T) {
		createOpts := PickerOptions{AllowCreate: true}
		m := NewPicker([]*config.SandboxMetadata{meta}, paths, nil, createOpts)
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		model := newModel.(Model)

		if model.screen != screenWizard {
			t.Error("Expected screen to be screenWizard")
		}
		if model.wizard == nil {
			t.Error("Expected wizard to be initialized")
		}
	})

	t.Run("window size update", func(t *testing.T) {
		m := NewPicker([]*config.SandboxMetadata{meta}, paths, nil, opts)
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
		Name:          "test-sandbox",
		Template:      "claude",
		WorkspaceMode: "direct",
		Workspace:     "/home/user/project",
	}

	paths := &config.Paths{
		SandboxesDir: "/var/lib/firefly-forage/sandboxes",
	}

	opts := PickerOptions{}

	t.Run("normal view contains help", func(t *testing.T) {
		m := NewPicker([]*config.SandboxMetadata{meta}, paths, nil, opts)
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
		m := NewPicker([]*config.SandboxMetadata{meta}, paths, nil, opts)
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

	result, err := RunPicker([]*config.SandboxMetadata{}, paths, nil, PickerOptions{})
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
		output := SimplePicker([]*config.SandboxMetadata{}, paths, nil)

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
				Name:        "sandbox1",
				Template:    "claude",
				NetworkSlot: 1,
				Workspace:   "/home/user/project1",
			},
			{
				Name:        "sandbox2",
				Template:    "aider",
				NetworkSlot: 2,
				Workspace:   "/home/user/project2",
			},
		}

		output := SimplePicker(sandboxes, paths, nil)

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
		if !strings.Contains(output, "10.100.1.2") {
			t.Error("Should contain container IP")
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

func TestPickerResultWithCreateOptions(t *testing.T) {
	result := PickerResult{
		Action: ActionNew,
		CreateOptions: &CreateOptions{
			Name:     "test-sandbox",
			Template: "claude",
			RepoPath: "/home/user/project",
			Direct:   true,
		},
	}

	if result.Action != ActionNew {
		t.Errorf("Action = %v, want ActionNew", result.Action)
	}
	if result.CreateOptions == nil {
		t.Fatal("CreateOptions should not be nil")
	}
	if result.CreateOptions.Name != "test-sandbox" {
		t.Errorf("Name = %q, want %q", result.CreateOptions.Name, "test-sandbox")
	}
	if !result.CreateOptions.Direct {
		t.Error("Direct should be true")
	}
}

func TestGroupedListInPicker(t *testing.T) {
	sandboxes := []*config.SandboxMetadata{
		{Name: "sb1", Template: "claude", SourceRepo: "/home/user/repo-a", Workspace: "/var/lib/ws/sb1", WorkspaceMode: "jj"},
		{Name: "sb2", Template: "aider", SourceRepo: "/home/user/repo-b", Workspace: "/var/lib/ws/sb2", WorkspaceMode: "jj"},
		{Name: "sb3", Template: "claude", SourceRepo: "/home/user/repo-a", Workspace: "/var/lib/ws/sb3", WorkspaceMode: "jj"},
	}

	paths := &config.Paths{
		SandboxesDir: "/var/lib/firefly-forage/sandboxes",
	}

	m := NewPicker(sandboxes, paths, nil, PickerOptions{})

	// The list should have headers + sandbox items
	items := m.list.Items()
	if len(items) != 5 { // 2 headers + 3 sandboxes
		t.Errorf("expected 5 items (2 headers + 3 sandboxes), got %d", len(items))
	}

	// Initial selection should skip the header
	selected := m.list.SelectedItem()
	if _, ok := selected.(headerItem); ok {
		t.Error("initial selection should not be a headerItem")
	}
}

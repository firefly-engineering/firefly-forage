package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSuggestName(t *testing.T) {
	tests := []struct {
		path     string
		template string
		want     string
	}{
		{"/home/user/my-project", "claude", "my-project-claude"},
		{"/home/user/MyProject", "aider", "myproject-aider"},
		{"/tmp/test", "claude", "test-claude"},
		{"/home/user/repo with spaces", "claude", "repo-with-spaces-claude"},
		{"", "claude", "sandbox-claude"},
		{"/", "claude", "sandbox-claude"},
	}

	for _, tt := range tests {
		t.Run(tt.path+"/"+tt.template, func(t *testing.T) {
			got := suggestName(tt.path, tt.template)
			if got != tt.want {
				t.Errorf("suggestName(%q, %q) = %q, want %q", tt.path, tt.template, got, tt.want)
			}
		})
	}
}

func TestSuggestNameTruncation(t *testing.T) {
	longPath := "/home/user/" + strings.Repeat("a", 60)
	name := suggestName(longPath, "claude")
	if len(name) > 63 {
		t.Errorf("name length %d exceeds 63", len(name))
	}
}

func TestWizardStepTransitions(t *testing.T) {
	t.Run("path to template", func(t *testing.T) {
		w := newWizardModel("")
		if w.step != stepPath {
			t.Fatalf("initial step = %v, want stepPath", w.step)
		}

		// Type a path
		w.pathInput.SetValue("/tmp/test")

		// Press enter to advance
		done, opts, _ := w.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if done {
			t.Error("should not be done after path step")
		}
		if opts != nil {
			t.Error("opts should be nil")
		}
		if w.step != stepTemplate {
			t.Errorf("step = %v, want stepTemplate", w.step)
		}
	})

	t.Run("empty path rejected", func(t *testing.T) {
		w := newWizardModel("")
		w.pathInput.SetValue("")

		done, _, _ := w.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if done {
			t.Error("should not be done")
		}
		if w.step != stepPath {
			t.Error("should stay on stepPath with empty input")
		}
	})

	t.Run("template to name", func(t *testing.T) {
		w := newWizardModel("")
		w.selectedPath = "/tmp/test"
		w.step = stepTemplate
		w.loadTemplates()

		// Press enter to select template
		done, opts, _ := w.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if done {
			t.Error("should not be done")
		}
		if opts != nil {
			t.Error("opts should be nil")
		}
		if w.step != stepName {
			t.Errorf("step = %v, want stepName", w.step)
		}
		// Name should be auto-suggested
		if w.nameInput.Value() == "" {
			t.Error("name should be auto-suggested")
		}
	})

	t.Run("name to confirm", func(t *testing.T) {
		w := newWizardModel("")
		w.step = stepName
		w.selectedPath = "/tmp/test"
		w.selectedTemplate = "claude"
		w.nameInput.SetValue("test-claude")

		done, opts, _ := w.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if done {
			t.Error("should not be done")
		}
		if opts != nil {
			t.Error("opts should be nil")
		}
		if w.step != stepConfirm {
			t.Errorf("step = %v, want stepConfirm", w.step)
		}
	})

	t.Run("name to advanced with ctrl+a", func(t *testing.T) {
		w := newWizardModel("")
		w.step = stepName
		w.selectedPath = "/tmp/test"
		w.selectedTemplate = "claude"
		w.nameInput.SetValue("test-claude")

		done, _, _ := w.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
		if done {
			t.Error("should not be done")
		}
		if w.step != stepAdvanced {
			t.Errorf("step = %v, want stepAdvanced", w.step)
		}
	})

	t.Run("invalid name rejected", func(t *testing.T) {
		w := newWizardModel("")
		w.step = stepName
		w.nameInput.SetValue("INVALID NAME")

		w.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if w.step != stepName {
			t.Error("should stay on stepName with invalid name")
		}
	})
}

func TestWizardConfirm(t *testing.T) {
	t.Run("enter confirms and produces CreateOptions", func(t *testing.T) {
		w := newWizardModel("")
		w.step = stepConfirm
		w.selectedPath = "/home/user/project"
		w.selectedTemplate = "claude"
		w.selectedName = "project-claude"
		w.direct = true
		w.noTmuxConfig = true

		done, opts, _ := w.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if !done {
			t.Error("should be done after confirm")
		}
		if opts == nil {
			t.Fatal("opts should not be nil")
		}
		if opts.Name != "project-claude" {
			t.Errorf("Name = %q, want %q", opts.Name, "project-claude")
		}
		if opts.Template != "claude" {
			t.Errorf("Template = %q, want %q", opts.Template, "claude")
		}
		if opts.RepoPath != "/home/user/project" {
			t.Errorf("RepoPath = %q, want %q", opts.RepoPath, "/home/user/project")
		}
		if !opts.Direct {
			t.Error("Direct should be true")
		}
		if !opts.NoTmuxConfig {
			t.Error("NoTmuxConfig should be true")
		}
	})

	t.Run("n restarts wizard", func(t *testing.T) {
		w := newWizardModel("")
		w.step = stepConfirm
		w.selectedPath = "/home/user/project"
		w.selectedTemplate = "claude"
		w.selectedName = "project-claude"

		done, opts, _ := w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		if done {
			t.Error("should not be done after restart")
		}
		if opts != nil {
			t.Error("opts should be nil")
		}
		if w.step != stepPath {
			t.Errorf("step = %v, want stepPath", w.step)
		}
		if w.selectedPath != "" {
			t.Error("path should be cleared")
		}
	})
}

func TestWizardCancel(t *testing.T) {
	t.Run("ctrl+c cancels", func(t *testing.T) {
		w := newWizardModel("")
		w.step = stepName

		done, opts, _ := w.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		if !done {
			t.Error("should be done after cancel")
		}
		if opts != nil {
			t.Error("opts should be nil (cancelled)")
		}
	})

	t.Run("esc at first step cancels", func(t *testing.T) {
		w := newWizardModel("")
		w.step = stepPath

		done, opts, _ := w.Update(tea.KeyMsg{Type: tea.KeyEsc})
		if !done {
			t.Error("should be done after esc at first step")
		}
		if opts != nil {
			t.Error("opts should be nil (cancelled)")
		}
	})

	t.Run("esc at later step goes back", func(t *testing.T) {
		w := newWizardModel("")
		w.step = stepName
		w.selectedPath = "/tmp/test"
		w.selectedTemplate = "claude"

		done, _, _ := w.Update(tea.KeyMsg{Type: tea.KeyEsc})
		if done {
			t.Error("should not be done")
		}
		if w.step != stepTemplate {
			t.Errorf("step = %v, want stepTemplate", w.step)
		}
	})
}

func TestWizardAdvanced(t *testing.T) {
	t.Run("toggle direct", func(t *testing.T) {
		w := newWizardModel("")
		w.step = stepAdvanced
		w.advCursor = advDirect

		if w.direct {
			t.Error("direct should start false")
		}

		// Space toggles
		w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
		if !w.direct {
			t.Error("direct should be true after toggle")
		}

		w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
		if w.direct {
			t.Error("direct should be false after second toggle")
		}
	})

	t.Run("toggle noTmuxConfig", func(t *testing.T) {
		w := newWizardModel("")
		w.step = stepAdvanced
		w.advCursor = advNoTmuxConfig

		w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
		if !w.noTmuxConfig {
			t.Error("noTmuxConfig should be true after toggle")
		}
	})

	t.Run("navigation", func(t *testing.T) {
		w := newWizardModel("")
		w.step = stepAdvanced
		w.advCursor = advDirect

		// Move down
		w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		if w.advCursor != advNoTmuxConfig {
			t.Errorf("cursor = %v, want advNoTmuxConfig", w.advCursor)
		}

		// Move up
		w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		if w.advCursor != advDirect {
			t.Errorf("cursor = %v, want advDirect", w.advCursor)
		}
	})

	t.Run("enter advances to confirm", func(t *testing.T) {
		w := newWizardModel("")
		w.step = stepAdvanced

		done, _, _ := w.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if done {
			t.Error("should not be done")
		}
		if w.step != stepConfirm {
			t.Errorf("step = %v, want stepConfirm", w.step)
		}
	})
}

func TestWizardView(t *testing.T) {
	t.Run("path step shows input", func(t *testing.T) {
		w := newWizardModel("")
		view := w.View()
		if !strings.Contains(view, "Create New Sandbox") {
			t.Error("should contain title")
		}
		if !strings.Contains(view, "Project directory") {
			t.Error("should contain path label")
		}
		if !strings.Contains(view, "1. Path") {
			t.Error("should contain progress bar")
		}
	})

	t.Run("confirm step shows values", func(t *testing.T) {
		w := newWizardModel("")
		w.step = stepConfirm
		w.selectedPath = "/home/user/project"
		w.selectedTemplate = "claude"
		w.selectedName = "project-claude"

		view := w.View()
		if !strings.Contains(view, "/home/user/project") {
			t.Error("should show path")
		}
		if !strings.Contains(view, "claude") {
			t.Error("should show template")
		}
		if !strings.Contains(view, "project-claude") {
			t.Error("should show name")
		}
	})
}

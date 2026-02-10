package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
)

// wizardStep identifies the current step.
type wizardStep int

const (
	stepPath wizardStep = iota
	stepTemplate
	stepName
	stepAdvanced
	stepConfirm
)

// advancedField identifies a field in the advanced step.
type advancedField int

const (
	advDirect advancedField = iota
	advNoMuxConfig
	advGitUser
	advGitEmail
	advSSHKeyPath
	advFieldCount
)

// wizardModel drives the multi-step creation wizard.
type wizardModel struct {
	step         wizardStep
	templatesDir string

	// Step 1: path
	pathInput textinput.Model

	// Step 2: template
	templateList list.Model
	templates    []string

	// Step 3: name
	nameInput textinput.Model

	// Step 4: advanced
	advCursor      advancedField
	direct         bool
	noMuxConfig   bool
	gitUserInput   textinput.Model
	gitEmailInput  textinput.Model
	sshKeyInput    textinput.Model

	// Collected values
	selectedPath     string
	selectedTemplate string
	selectedName     string

	width  int
	height int
}

// templateItem implements list.Item for template selection.
type templateItem struct {
	name        string
	description string
}

func (t templateItem) Title() string       { return t.name }
func (t templateItem) Description() string { return t.description }
func (t templateItem) FilterValue() string { return t.name }

// wizardStyles
var (
	wizardTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("39")).
				MarginBottom(1)

	wizardStepStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	wizardActiveStepStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("39"))

	wizardLabelStyle = lipgloss.NewStyle().
				Bold(true).
				MarginBottom(1)

	wizardValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("39"))

	wizardDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
)

func newWizardModel(templatesDir string) wizardModel {
	pi := textinput.New()
	pi.Placeholder = "/path/to/project"
	pi.Focus()
	pi.CharLimit = 256
	pi.Width = 60
	pi.ShowSuggestions = true

	ni := textinput.New()
	ni.Placeholder = "sandbox-name"
	ni.CharLimit = 63
	ni.Width = 40

	gui := textinput.New()
	gui.Placeholder = "Agent Name"
	gui.CharLimit = 128
	gui.Width = 50

	gei := textinput.New()
	gei.Placeholder = "agent@example.com"
	gei.CharLimit = 128
	gei.Width = 50

	ski := textinput.New()
	ski.Placeholder = "/path/to/ssh/key"
	ski.CharLimit = 256
	ski.Width = 60

	return wizardModel{
		step:          stepPath,
		templatesDir:  templatesDir,
		pathInput:     pi,
		nameInput:     ni,
		gitUserInput:  gui,
		gitEmailInput: gei,
		sshKeyInput:   ski,
	}
}

func (w *wizardModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update processes a message and returns (done, createOptions, cmd).
// done=true with non-nil opts means wizard completed successfully.
// done=true with nil opts means wizard was cancelled.
func (w *wizardModel) Update(msg tea.Msg) (bool, *CreateOptions, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyCtrlC:
			return true, nil, nil
		case tea.KeyEsc:
			return w.handleBack()
		}
	}

	switch w.step {
	case stepPath:
		return w.updatePath(msg)
	case stepTemplate:
		return w.updateTemplate(msg)
	case stepName:
		return w.updateName(msg)
	case stepAdvanced:
		return w.updateAdvanced(msg)
	case stepConfirm:
		return w.updateConfirm(msg)
	}

	return false, nil, nil
}

func (w *wizardModel) handleBack() (bool, *CreateOptions, tea.Cmd) {
	switch w.step {
	case stepPath:
		// Esc at first step cancels wizard
		return true, nil, nil
	case stepTemplate:
		w.step = stepPath
		w.pathInput.Focus()
		return false, nil, textinput.Blink
	case stepName:
		w.step = stepTemplate
		w.nameInput.Blur()
		return false, nil, nil
	case stepAdvanced:
		w.step = stepName
		w.nameInput.Focus()
		return false, nil, textinput.Blink
	case stepConfirm:
		w.step = stepName
		w.nameInput.Focus()
		return false, nil, textinput.Blink
	}
	return false, nil, nil
}

func (w *wizardModel) updatePath(msg tea.Msg) (bool, *CreateOptions, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyEnter {
		path := strings.TrimSpace(w.pathInput.Value())
		if path == "" {
			return false, nil, nil
		}
		w.selectedPath = path
		w.step = stepTemplate
		w.pathInput.Blur()
		w.loadTemplates()
		return false, nil, nil
	}

	var cmd tea.Cmd
	w.pathInput, cmd = w.pathInput.Update(msg)

	// Update path suggestions after each keystroke
	w.updatePathSuggestions()

	return false, nil, cmd
}

func (w *wizardModel) updateTemplate(msg tea.Msg) (bool, *CreateOptions, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyEnter {
		if item, ok := w.templateList.SelectedItem().(templateItem); ok {
			w.selectedTemplate = item.name
			w.step = stepName
			w.nameInput.Focus()
			// Auto-suggest name
			suggested := suggestName(w.selectedPath, w.selectedTemplate)
			w.nameInput.SetValue(suggested)
			return false, nil, textinput.Blink
		}
		return false, nil, nil
	}

	var cmd tea.Cmd
	w.templateList, cmd = w.templateList.Update(msg)
	return false, nil, cmd
}

func (w *wizardModel) updateName(msg tea.Msg) (bool, *CreateOptions, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyEnter:
			name := strings.TrimSpace(w.nameInput.Value())
			if name == "" {
				return false, nil, nil
			}
			if err := config.ValidateSandboxName(name); err != nil {
				return false, nil, nil
			}
			w.selectedName = name
			w.step = stepConfirm
			w.nameInput.Blur()
			return false, nil, nil
		case tea.KeyCtrlA:
			w.selectedName = strings.TrimSpace(w.nameInput.Value())
			w.step = stepAdvanced
			w.nameInput.Blur()
			return false, nil, nil
		}
	}

	var cmd tea.Cmd
	w.nameInput, cmd = w.nameInput.Update(msg)
	return false, nil, cmd
}

func (w *wizardModel) isTextInputField() bool {
	return w.advCursor == advGitUser || w.advCursor == advGitEmail || w.advCursor == advSSHKeyPath
}

func (w *wizardModel) activeTextInput() *textinput.Model {
	switch w.advCursor {
	case advGitUser:
		return &w.gitUserInput
	case advGitEmail:
		return &w.gitEmailInput
	case advSSHKeyPath:
		return &w.sshKeyInput
	}
	return nil
}

func (w *wizardModel) blurAllAdvTextInputs() {
	w.gitUserInput.Blur()
	w.gitEmailInput.Blur()
	w.sshKeyInput.Blur()
}

func (w *wizardModel) focusCurrentTextField() tea.Cmd {
	w.blurAllAdvTextInputs()
	if ti := w.activeTextInput(); ti != nil {
		ti.Focus()
		return textinput.Blink
	}
	return nil
}

func (w *wizardModel) updateAdvanced(msg tea.Msg) (bool, *CreateOptions, tea.Cmd) {
	// If we're on a text input field, forward keystrokes to it
	if w.isTextInputField() {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.Type {
			case tea.KeyEnter:
				w.blurAllAdvTextInputs()
				w.step = stepConfirm
				return false, nil, nil
			case tea.KeyUp:
				w.blurAllAdvTextInputs()
				w.advCursor = (w.advCursor - 1 + advFieldCount) % advFieldCount
				return false, nil, w.focusCurrentTextField()
			case tea.KeyDown:
				w.blurAllAdvTextInputs()
				w.advCursor = (w.advCursor + 1) % advFieldCount
				return false, nil, w.focusCurrentTextField()
			case tea.KeyTab:
				w.blurAllAdvTextInputs()
				w.advCursor = (w.advCursor + 1) % advFieldCount
				return false, nil, w.focusCurrentTextField()
			}
		}
		// Forward to text input
		if ti := w.activeTextInput(); ti != nil {
			var cmd tea.Cmd
			*ti, cmd = ti.Update(msg)
			return false, nil, cmd
		}
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "enter":
			w.step = stepConfirm
			return false, nil, nil
		case "j", "down":
			w.advCursor = (w.advCursor + 1) % advFieldCount
			return false, nil, w.focusCurrentTextField()
		case "k", "up":
			w.advCursor = (w.advCursor - 1 + advFieldCount) % advFieldCount
			return false, nil, w.focusCurrentTextField()
		case "tab":
			w.advCursor = (w.advCursor + 1) % advFieldCount
			return false, nil, w.focusCurrentTextField()
		case " ":
			switch w.advCursor {
			case advDirect:
				w.direct = !w.direct
			case advNoMuxConfig:
				w.noMuxConfig = !w.noMuxConfig
			}
			return false, nil, nil
		}
	}
	return false, nil, nil
}

func (w *wizardModel) updateConfirm(msg tea.Msg) (bool, *CreateOptions, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "enter", "y":
			return true, &CreateOptions{
				Name:         w.selectedName,
				Template:     w.selectedTemplate,
				RepoPath:     w.selectedPath,
				Direct:       w.direct,
				NoMuxConfig: w.noMuxConfig,
				GitUser:      strings.TrimSpace(w.gitUserInput.Value()),
				GitEmail:     strings.TrimSpace(w.gitEmailInput.Value()),
				SSHKeyPath:   strings.TrimSpace(w.sshKeyInput.Value()),
			}, nil
		case "n":
			// Restart wizard
			w.step = stepPath
			w.pathInput.SetValue("")
			w.pathInput.Focus()
			w.selectedPath = ""
			w.selectedTemplate = ""
			w.selectedName = ""
			w.direct = false
			w.noMuxConfig = false
			w.gitUserInput.SetValue("")
			w.gitEmailInput.SetValue("")
			w.sshKeyInput.SetValue("")
			return false, nil, textinput.Blink
		}
	}
	return false, nil, nil
}

func (w *wizardModel) View() string {
	var b strings.Builder

	b.WriteString(wizardTitleStyle.Render("Create New Sandbox"))
	b.WriteString("\n")
	b.WriteString(w.progressBar())
	b.WriteString("\n\n")

	switch w.step {
	case stepPath:
		b.WriteString(wizardLabelStyle.Render("Project directory:"))
		b.WriteString("\n")
		b.WriteString(w.pathInput.View())
		b.WriteString("\n\n")
		b.WriteString(wizardDimStyle.Render("Enter the path to your project. Tab to complete."))
	case stepTemplate:
		b.WriteString(wizardLabelStyle.Render("Select template:"))
		b.WriteString("\n")
		b.WriteString(w.templateList.View())
	case stepName:
		b.WriteString(wizardLabelStyle.Render("Sandbox name:"))
		b.WriteString("\n")
		b.WriteString(w.nameInput.View())
		b.WriteString("\n\n")
		b.WriteString(wizardDimStyle.Render("Enter to confirm, Ctrl+A for advanced options."))
	case stepAdvanced:
		b.WriteString(wizardLabelStyle.Render("Advanced options:"))
		b.WriteString("\n\n")
		b.WriteString(w.renderToggle(advDirect, "Direct mount", "Skip VCS isolation, mount directory directly"))
		b.WriteString("\n")
		b.WriteString(w.renderToggle(advNoMuxConfig, "No mux config", "Don't mount host multiplexer config"))
		b.WriteString("\n")
		b.WriteString(w.renderTextInput(advGitUser, "Git user", "Git user.name for agent commits", &w.gitUserInput))
		b.WriteString("\n")
		b.WriteString(w.renderTextInput(advGitEmail, "Git email", "Git user.email for agent commits", &w.gitEmailInput))
		b.WriteString("\n")
		b.WriteString(w.renderTextInput(advSSHKeyPath, "SSH key path", "Host path to SSH private key for push access", &w.sshKeyInput))
		b.WriteString("\n\n")
		b.WriteString(wizardDimStyle.Render("Space/type to edit, Enter to continue, Esc to go back."))
	case stepConfirm:
		b.WriteString(wizardLabelStyle.Render("Confirm:"))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("  Path:     %s\n", wizardValueStyle.Render(w.selectedPath)))
		b.WriteString(fmt.Sprintf("  Template: %s\n", wizardValueStyle.Render(w.selectedTemplate)))
		b.WriteString(fmt.Sprintf("  Name:     %s\n", wizardValueStyle.Render(w.selectedName)))
		if w.direct {
			b.WriteString(fmt.Sprintf("  Direct:   %s\n", wizardValueStyle.Render("yes")))
		}
		if w.noMuxConfig {
			b.WriteString(fmt.Sprintf("  No mux:   %s\n", wizardValueStyle.Render("yes")))
		}
		if v := strings.TrimSpace(w.gitUserInput.Value()); v != "" {
			b.WriteString(fmt.Sprintf("  Git user: %s\n", wizardValueStyle.Render(v)))
		}
		if v := strings.TrimSpace(w.gitEmailInput.Value()); v != "" {
			b.WriteString(fmt.Sprintf("  Git email:%s\n", wizardValueStyle.Render(v)))
		}
		if v := strings.TrimSpace(w.sshKeyInput.Value()); v != "" {
			b.WriteString(fmt.Sprintf("  SSH key:  %s\n", wizardValueStyle.Render(v)))
		}
		b.WriteString("\n")
		b.WriteString(wizardDimStyle.Render("Enter to create, n to restart, Esc to go back."))
	}

	return b.String()
}

func (w *wizardModel) progressBar() string {
	steps := []struct {
		num  int
		name string
	}{
		{1, "Path"},
		{2, "Template"},
		{3, "Name"},
		{4, "Confirm"},
	}

	var parts []string
	for _, s := range steps {
		label := fmt.Sprintf("%d. %s", s.num, s.name)
		currentStep := int(w.step) + 1
		// Map stepAdvanced to stepName for progress display
		if w.step == stepAdvanced {
			currentStep = int(stepName) + 1
		}
		if s.num == currentStep {
			parts = append(parts, wizardActiveStepStyle.Render(label))
		} else {
			parts = append(parts, wizardStepStyle.Render(label))
		}
	}

	return strings.Join(parts, wizardDimStyle.Render(" > "))
}

func (w *wizardModel) renderToggle(field advancedField, name, desc string) string {
	cursor := " "
	if w.advCursor == field {
		cursor = ">"
	}

	checked := " "
	switch field {
	case advDirect:
		if w.direct {
			checked = "x"
		}
	case advNoMuxConfig:
		if w.noMuxConfig {
			checked = "x"
		}
	}

	line := fmt.Sprintf("  %s [%s] %s", cursor, checked, name)
	if w.advCursor == field {
		return selectedStyle.Render(line) + "\n" + wizardDimStyle.Render("      "+desc)
	}
	return line + "\n" + wizardDimStyle.Render("      "+desc)
}

func (w *wizardModel) renderTextInput(field advancedField, name, desc string, ti *textinput.Model) string {
	cursor := " "
	if w.advCursor == field {
		cursor = ">"
	}

	val := strings.TrimSpace(ti.Value())
	if w.advCursor == field {
		// Show active text input
		line := fmt.Sprintf("  %s %s: %s", cursor, name, ti.View())
		return selectedStyle.Render(line) + "\n" + wizardDimStyle.Render("      "+desc)
	}
	if val == "" {
		line := fmt.Sprintf("  %s %s: (not set)", cursor, name)
		return line + "\n" + wizardDimStyle.Render("      "+desc)
	}
	line := fmt.Sprintf("  %s %s: %s", cursor, name, val)
	return line + "\n" + wizardDimStyle.Render("      "+desc)
}

func (w *wizardModel) loadTemplates() {
	w.templates = nil
	var items []list.Item

	if w.templatesDir != "" {
		templates, err := config.ListTemplates(w.templatesDir)
		if err == nil {
			for _, t := range templates {
				w.templates = append(w.templates, t.Name)
				items = append(items, templateItem{name: t.Name, description: t.Description})
			}
		}
	}

	if len(items) == 0 {
		// If no templates loaded, add a placeholder
		items = append(items, templateItem{name: "default", description: "Default template"})
		w.templates = append(w.templates, "default")
	}

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = selectedStyle
	delegate.Styles.SelectedDesc = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	l := list.New(items, delegate, 60, 10)
	l.Title = ""
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	if w.width > 0 {
		l.SetWidth(w.width - 4)
	}
	if w.height > 0 {
		l.SetHeight(w.height - 10)
	}

	w.templateList = l
}

func (w *wizardModel) updatePathSuggestions() {
	val := w.pathInput.Value()
	if val == "" {
		w.pathInput.SetSuggestions(nil)
		return
	}

	// Expand ~ to home directory
	expanded := val
	if strings.HasPrefix(val, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			expanded = home + val[1:]
		}
	}

	dir := expanded
	prefix := ""

	info, err := os.Stat(expanded)
	if err != nil || !info.IsDir() {
		dir = filepath.Dir(expanded)
		prefix = filepath.Base(expanded)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		w.pathInput.SetSuggestions(nil)
		return
	}

	var suggestions []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if prefix != "" && !strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
			continue
		}
		full := filepath.Join(dir, name)
		// Convert back to use ~ if original used ~
		if strings.HasPrefix(val, "~") {
			if home, err := os.UserHomeDir(); err == nil {
				full = "~" + strings.TrimPrefix(full, home)
			}
		}
		suggestions = append(suggestions, full)
	}

	w.pathInput.SetSuggestions(suggestions)
}

// sanitizeNameRegex matches characters not valid in sandbox names.
var sanitizeNameRegex = regexp.MustCompile(`[^a-z0-9_-]`)

// suggestName generates a sandbox name from path and template.
func suggestName(path, template string) string {
	base := filepath.Base(path)
	base = strings.ToLower(base)
	base = sanitizeNameRegex.ReplaceAllString(base, "-")
	// Trim leading/trailing hyphens
	base = strings.Trim(base, "-")

	if base == "" {
		base = "sandbox"
	}

	name := base + "-" + template
	// Truncate to 63 chars
	if len(name) > 63 {
		name = name[:63]
	}
	// Trim trailing hyphens from truncation
	name = strings.TrimRight(name, "-")

	return name
}

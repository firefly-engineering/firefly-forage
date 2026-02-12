// Package tui provides terminal user interface components for forage-ctl
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/health"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/multiplexer"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
)

// Action represents the action to take after picker selection
type Action int

const (
	ActionNone Action = iota
	ActionAttach
	ActionNew
	ActionDown
	ActionQuit
)

// screen identifies which screen the picker is showing.
type screen int

const (
	screenList screen = iota
	screenWizard
)

// PickerOptions configures the picker behavior.
type PickerOptions struct {
	AllowCreate  bool   // enables creation wizard (false for gateway)
	TemplatesDir string // for loading templates in wizard
}

// CreateOptions holds wizard-collected values for sandbox creation.
type CreateOptions struct {
	Name        string
	Template    string
	RepoPath    string
	Direct      bool
	NoMuxConfig bool
	GitUser     string
	GitEmail    string
	SSHKeyPath  string
}

// PickerResult holds the result of the picker
type PickerResult struct {
	Action        Action
	Sandbox       *config.SandboxMetadata
	CreateOptions *CreateOptions // non-nil when wizard completed
}

// sandboxItem implements list.Item for sandbox display
type sandboxItem struct {
	metadata *config.SandboxMetadata
	status   health.Status
	uptime   string
}

func (i sandboxItem) Title() string {
	return i.metadata.Name
}

func (i sandboxItem) Description() string {
	mode := i.metadata.WorkspaceMode

	statusIcon := "●"
	switch i.status {
	case health.StatusHealthy:
		statusIcon = "✓"
	case health.StatusUnhealthy:
		statusIcon = "⚠"
	case health.StatusNoMux:
		statusIcon = "○"
	case health.StatusStopped:
		statusIcon = "●"
	}

	// Show source repo for jj/git-worktree modes, workspace otherwise
	location := i.metadata.Workspace
	if i.metadata.SourceRepo != "" {
		location = i.metadata.SourceRepo
	}

	return fmt.Sprintf("%s %s | %s | %s",
		statusIcon,
		i.metadata.Template,
		mode,
		truncatePath(location, 40),
	)
}

func (i sandboxItem) FilterValue() string {
	return i.metadata.Name
}

func truncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	return "..." + path[len(path)-maxLen+3:]
}

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39")).
			MarginBottom(1)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(1)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true)
)

// Model is the bubbletea model for the sandbox picker
type Model struct {
	list     list.Model
	result   PickerResult
	quitting bool
	paths    *config.Paths
	options  PickerOptions
	screen   screen
	wizard   *wizardModel
	width    int
	height   int
}

// NewPicker creates a new sandbox picker.
// The rt parameter is optional; if nil, all sandboxes will show as stopped.
func NewPicker(sandboxes []*config.SandboxMetadata, paths *config.Paths, rt runtime.Runtime, opts PickerOptions) Model {
	items := buildGroupedItems(sandboxes, rt)

	delegate := newGroupedDelegate()
	l := list.New(items, delegate, 80, 20)
	l.Title = "Firefly Forage - Select Sandbox"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = titleStyle

	m := Model{
		list:    l,
		paths:   paths,
		options: opts,
		screen:  screenList,
	}

	// Skip initial header selection
	skipHeaders(&m.list, 1)

	return m
}

func (m Model) Init() tea.Cmd {
	if m.screen == screenWizard && m.wizard != nil {
		return m.wizard.Init()
	}
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenWizard:
		return m.updateWizard(msg)
	default:
		return m.updateList(msg)
	}
}

func (m Model) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height-4)
		return m, nil

	case tea.KeyMsg:
		// Don't handle keys if filtering
		if m.list.FilterState() == list.Filtering {
			break
		}

		switch msg.String() {
		case "enter":
			if item, ok := m.list.SelectedItem().(sandboxItem); ok {
				m.result = PickerResult{
					Action:  ActionAttach,
					Sandbox: item.metadata,
				}
				m.quitting = true
				return m, tea.Quit
			}

		case "n":
			if m.options.AllowCreate {
				m.screen = screenWizard
				w := newWizardModel(m.options.TemplatesDir)
				m.wizard = &w
				if m.width > 0 && m.height > 0 {
					m.wizard.width = m.width
					m.wizard.height = m.height
				}
				return m, m.wizard.Init()
			}
			m.result = PickerResult{Action: ActionNew}
			m.quitting = true
			return m, tea.Quit

		case "d":
			if item, ok := m.list.SelectedItem().(sandboxItem); ok {
				m.result = PickerResult{
					Action:  ActionDown,
					Sandbox: item.metadata,
				}
				m.quitting = true
				return m, tea.Quit
			}

		case "q", "esc":
			m.result = PickerResult{Action: ActionQuit}
			m.quitting = true
			return m, tea.Quit
		}
	}

	prevIdx := m.list.Index()
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)

	// Skip headers after navigation
	if m.list.Index() != prevIdx {
		dir := 1
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			dir = navigationDirection(keyMsg)
		}
		skipHeaders(&m.list, dir)
	}

	return m, cmd
}

func (m Model) updateWizard(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.wizard == nil {
		m.screen = screenList
		return m, nil
	}

	// Handle window size for wizard
	if wsMsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wsMsg.Width
		m.height = wsMsg.Height
		m.wizard.width = wsMsg.Width
		m.wizard.height = wsMsg.Height
	}

	done, opts, cmd := m.wizard.Update(msg)
	if done {
		if opts != nil {
			m.result = PickerResult{
				Action:        ActionNew,
				CreateOptions: opts,
			}
			m.quitting = true
			return m, tea.Quit
		}
		// Wizard cancelled, return to list
		m.screen = screenList
		m.wizard = nil
		return m, nil
	}
	return m, cmd
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	switch m.screen {
	case screenWizard:
		if m.wizard != nil {
			return m.wizard.View()
		}
	}

	help := helpStyle.Render("[enter] Attach  [n] New  [d] Down  [/] Filter  [q] Quit")

	return m.list.View() + "\n" + help
}

// Result returns the picker result
func (m Model) Result() PickerResult {
	return m.result
}

// RunPicker runs the interactive sandbox picker.
// The rt parameter is optional; if nil, all sandboxes will show as stopped.
func RunPicker(sandboxes []*config.SandboxMetadata, paths *config.Paths, rt runtime.Runtime, opts PickerOptions) (PickerResult, error) {
	if len(sandboxes) == 0 {
		if opts.AllowCreate {
			// Go directly to wizard
			w := newWizardModel(opts.TemplatesDir)
			m := Model{
				paths:   paths,
				options: opts,
				screen:  screenWizard,
				wizard:  &w,
			}
			p := tea.NewProgram(m, tea.WithAltScreen())
			finalModel, err := p.Run()
			if err != nil {
				return PickerResult{}, err
			}
			model, ok := finalModel.(Model)
			if !ok {
				return PickerResult{}, fmt.Errorf("unexpected model type")
			}
			return model.Result(), nil
		}
		return PickerResult{Action: ActionNew}, nil
	}

	m := NewPicker(sandboxes, paths, rt, opts)
	p := tea.NewProgram(m, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return PickerResult{}, err
	}

	model, ok := finalModel.(Model)
	if !ok {
		return PickerResult{}, fmt.Errorf("unexpected model type")
	}
	return model.Result(), nil
}

// SimplePicker is a non-interactive picker that just lists sandboxes.
// The rt parameter is optional; if nil, all sandboxes will show as stopped.
func SimplePicker(sandboxes []*config.SandboxMetadata, paths *config.Paths, rt runtime.Runtime) string {
	var sb strings.Builder

	sb.WriteString("Firefly Forage - Sandboxes\n")
	sb.WriteString(strings.Repeat("─", 60) + "\n\n")

	if len(sandboxes) == 0 {
		sb.WriteString("No sandboxes found.\n")
		sb.WriteString("Create one with: forage-ctl up <name> -t <template>\n")
		return sb.String()
	}

	for i, sandbox := range sandboxes {
		mux := multiplexer.New(multiplexer.Type(sandbox.Multiplexer))
		status := health.GetSummary(sandbox.Name, sandbox.ContainerIP(), rt, mux)
		statusIcon := "●"
		switch status {
		case health.StatusHealthy:
			statusIcon = "✓"
		case health.StatusUnhealthy:
			statusIcon = "⚠"
		case health.StatusNoMux:
			statusIcon = "○"
		}

		// Show source repo for jj/git-worktree modes, workspace otherwise
		location := sandbox.Workspace
		if sandbox.SourceRepo != "" {
			location = sandbox.SourceRepo
		}

		sb.WriteString(fmt.Sprintf("%d. %s %s (%s)\n",
			i+1, statusIcon, sandbox.Name, sandbox.Template))
		sb.WriteString(fmt.Sprintf("   IP: %s | %s\n\n",
			sandbox.ContainerIP(), truncatePath(location, 40)))
	}

	return sb.String()
}

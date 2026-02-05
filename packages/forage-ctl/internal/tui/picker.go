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

// PickerResult holds the result of the picker
type PickerResult struct {
	Action  Action
	Sandbox *config.SandboxMetadata
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
	if mode == "" {
		mode = "dir"
	}

	statusIcon := "●"
	switch i.status {
	case health.StatusHealthy:
		statusIcon = "✓"
	case health.StatusUnhealthy:
		statusIcon = "⚠"
	case health.StatusNoTmux:
		statusIcon = "○"
	case health.StatusStopped:
		statusIcon = "●"
	}

	return fmt.Sprintf("%s %s | %s | %s | %s",
		statusIcon,
		i.metadata.Template,
		mode,
		i.uptime,
		truncatePath(i.metadata.Workspace, 30),
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
	width    int
	height   int
}

// NewPicker creates a new sandbox picker
func NewPicker(sandboxes []*config.SandboxMetadata, paths *config.Paths) Model {
	items := make([]list.Item, len(sandboxes))
	for i, sb := range sandboxes {
		status := health.GetSummary(sb.Name, sb.Port, paths.SandboxesDir)
		uptime := "stopped"
		if status != health.StatusStopped {
			uptime = health.GetUptime(sb.Name)
		}
		items[i] = sandboxItem{
			metadata: sb,
			status:   status,
			uptime:   uptime,
		}
	}

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = selectedStyle
	delegate.Styles.SelectedDesc = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	l := list.New(items, delegate, 80, 20)
	l.Title = "Firefly Forage - Select Sandbox"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = titleStyle

	return Model{
		list:  l,
		paths: paths,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	help := helpStyle.Render("[enter] Attach  [n] New  [d] Down  [/] Filter  [q] Quit")

	return m.list.View() + "\n" + help
}

// Result returns the picker result
func (m Model) Result() PickerResult {
	return m.result
}

// RunPicker runs the interactive sandbox picker
func RunPicker(sandboxes []*config.SandboxMetadata, paths *config.Paths) (PickerResult, error) {
	if len(sandboxes) == 0 {
		return PickerResult{Action: ActionNew}, nil
	}

	m := NewPicker(sandboxes, paths)
	p := tea.NewProgram(m, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return PickerResult{}, err
	}

	return finalModel.(Model).Result(), nil
}

// SimplePicker is a non-interactive picker that just lists sandboxes
func SimplePicker(sandboxes []*config.SandboxMetadata, paths *config.Paths) string {
	var sb strings.Builder

	sb.WriteString("Firefly Forage - Sandboxes\n")
	sb.WriteString(strings.Repeat("─", 60) + "\n\n")

	if len(sandboxes) == 0 {
		sb.WriteString("No sandboxes found.\n")
		sb.WriteString("Create one with: forage-ctl up <name> -t <template>\n")
		return sb.String()
	}

	for i, sandbox := range sandboxes {
		status := health.GetSummary(sandbox.Name, sandbox.Port, paths.SandboxesDir)
		statusIcon := "●"
		switch status {
		case health.StatusHealthy:
			statusIcon = "✓"
		case health.StatusUnhealthy:
			statusIcon = "⚠"
		case health.StatusNoTmux:
			statusIcon = "○"
		}

		sb.WriteString(fmt.Sprintf("%d. %s %s (%s)\n",
			i+1, statusIcon, sandbox.Name, sandbox.Template))
		sb.WriteString(fmt.Sprintf("   Port: %d | Workspace: %s\n\n",
			sandbox.Port, truncatePath(sandbox.Workspace, 40)))
	}

	return sb.String()
}

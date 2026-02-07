package tui

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/health"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
)

// headerItem is a non-selectable group separator in the picker list.
type headerItem struct {
	label string
}

func (h headerItem) FilterValue() string { return "" }
func (h headerItem) Title() string       { return h.label }
func (h headerItem) Description() string { return "" }

// groupKey returns the grouping key for a sandbox.
// Uses SourceRepo if set (for jj/git-worktree), otherwise Workspace.
func groupKey(sb *config.SandboxMetadata) string {
	if sb.SourceRepo != "" {
		return sb.SourceRepo
	}
	return sb.Workspace
}

// buildGroupedItems groups sandboxes by project and returns list items
// with headerItem separators. The rt parameter is optional; if nil, all
// sandboxes will show as stopped.
func buildGroupedItems(sandboxes []*config.SandboxMetadata, rt runtime.Runtime) []list.Item {
	if len(sandboxes) == 0 {
		return nil
	}

	// Group sandboxes by key
	type group struct {
		key       string
		sandboxes []*config.SandboxMetadata
	}
	groupMap := make(map[string]*group)
	for _, sb := range sandboxes {
		key := groupKey(sb)
		g, ok := groupMap[key]
		if !ok {
			g = &group{key: key}
			groupMap[key] = g
		}
		g.sandboxes = append(g.sandboxes, sb)
	}

	// Sort groups alphabetically
	groups := make([]*group, 0, len(groupMap))
	for _, g := range groupMap {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].key < groups[j].key
	})

	// Build items with headers
	var items []list.Item
	for _, g := range groups {
		items = append(items, headerItem{label: g.key})
		for _, sb := range g.sandboxes {
			status := health.GetSummary(sb.Name, sb.ContainerIP(), rt)
			uptime := "stopped"
			if status != health.StatusStopped {
				uptime = health.GetUptime(sb.Name, rt)
			}
			items = append(items, sandboxItem{
				metadata: sb,
				status:   status,
				uptime:   uptime,
			})
		}
	}

	return items
}

// headerStyle is the style for group header items.
var headerStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("241")).
	PaddingLeft(2)

// groupedDelegate renders both headerItem and sandboxItem in the picker list.
type groupedDelegate struct {
	inner list.DefaultDelegate
}

// newGroupedDelegate creates a groupedDelegate wrapping a configured DefaultDelegate.
func newGroupedDelegate() groupedDelegate {
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = selectedStyle
	delegate.Styles.SelectedDesc = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	return groupedDelegate{inner: delegate}
}

func (d groupedDelegate) Height() int                             { return d.inner.Height() }
func (d groupedDelegate) Spacing() int                            { return d.inner.Spacing() }
func (d groupedDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d groupedDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	if h, ok := item.(headerItem); ok {
		str := headerStyle.Render(h.label)
		fmt.Fprint(w, str)
		return
	}

	d.inner.Render(w, m, index, item)
}

// skipHeaders adjusts the cursor position to skip headerItem entries.
// direction should be 1 (down) or -1 (up).
func skipHeaders(l *list.Model, direction int) {
	items := l.Items()
	if len(items) == 0 {
		return
	}

	idx := l.Index()
	if _, ok := items[idx].(headerItem); !ok {
		return
	}

	// Try to move in the given direction first
	next := idx + direction
	if next >= 0 && next < len(items) {
		if _, ok := items[next].(headerItem); !ok {
			l.Select(next)
			return
		}
	}

	// Fall back to the opposite direction
	opposite := idx - direction
	if opposite >= 0 && opposite < len(items) {
		if _, ok := items[opposite].(headerItem); !ok {
			l.Select(opposite)
			return
		}
	}

	// Search forward from current position for any non-header
	for i := 0; i < len(items); i++ {
		candidate := (idx + i*direction + len(items)) % len(items)
		if _, ok := items[candidate].(headerItem); !ok {
			l.Select(candidate)
			return
		}
	}
}

// isHeaderSelected returns true if the currently selected item is a headerItem.
func isHeaderSelected(l *list.Model) bool {
	if item := l.SelectedItem(); item != nil {
		_, ok := item.(headerItem)
		return ok
	}
	return false
}

// navigationDirection returns 1 for down/j keys, -1 for up/k keys.
func navigationDirection(msg tea.KeyMsg) int {
	switch {
	case msg.String() == "up" || msg.String() == "k":
		return -1
	default:
		return 1
	}
}

// headerCount returns the number of headerItems before the given index.
// This is used to compute a reasonable status bar item count.
func headerCount(items []list.Item) int {
	count := 0
	for _, item := range items {
		if _, ok := item.(headerItem); ok {
			count++
		}
	}
	return count
}

// shortenGroupKey shortens a path for display as a group header label.
func shortenGroupKey(path string) string {
	// Show just the last 2 components for readability
	parts := strings.Split(path, "/")
	if len(parts) > 2 {
		return strings.Join(parts[len(parts)-2:], "/")
	}
	return path
}

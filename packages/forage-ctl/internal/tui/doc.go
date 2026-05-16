// Package tui provides terminal user interface components for forage-ctl.
//
// This package uses the Bubble Tea framework to create interactive terminal
// interfaces, primarily for the gateway's sandbox picker.
//
// # Sandbox Picker
//
// The picker displays running sandboxes grouped by project and allows selection:
//
//	opts := tui.PickerOptions{AllowCreate: true, TemplatesDir: paths.TemplatesDir}
//	result, err := tui.RunPicker(sandboxes, paths, rt, opts)
//	switch result.Action {
//	case tui.ActionAttach:
//	    // Connect to result.Sandbox
//	case tui.ActionNew:
//	    if result.CreateOptions != nil {
//	        // Create sandbox from wizard results
//	    }
//	case tui.ActionDown:
//	    // Stop selected sandbox
//	case tui.ActionQuit:
//	    // Exit
//	}
//
// # Picker Features
//
//   - Lists all sandboxes grouped by project (SourceRepo or Workspace)
//   - Keyboard navigation (j/k or arrows), headers auto-skipped
//   - Quick actions: Enter (attach), n (new/wizard), d (down), q (quit)
//   - Color-coded status indicators
//   - Creation wizard when AllowCreate is true (path, template, name, advanced)
//
// # Dependencies
//
// Uses the Charm libraries:
//   - github.com/charmbracelet/bubbletea - TUI framework
//   - github.com/charmbracelet/bubbles - UI components
//   - github.com/charmbracelet/lipgloss - Styling
package tui

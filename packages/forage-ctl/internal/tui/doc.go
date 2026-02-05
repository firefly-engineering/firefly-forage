// Package tui provides terminal user interface components for forage-ctl.
//
// This package uses the Bubble Tea framework to create interactive terminal
// interfaces, primarily for the gateway's sandbox picker.
//
// # Sandbox Picker
//
// The picker displays running sandboxes and allows selection:
//
//	result, err := tui.RunPicker(sandboxes, paths, rt)
//	switch result.Action {
//	case tui.ActionAttach:
//	    // Connect to result.Sandbox
//	case tui.ActionNew:
//	    // Create new sandbox
//	case tui.ActionDown:
//	    // Stop selected sandbox
//	case tui.ActionQuit:
//	    // Exit
//	}
//
// # Picker Features
//
//   - Lists all sandboxes with health status and uptime
//   - Keyboard navigation (j/k or arrows)
//   - Quick actions: Enter (attach), n (new), d (down), q (quit)
//   - Color-coded status indicators
//
// # Dependencies
//
// Uses the Charm libraries:
//   - github.com/charmbracelet/bubbletea - TUI framework
//   - github.com/charmbracelet/bubbles - UI components
//   - github.com/charmbracelet/lipgloss - Styling
package tui

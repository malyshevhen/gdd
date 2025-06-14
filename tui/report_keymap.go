package tui

import "github.com/charmbracelet/bubbles/key"

// ReportKeyMap defines keybindings for the report view.
type ReportKeyMap struct {
	BackToList key.Binding
	// Viewport keys are handled by the viewport model itself (up, down, pgup, pgdn, etc.)
}

// DefaultReportKeyMap returns a new ReportKeyMap with default keybindings.
func DefaultReportKeyMap() ReportKeyMap {
	return ReportKeyMap{
		BackToList: key.NewBinding(
			key.WithKeys("esc", "q", "b"), // Allow Esc, q, or b to go back
			key.WithHelp("esc/q/b", "back to list"),
		),
	}
}

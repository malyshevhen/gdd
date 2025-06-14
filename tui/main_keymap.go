package tui

import (
	"github.com/charmbracelet/bubbles/key"
)

// ListKeyMap defines keybindings specifically for the list view actions.
// These are in addition to the default list navigation and filtering keys.
type ListKeyMap struct {
	RunSelectedTest key.Binding
	RunPackageTests key.Binding
	RunAllTests     key.Binding
	// Help            key.Binding // Potentially for a context-sensitive help view
}

// DefaultListKeyMap returns a new ListKeyMap with default keybindings.
func DefaultListKeyMap() ListKeyMap {
	return ListKeyMap{
		RunSelectedTest: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "run selected"),
		),
		RunPackageTests: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "run package"),
		),
		RunAllTests: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "run all"),
		),
	}
}

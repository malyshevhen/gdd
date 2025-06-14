package tui

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
)

// ListModel manages the state of the test list view.
// It embeds charmbracelet/bubbles/list.Model and adds application-specific logic.
type ListModel struct {
	list   list.Model
	keys   ListKeyMap
	styles *AppStyles // Styles passed from MainModel
	logger *log.Logger

	width  int
	height int
}

// NewListModel creates a new instance of the ListModel.
// The list.DefaultDelegate should be created and styled in MainModel and passed here.
func NewListModel(delegate *list.DefaultDelegate, logger *log.Logger, styles *AppStyles) ListModel {
	l := list.New([]list.Item{}, delegate, 0, 0) // Initialized empty, sized on WindowSizeMsg

	// Configure list-specific styles (chrome around the items)
	l.Styles.Title = styles.ListHeader
	l.Styles.FilterPrompt = styles.ListFilterPrompt
	l.Styles.FilterCursor = styles.ListFilterCursor
	l.Styles.PaginationStyle = styles.ListPagination
	l.Styles.HelpStyle = styles.Help // For the list's built-in help

	// Define additional keybindings for list actions shown in help
	keys := DefaultListKeyMap()
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			keys.RunSelectedTest,
			keys.RunPackageTests,
			keys.RunAllTests,
		}
	}
	// l.SetShowHelp(true) // By default, list shows its help. MainModel can control this.

	// Set an initial title. This can be updated based on state (e.g., loading, no items).
	l.Title = "Discovering tests..."

	return ListModel{
		list:   l,
		keys:   keys,
		styles: styles,
		logger: logger,
	}
}

// Init is part of the tea.Model interface. For ListModel, it typically does nothing.
func (m ListModel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the ListModel.
// It processes key presses for list actions and delegates other messages
// to the embedded list.Model.
func (m ListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// The list.SetSize call will adjust the viewport of the list.
		// MainModel is responsible for allocating the correct height to ListModel.
		m.list.SetSize(m.width, m.height)
		return m, nil

	case tea.KeyMsg:
		// When the list is filtering, allow the default list behavior to handle keys.
		if m.list.FilterState() == list.Filtering {
			break // Handled by m.list.Update(msg) below
		}

		// Handle custom keybindings for test execution.
		// These messages will be caught by the MainModel's Update function.
		switch {
		case key.Matches(msg, m.keys.RunSelectedTest):
			m.logger.Debug("ListModel: 'Run Selected Test' key pressed.")
			if m.list.SelectedItem() != nil {
				// Send a message to MainModel to trigger running the selected test.
				// MainModel will retrieve the selected item itself.
				return m, func() tea.Msg { return triggerRunSelectedTestMsg{} }
			}
		case key.Matches(msg, m.keys.RunPackageTests):
			m.logger.Debug("ListModel: 'Run Package Tests' key pressed.")
			if m.list.SelectedItem() != nil {
				return m, func() tea.Msg { return triggerRunPackageTestsMsg{} }
			}
		case key.Matches(msg, m.keys.RunAllTests):
			m.logger.Debug("ListModel: 'Run All Tests' key pressed.")
			return m, func() tea.Msg { return triggerRunAllTestsMsg{} }
		}
	}

	// Delegate all other messages to the embedded list.Model's Update function.
	var listCmd tea.Cmd
	m.list, listCmd = m.list.Update(msg)
	cmds = append(cmds, listCmd)

	return m, tea.Batch(cmds...)
}

// View renders the ListModel.
// It simply calls the View method of the embedded list.Model.
func (m ListModel) View() string {
	if m.width == 0 || m.height == 0 {
		// Avoid rendering if dimensions are not yet set, can happen at init.
		return m.styles.Loading.Render("Initializing list view...")
	}
	// If list has no items and not filtering, show "no items" message using style.
	// The list.Model has some internal handling for this, but we can customize.
	if len(m.list.VisibleItems()) == 0 && m.list.FilterState() != list.Filtering {
		// If the title already indicates no tests, just render the list (which will be empty)
		// Otherwise, we could provide a centered message.
		// For now, relying on the list title and its empty state rendering.
		// To force a message like "No items found" in the body, one might need to
		// customize the delegate or use list.SetItems with a special placeholder.
		// An alternative: if m.list.Items() is empty, make View return m.styles.ListNoItems.Render(...).
		// However, list.View() handles its empty states, so let it.
	}

	return m.list.View()
}

// --- Helper methods for MainModel to interact with ListModel ---

// SetItems populates the list with discovered test items.
// This is typically called by MainModel after tests are found.
func (m *ListModel) SetItems(items []list.Item) tea.Cmd {
	numItems := len(items)
	m.logger.Debugf("ListModel: Setting %d items.", numItems)

	if numItems == 0 {
		m.list.Title = "No Go Tests Found in Project"
		// list.Model handles showing "No items" or similar based on its items.
	} else {
		m.list.Title = "Available Go Tests"
	}
	// This will also reset the filter and selection.
	return m.list.SetItems(items)
}

// SelectedItem returns the currently highlighted item in the list.
// MainModel uses this to get details of the test to run.
func (m *ListModel) SelectedItem() list.Item {
	return m.list.SelectedItem()
}

// GetHeight returns the height of the list component.
func (m ListModel) GetHeight() int {
	return m.height
}

// GetWidth returns the width of the list component.
func (m ListModel) GetWidth() int {
	return m.width
}

// VisibleItems returns the currently visible (filtered) items.
func (m ListModel) VisibleItems() []list.Item {
	return m.list.VisibleItems()
}

// ViewHelp renders the help text for the list.
// This can be integrated into a global help view by MainModel.
// The list.Model itself has a HelpView() method.
func (m ListModel) ViewHelp() string {
	return m.list.View() // The list.View() includes help if ShowHelp is true
}

// SetTitle allows MainModel to change the list title (e.g., "Loading tests...")
func (m *ListModel) SetTitle(title string) {
	m.list.Title = title
}

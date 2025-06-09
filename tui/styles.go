package tui

import "github.com/charmbracelet/lipgloss"

// AppStyles holds various lipgloss styles used throughout the application.
// These styles are centralized here for consistency and easier modification.
type AppStyles struct {
	// General UI elements
	Base  lipgloss.Style // Base style, can be used for general text
	Help  lipgloss.Style // For help text (e.g., keybindings)
	Error lipgloss.Style // For displaying error messages
	Title lipgloss.Style // For view titles or major headings
	Debug lipgloss.Style // For any ad-hoc debug info in UI

	// List View (Test List)
	ListHeader       lipgloss.Style // Header/title of the list
	ListItem         lipgloss.Style // Default style for a list item
	ListSelectedItem lipgloss.Style // Style for a selected list item
	ListDescription  lipgloss.Style // Style for item descriptions (e.g., package name)
	ListSelectedDesc lipgloss.Style // Style for selected item descriptions
	ListFilterPrompt lipgloss.Style // Prompt for the filter input (e.g., "Filter:")
	ListFilterCursor lipgloss.Style // Style for the cursor in the filter input
	ListPagination   lipgloss.Style // Style for pagination info (e.g., "1/10")
	ListStatus       lipgloss.Style // For status messages within the list view
	ListNoItems      lipgloss.Style // Message when no items are in the list

	// Spinner / Loading State
	Spinner lipgloss.Style // Style for the spinner itself
	Loading lipgloss.Style // Style for text accompanying the spinner (e.g., "Loading...")

	// Report View (Test Results)
	ReportViewport      lipgloss.Style // Border/container for the results viewport
	ReportTitle         lipgloss.Style // Title of the report
	ReportSummaryHeader lipgloss.Style // Header for the summary section (e.g., "## Summary")
	ReportDetailsHeader lipgloss.Style // Header for the failed tests details section
	ReportMeta          lipgloss.Style // For metadata like "Run duration: 1.2s"

	// Test Status specific styles
	StatusPass    lipgloss.Style // For "PASS" text and icons
	StatusFail    lipgloss.Style // For "FAIL" text and icons
	StatusSkip    lipgloss.Style // For "SKIP" text and icons
	StatusUnknown lipgloss.Style // For tests with unknown status
	PassIcon      string
	FailIcon      string
	SkipIcon      string
	UnknownIcon   string

	// Code blocks within report (primarily for Glamour, but can define outer padding/margin)
	ReportCodeBlock lipgloss.Style

	// Footer / Global Status Bar
	FooterStatus lipgloss.Style // For persistent status messages at the bottom
}

// DefaultStyles initializes and returns an AppStyles struct with sensible default styling.
// Colors are chosen to be generally clear and TUI-friendly.
func DefaultStyles() *AppStyles {
	s := new(AppStyles)

	// --- General UI Elements ---
	s.Base = lipgloss.NewStyle()
	s.Help = lipgloss.NewStyle().Faint(true)                                    // Dimmed text for help
	s.Error = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555"))         // Bright Red for errors
	s.Title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))   // Purple, bold
	s.Debug = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Faint(true) // Dim gray for debug

	// --- List View ---
	s.ListHeader = lipgloss.NewStyle().
		Bold(true).
		Padding(0, 1).
		Background(lipgloss.Color("62")). // Purple background
		Foreground(lipgloss.Color("230")) // Light foreground for contrast

	s.ListItem = lipgloss.NewStyle().Padding(0, 0, 0, 2) // Indent items

	s.ListSelectedItem = lipgloss.NewStyle().
		Foreground(lipgloss.Color("212")). // Bright Pink/Purple
		Background(lipgloss.Color("237")). // Darker gray background for selection
		Padding(0, 0, 0, 1).Bold(true)

	s.ListDescription = lipgloss.NewStyle().
		Faint(true).
		PaddingLeft(2) // Align with item padding

	s.ListSelectedDesc = s.ListSelectedItem.
		Faint(false).   // Ensure description is not faint when its item is selected
		PaddingLeft(1). // Adjust padding for description within the selected item style
		Bold(false)     // Description usually isn't bold even when selected

	s.ListFilterPrompt = lipgloss.NewStyle().Foreground(lipgloss.Color("205")) // Magenta/Pink
	s.ListFilterCursor = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	s.ListPagination = lipgloss.NewStyle().Faint(true).Padding(0, 1)
	s.ListStatus = lipgloss.NewStyle().Faint(true).Padding(0, 1)
	s.ListNoItems = lipgloss.NewStyle().Faint(true).Padding(1, 2).SetString("No tests discovered.")

	// --- Spinner / Loading State ---
	s.Spinner = lipgloss.NewStyle().Foreground(lipgloss.Color("205")) // Magenta/Pink, matches filter prompt
	s.Loading = lipgloss.NewStyle().Padding(1, 2)                     // For "Loading tests..." text

	// --- Report View ---
	s.ReportViewport = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")). // Purple border, consistent with titles
		Padding(0, 1)                           // Padding inside the border, before Glamour content

	s.ReportTitle = s.Title.MarginBottom(1) // Reuse general title style
	s.ReportSummaryHeader = lipgloss.NewStyle().Bold(true).MarginTop(1).MarginBottom(1)
	s.ReportDetailsHeader = lipgloss.NewStyle().Bold(true).MarginTop(1).MarginBottom(1)
	s.ReportMeta = lipgloss.NewStyle().Faint(true).MarginBottom(1)

	// Test Status specific styles
	s.PassIcon = "✅"
	s.FailIcon = "❌"
	s.SkipIcon = "⏭️"
	s.UnknownIcon = "❓"

	s.StatusPass = lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")) // Green
	s.StatusFail = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")) // Red
	s.StatusSkip = lipgloss.NewStyle().Foreground(lipgloss.Color("#F1FA8C")) // Yellow
	s.StatusUnknown = lipgloss.NewStyle().Faint(true)                        // Dim for unknown status

	// Glamour handles internal code block styling. This is if we wrap it.
	s.ReportCodeBlock = lipgloss.NewStyle().Padding(0, 1)

	// --- Footer / Global Status Bar ---
	s.FooterStatus = lipgloss.NewStyle().
		Padding(0, 1).
		Background(lipgloss.Color("236")). // Dark gray background
		Foreground(lipgloss.Color("246"))  // Light gray text

	return s
}

package tui

import (
	"fmt"
	"strings"
	"time"

	"gdd/parser"
	"gdd/runner"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/log"
)

// ReportModel manages the state of the test report view.
type ReportModel struct {
	viewport viewport.Model
	keys     ReportKeyMap
	styles   *AppStyles
	logger   *log.Logger

	width          int
	height         int
	currentContent string // Stores the raw Markdown content before rendering

	// Information about the test run for display
	testRunScope  string            // e.g., "all project tests", "package foo", "TestBar"
	overallStatus parser.TestStatus // Overall status of this run
	totalDuration time.Duration
	totalTests    int
	passedCount   int
	failedCount   int
	skippedCount  int
}

// NewReportModel creates a new instance of the ReportModel.
func NewReportModel(logger *log.Logger, styles *AppStyles) ReportModel {
	vp := viewport.New(0, 0) // Dimensions will be set on WindowSizeMsg
	vp.Style = styles.ReportViewport

	// Configure Glamour rendering options (optional, defaults are usually good)
	// For example, to use a specific style:
	// glamourRenderer, _ := glamour.NewTermRenderer(
	// 	glamour.WithStandardStyle("dracula"), // or "dark", "light", "notty"
	// 	glamour.WithWordWrap(0), // glamour handles word wrap, viewport width constraints it
	// )
	// vp.SetContentRenderer(func(content string) string {
	// 	out, _ := glamourRenderer.Render(content)
	// 	return out
	// })
	// For now, we will call glamour.Render directly when content is set.

	return ReportModel{
		viewport: vp,
		keys:     DefaultReportKeyMap(),
		styles:   styles,
		logger:   logger,
	}
}

// Init is part of the tea.Model interface.
func (m ReportModel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the ReportModel.
func (m ReportModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) { // Changed return type
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// ReportModel usually takes the full window height or height allocated by MainModel.
		// Header/footer for the report can be part of the content rendered into the viewport
		// or styled lipgloss components outside the viewport managed by MainModel.
		// For simplicity, assume ReportModel gets the full height for its viewport.
		m.viewport.Width = m.width
		m.viewport.Height = m.height
		m.logger.Debugf("ReportModel: WindowSizeMsg. Viewport W: %d, H: %d", m.viewport.Width, m.viewport.Height)

		// Re-render content if it exists, as Glamour's output can depend on width.
		if m.currentContent != "" {
			// Determine glamour style based on app styles (basic detection)
			glamourTheme := "dark"
			// Example: if your base style suggests a light background, use "light" theme for glamour
			// This is a placeholder; a more robust theme detection/configuration might be needed.
			// if m.styles.Base.GetBackground() == lipgloss.Color("light") { // This is hypothetical
			// 	glamourTheme = "light"
			// }

			rendered, err := glamour.Render(m.currentContent, glamourTheme)
			if err != nil {
				m.logger.Errorf("Error re-rendering report content for new size: %v", err)
				m.viewport.SetContent(m.styles.Error.Render("Error re-rendering report: " + err.Error()))
			} else {
				m.viewport.SetContent(rendered)
			}
		}
		return m, nil

	case tea.KeyMsg:
		if key.Matches(msg, m.keys.BackToList) {
			m.logger.Debug("ReportModel: 'Back To List' key pressed.")
			// Send a message to MainModel to transition back to the list view.
			return m, func() tea.Msg { return backToListMsg{} }
		}
		// All other keys are passed to the viewport for scrolling.
	}

	// Delegate to viewport's Update method for scrolling and other internal handling.
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// View renders the ReportModel.
func (m ReportModel) View() string {
	if m.width == 0 || m.height == 0 {
		return m.styles.Loading.Render("Initializing report view...")
	}
	return m.viewport.View()
}

// SetContent processes the parsed test results and updates the viewport.
// This method is called by MainModel when test results are ready.
func (m *ReportModel) SetContent(results []*parser.PackageResult, runCfg runner.TestRunConfig) tea.Cmd {
	m.logger.Debugf("ReportModel: Setting content for scope '%s', %d package results.", runCfg.Type.String(), len(results))

	// Set the testRunScope based on the type of run from runCfg
	switch runCfg.Type {
	case runner.AllTests:
		m.testRunScope = "All Project Tests"
	case runner.PackageTests:
		m.testRunScope = fmt.Sprintf("Package: %s", runCfg.PackagePath)
	case runner.SingleTest:
		m.testRunScope = fmt.Sprintf("Test: %s (in %s)", runCfg.TestName, runCfg.PackagePath)
	default:
		m.testRunScope = "Unknown Test Scope"
	}

	m.totalDuration = 0
	m.totalTests = 0
	m.passedCount = 0
	m.failedCount = 0
	m.skippedCount = 0

	var md strings.Builder

	// --- Report Header ---
	md.WriteString(fmt.Sprintf("# Test Report: %s\n\n", m.testRunScope))

	// --- Overall Summary Section ---
	overallPackageStatus := parser.StatusPass // Assume pass until a failure is found
	if len(results) == 0 {
		overallPackageStatus = parser.StatusSkip // Or Unknown, if no tests run
		md.WriteString(m.styles.ListNoItems.Render("No test results to display for this run.") + "\n")
	}

	for _, pkgResult := range results {
		m.totalDuration += pkgResult.Duration // Sum up package durations for an approximate total
		for _, test := range pkgResult.Tests {
			m.totalTests++
			switch test.Status {
			case parser.StatusPass:
				m.passedCount++
			case parser.StatusFail:
				m.failedCount++
				if overallPackageStatus != parser.StatusFail { // A single test fail makes the package fail
					overallPackageStatus = parser.StatusFail
				}
			case parser.StatusSkip:
				m.skippedCount++
			default: // parser.StatusUnknown or parser.StatusRunning (if something went very wrong)
				// Count as skipped or handle as an error indicator
				m.skippedCount++
			}
		}
		// If any package failed, the overall run is a fail.
		if pkgResult.Status == parser.StatusFail && overallPackageStatus != parser.StatusFail {
			overallPackageStatus = parser.StatusFail
		} else if pkgResult.Status == parser.StatusSkip && overallPackageStatus == parser.StatusPass {
			// If previous were passes, a skip makes overall skip (unless a fail occurs later)
			overallPackageStatus = parser.StatusSkip
		}
	}
	m.overallStatus = overallPackageStatus

	md.WriteString("## Summary\n\n")
	statusIcon := m.styles.PassIcon
	statusStyle := m.styles.StatusPass
	switch m.overallStatus {
	case parser.StatusFail:
		statusIcon = m.styles.FailIcon
		statusStyle = m.styles.StatusFail
	case parser.StatusSkip:
		statusIcon = m.styles.SkipIcon
		statusStyle = m.styles.StatusSkip
	}
	md.WriteString(fmt.Sprintf("**Overall Status: %s %s**\n\n", statusIcon, statusStyle.Render(string(m.overallStatus))))

	summaryTable := fmt.Sprintf("| Statistic        | Value      |\n")
	summaryTable += fmt.Sprintf("| ---------------- | ---------- |\n")
	summaryTable += fmt.Sprintf("| %s Total Tests    | %-10d |\n", m.styles.UnknownIcon, m.totalTests)
	summaryTable += fmt.Sprintf("| %s Passed         | %-10d |\n", m.styles.PassIcon, m.passedCount)
	summaryTable += fmt.Sprintf("| %s Failed         | %-10d |\n", m.styles.FailIcon, m.failedCount)
	summaryTable += fmt.Sprintf("| %s Skipped        | %-10d |\n", m.styles.SkipIcon, m.skippedCount)
	summaryTable += fmt.Sprintf("| ⏱️ Total Duration | %-10s |\n", m.totalDuration.Round(time.Millisecond).String())
	md.WriteString(summaryTable)
	md.WriteString("\n")

	// --- Detailed Results Per Package ---
	if m.failedCount > 0 {
		md.WriteString("## Failed Tests Details\n\n")
	}

	for _, pkgResult := range results {
		// Optionally, print package header even if it passed, for complete logs.
		// For now, focusing on failures.
		pkgFailed := false
		var pkgFailures strings.Builder
		for _, test := range pkgResult.Tests {
			if test.Status == parser.StatusFail {
				pkgFailed = true
				pkgFailures.WriteString(fmt.Sprintf("### %s %s `[%s]`\n", m.styles.FailIcon, test.Name, pkgResult.PackageName))
				pkgFailures.WriteString(fmt.Sprintf("*Duration: %s*\n\n", test.Duration.Round(time.Millisecond)))
				if len(test.Output) > 0 {
					pkgFailures.WriteString("```log\n")
					// Clean and indent output for readability
					for _, line := range test.Output {
						pkgFailures.WriteString(strings.TrimSpace(line) + "\n")
					}
					pkgFailures.WriteString("```\n\n")
				} else {
					pkgFailures.WriteString("*(No output captured for this failed test.)*\n\n")
				}
			}
		}
		if pkgFailed {
			md.WriteString(pkgFailures.String())
		}

		// Include package summary output if it exists and the package itself failed or had issues
		if pkgResult.Status == parser.StatusFail && len(pkgResult.SummaryOutput) > 0 && !pkgFailed {
			// If package failed but no specific test did, show summary output under package error
			md.WriteString(fmt.Sprintf("### %s Package Error `[%s]`\n\n", m.styles.FailIcon, pkgResult.PackageName))
			md.WriteString("*This package reported an error. See output below.*\n\n")
			md.WriteString("```log\n")
			for _, line := range pkgResult.SummaryOutput {
				md.WriteString(strings.TrimSpace(line) + "\n")
			}
			md.WriteString("```\n\n")
		}
	}

	if m.failedCount == 0 && m.totalTests > 0 {
		md.WriteString("\n**✨ All tests passed! ✨**\n")
	} else if m.totalTests == 0 && m.overallStatus != parser.StatusFail {
		md.WriteString("\n*(No tests were executed or matched the criteria.)*\n")
	}

	m.currentContent = md.String()
	m.logger.Debugf("ReportModel: Markdown content generated (length: %d characters).", len(m.currentContent))

	// Render Markdown content using Glamour.
	// Use a specific style for Glamour if desired. "dark", "light", "notty", or a custom glamour.TermRenderer.
	// The viewport width is important for glamour's word wrapping.
	// Ensure m.viewport.Width is up-to-date before this.
	glamourTheme := "dark"
	// Basic theme detection (example, might need refinement)
	// Check a representative style to guess if the theme is light or dark
	// This is a simplistic way; a more robust method might involve explicit theme setting.
	// if m.styles.Base.GetBackground() == lipgloss.Color("light") { // Hypothetical check
	// 	 glamourTheme = "light"
	// }

	renderedContent, err := glamour.Render(m.currentContent, glamourTheme)
	if err != nil {
		m.logger.Errorf("ReportModel: Error rendering Markdown with Glamour: %v", err)
		m.viewport.SetContent(m.styles.Error.Render(fmt.Sprintf("Error rendering report: %v\n\nRaw Markdown:\n%s", err, m.currentContent)))
	} else {
		m.viewport.SetContent(renderedContent)
	}

	m.viewport.GotoTop() // Reset scroll to top for the new report.
	return nil
}

// Reset clears the content of the report view, preparing for a new report or view change.
func (m *ReportModel) Reset() {
	m.logger.Debug("ReportModel: Resetting content.")
	m.currentContent = ""
	m.viewport.SetContent("")
	m.viewport.GotoTop()
	m.testRunScope = ""
	m.overallStatus = parser.StatusUnknown
	m.totalDuration = 0
	m.totalTests = 0
	m.passedCount = 0
	m.failedCount = 0
	m.skippedCount = 0
}

// HelpView returns a string containing the help information for the report view.
func (m ReportModel) HelpView() string {
	var helpItems []string
	helpItems = append(helpItems, m.keys.BackToList.Help().Key+" → "+m.keys.BackToList.Help().Desc)
	helpItems = append(helpItems, "↑/↓/k/j/pgup/pgdn → scroll")
	return strings.Join(helpItems, ", ")
}

package tui

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gdd/finder"
	"gdd/parser"
	"gdd/runner"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
)

// appState defines the different views/states of the application.
type appState int

const (
	stateInitializing appState = iota // Initial state, discovering tests
	stateTestList                     // Displaying the list of tests
	stateRunningTests                 // Tests are currently being executed
	stateReportView                   // Displaying the test results report
	stateError                        // Displaying a fatal error
)

// TestItem is a list.Item implementation for discovered Go tests.
type TestItem struct {
	finder.TestInfo // Embed TestInfo from the finder package
}

// Title returns the function name for the list item.
func (ti TestItem) Title() string { return ti.Name }

// Description returns the package name and directory for the list item.
func (ti TestItem) Description() string {
	return fmt.Sprintf("Pkg: %s (%s)", ti.PackageName, ti.PackageDir)
}

// FilterValue returns the string to filter on.
func (ti TestItem) FilterValue() string {
	return fmt.Sprintf("%s %s %s", ti.Name, ti.PackageName, ti.PackageDir)
}

// --- Messages ---

// testsFoundMsg is sent when test discovery is complete.
type testsFoundMsg struct {
	items []list.Item
}

// testsLoadFailedMsg is sent if test discovery fails.
type testsLoadFailedMsg struct{ err error }

// triggerRunAllTestsMsg signals an intent to run all discovered tests.
type triggerRunAllTestsMsg struct{}

// triggerRunPackageTestsMsg signals an intent to run tests for the selected package.
type triggerRunPackageTestsMsg struct{}

// triggerRunSelectedTestMsg signals an intent to run the single selected test function.
type triggerRunSelectedTestMsg struct{}

// displayReportMsg is an internal message to trigger showing the report.
// It carries the parsed results and the original run configuration.
type displayReportMsg struct {
	parsedResults []*parser.PackageResult
	runConfig     runner.TestRunConfig
}

// backToListMsg signals to return from the report view to the test list view.
type backToListMsg struct{}

// errorMsg is a generic message for propagating non-fatal errors to potentially display.
// For fatal errors, m.fatalErr and stateError are used.
type errorMsg struct{ err error }

// --- Main Model ---

// MainModel is the central state of the TUI application.
type MainModel struct {
	state    appState
	fatalErr error // For displaying critical errors that halt the app

	listModel   ListModel   // Changed to value type, update methods should handle this
	reportModel ReportModel // Changed to value type
	spinner     spinner.Model
	styles      *AppStyles
	logger      *log.Logger

	width  int
	height int

	// Test execution related fields
	currentTestRunConfig  *runner.TestRunConfig // Config for the ongoing or last test run
	accumulatedJSONOutput bytes.Buffer          // Stores JSON lines from `go test -json`
	testOutputChan        <-chan tea.Msg        // Channel for messages from test runner goroutine
	statusMessage         string                // General status message for footer
}

// NewMainModel creates the initial model for the Bubble Tea program.
func NewMainModel() (*MainModel, error) {
	globalLogger := log.Default()
	globalLogger.Debug("MainModel: Initializing...")

	styles := DefaultStyles()

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styles.Spinner

	delegate := list.NewDefaultDelegate()
	delegate.Styles.NormalTitle = styles.ListItem
	delegate.Styles.NormalDesc = styles.ListDescription
	delegate.Styles.SelectedTitle = styles.ListSelectedItem
	delegate.Styles.SelectedDesc = styles.ListSelectedDesc
	delegate.Styles.DimmedTitle = styles.ListItem.Faint(true)
	delegate.Styles.DimmedDesc = styles.ListDescription.Faint(true)

	lm := NewListModel(&delegate, globalLogger, styles) // NewListModel returns ListModel (value)
	rm := NewReportModel(globalLogger, styles)          // NewReportModel returns ReportModel (value)

	m := &MainModel{
		state:         stateInitializing,
		spinner:       s,
		listModel:     lm,
		reportModel:   rm,
		styles:        styles,
		logger:        globalLogger,
		statusMessage: "Initializing...",
	}
	return m, nil
}

// Init is called once when the program starts.
func (m *MainModel) Init() tea.Cmd { // Receiver is pointer
	m.logger.Info("MainModel: Init called. Starting test discovery.")
	return tea.Batch(
		m.spinner.Tick,
		discoverTestsCmd(),
	)
}

// discoverTestsCmd is a command that runs the test discovery process.
func discoverTestsCmd() tea.Cmd {
	return func() tea.Msg {
		log.Debug("discoverTestsCmd: Starting test discovery...")
		foundTests, err := finder.FindTests(".")
		if err != nil {
			log.Errorf("discoverTestsCmd: Failed to discover tests: %v", err)
			return testsLoadFailedMsg{err: fmt.Errorf("test discovery failed: %w", err)}
		}

		if len(foundTests) == 0 {
			log.Info("discoverTestsCmd: No tests found.")
		} else {
			log.Infof("discoverTestsCmd: Discovered %d test functions.", len(foundTests))
		}

		items := make([]list.Item, len(foundTests))
		for i, t := range foundTests {
			items[i] = TestItem{TestInfo: t}
		}
		return testsFoundMsg{items: items}
	}
}

// Update handles incoming messages and updates the model accordingly.
func (m *MainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) { // Receiver is pointer, returns tea.Model
	var cmds []tea.Cmd
	var cmd tea.Cmd // Declare cmd here for broader scope within this function

	m.logger.Debugf("MainModel: Update received. State: %d, Msg: %T", m.state, msg)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.logger.Debugf("MainModel: WindowSizeMsg. W: %d, H: %d", m.width, m.height)

		footerHeight := lipgloss.Height(m.footerView())
		viewHeight := m.height - footerHeight

		m.listModel.width = m.width
		m.listModel.height = viewHeight
		m.listModel.list.SetSize(m.width, viewHeight)

		m.reportModel.width = m.width
		m.reportModel.height = viewHeight
		m.reportModel.viewport.Width = m.width
		m.reportModel.viewport.Height = viewHeight

		if m.state == stateReportView && m.reportModel.currentContent != "" {
			// Let reportModel's own Update/View handle re-rendering on size change if needed
			// Or explicitly trigger a re-render if necessary:
			// m.reportModel, cmd = m.reportModel.Update(msg)
			// cmds = append(cmds, cmd)
		}
		return m, nil // Return early as this is a global message

	case tea.KeyMsg:
		if key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c"))) {
			m.logger.Info("MainModel: Ctrl+C pressed, quitting.")
			return m, tea.Quit
		}
		if msg.String() == "q" {
			if m.state == stateTestList && m.listModel.list.FilterState() != list.Filtering {
				m.logger.Info("MainModel: 'q' pressed in TestList, quitting.")
				return m, tea.Quit
			}
		}
		if m.state == stateError && msg.String() != "" {
			m.logger.Info("MainModel: Key pressed in Error state, quitting.")
			return m, tea.Quit
		}
		// If not a global key press, let the active view handle it or fall through.

	case spinner.TickMsg:
		if m.state == stateInitializing || m.state == stateRunningTests {
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
		// Don't return early here, allow other processing or delegation.

	case testsFoundMsg:
		m.logger.Infof("MainModel: testsFoundMsg received with %d items.", len(msg.items))
		m.state = stateTestList
		m.statusMessage = "Select a test or action (a: all, p: package, enter: selected). Press '/' to filter."
		if len(msg.items) == 0 {
			m.statusMessage = "No tests found. Press 'q' to quit or Ctrl+C."
		}
		cmd = m.listModel.SetItems(msg.items) // SetItems is a method on ListModel (value type)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...) // Return early, significant state change

	case testsLoadFailedMsg:
		m.logger.Errorf("MainModel: testsLoadFailedMsg: %v", msg.err)
		m.fatalErr = msg.err
		m.state = stateError
		m.statusMessage = fmt.Sprintf("Error: %v. Press any key to quit.", msg.err)
		return m, nil // Return early

	case triggerRunAllTestsMsg, triggerRunPackageTestsMsg, triggerRunSelectedTestMsg:
		if m.state == stateRunningTests {
			m.logger.Warn("MainModel: Received trigger test run message while already running tests. Ignoring.")
			return m, nil
		}

		var runCfg runner.TestRunConfig
		runCfg.WorkingDir, _ = os.Getwd()

		switch msg.(type) {
		case triggerRunAllTestsMsg:
			m.logger.Info("MainModel: Triggering 'Run All Tests'.")
			runCfg.Type = runner.AllTests
			m.currentTestRunConfig = &runner.TestRunConfig{Type: runner.AllTests, PackagePath: "./...", WorkingDir: runCfg.WorkingDir}
			m.statusMessage = "Running all project tests..."
		case triggerRunPackageTestsMsg:
			selectedItem, ok := m.listModel.SelectedItem().(TestItem)
			if !ok {
				m.logger.Error("MainModel: Failed to get selected item for 'Run Package Tests'.")
				m.statusMessage = "Error: Could not determine selected package."
				return m, nil
			}
			m.logger.Infof("MainModel: Triggering 'Run Package Tests' for package: %s (dir: ./%s)", selectedItem.PackageName, selectedItem.PackageDir)
			runCfg.Type = runner.PackageTests
			runCfg.PackagePath = "./" + selectedItem.PackageDir
			m.currentTestRunConfig = &runCfg
			m.statusMessage = fmt.Sprintf("Running tests for package %s...", selectedItem.PackageName)
		case triggerRunSelectedTestMsg:
			selectedItem, ok := m.listModel.SelectedItem().(TestItem)
			if !ok {
				m.logger.Error("MainModel: Failed to get selected item for 'Run Selected Test'.")
				m.statusMessage = "Error: Could not determine selected test."
				return m, nil
			}
			m.logger.Infof("MainModel: Triggering 'Run Selected Test': %s in package %s (dir: ./%s)", selectedItem.Name, selectedItem.PackageName, selectedItem.PackageDir)
			runCfg.Type = runner.SingleTest
			runCfg.PackagePath = "./" + selectedItem.PackageDir
			runCfg.TestName = selectedItem.Name
			m.currentTestRunConfig = &runCfg
			m.statusMessage = fmt.Sprintf("Running test %s...", selectedItem.Name)
		}

		m.state = stateRunningTests
		m.accumulatedJSONOutput.Reset()
		m.testOutputChan = nil
		m.logger.Debugf("MainModel: Executing tests with config: %+v", runCfg)
		cmds = append(cmds, runner.ExecuteTestsCmd(runCfg), m.spinner.Tick)
		return m, tea.Batch(cmds...) // Return early

	case runner.StreamMsg: // Exported from runner package
		streamMessage := msg
		m.logger.Debug("MainModel: Received StreamMsg from runner.")
		m.testOutputChan = streamMessage.Stream
		cmds = append(cmds, runner.WaitForStreamMsgCmd(m.testOutputChan))
		// Don't return early, batch cmds

	case runner.TestOutputLineMsg:
		m.logger.Debugf("MainModel: Received TestOutputLineMsg.")
		m.accumulatedJSONOutput.WriteString(msg.Line + "\n")
		if m.testOutputChan != nil {
			cmds = append(cmds, runner.WaitForStreamMsgCmd(m.testOutputChan))
		}
		// Don't return early, batch cmds

	case runner.TestRunCompleteMsg:
		m.logger.Infof("MainModel: TestRunCompleteMsg received. Error: %v", msg.Err)
		m.testOutputChan = nil

		var parsedData []*parser.PackageResult
		var parseErr error

		if m.accumulatedJSONOutput.Len() == 0 && msg.Err != nil {
			m.logger.Errorf("MainModel: Test run completed with error and no JSON output: %v", msg.Err)
			parsedData = []*parser.PackageResult{
				{
					PackageName: "Test Execution Error", Status: parser.StatusFail,
					SummaryOutput: []string{
						"Execution failed to produce JSON output.",
						"Error from `go test`: " + msg.Err.Error(),
						"Check GDD log for `go test stderr` details.",
					},
					Tests: []*parser.TestResult{{
						PackageName: "Test Execution Error", Name: "Overall Execution",
						Status: parser.StatusFail, Output: []string{"Command `go test` failed: " + msg.Err.Error()},
					}},
				},
			}
		} else {
			parsedData, parseErr = parser.Parse(m.accumulatedJSONOutput.Bytes())
			if parseErr != nil {
				m.logger.Errorf("MainModel: Failed to parse test JSON output: %v", parseErr)
				parsedData = []*parser.PackageResult{
					{
						PackageName: "JSON Parsing Error", Status: parser.StatusFail,
						SummaryOutput: []string{
							"Failed to parse `go test -json` output.",
							"Error: " + parseErr.Error(),
							"Raw output (first 1KB):",
							limitString(m.accumulatedJSONOutput.String(), 1024),
						},
					},
				}
			}
		}
		m.logger.Infof("MainModel: Parsed %d package results.", len(parsedData))
		if m.currentTestRunConfig == nil {
			m.logger.Error("MainModel: currentTestRunConfig is nil when trying to display report. Using placeholder.")
			m.currentTestRunConfig = &runner.TestRunConfig{Type: runner.AllTests, PackagePath: "./...", WorkingDir: "."}
		}

		cmds = append(cmds, func() tea.Msg {
			return displayReportMsg{
				parsedResults: parsedData,
				runConfig:     *m.currentTestRunConfig,
			}
		})
		// Don't return early, batch cmds

	case displayReportMsg:
		m.logger.Info("MainModel: displayReportMsg received. Transitioning to ReportView.")
		m.state = stateReportView
		cmd = m.reportModel.SetContent(msg.parsedResults, msg.runConfig) // reportModel is value type
		m.statusMessage = m.reportModel.HelpView()
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...) // Return early

	case backToListMsg:
		m.logger.Info("MainModel: backToListMsg received. Transitioning to TestList state.")
		m.state = stateTestList
		m.reportModel.Reset()
		m.listModel.list.FilterInput.SetValue("")
		m.statusMessage = "Select a test or action (a: all, p: package, enter: selected)."
		return m, tea.Batch(cmds...) // Return early

	case errorMsg:
		m.logger.Errorf("MainModel: Generic errorMsg received: %v", msg.err)
		m.statusMessage = fmt.Sprintf("Error: %v", msg.err)
		// Don't return early, batch cmds
	}

	// --- Delegate messages to active child model ---
	var childCmd tea.Cmd // Renamed from cmd to avoid conflict with outer scope cmd
	var updatedModel tea.Model

	currentFocusedModelName := "None"

	switch m.state {
	case stateTestList:
		updatedModel, childCmd = m.listModel.Update(msg) // listModel.Update returns (tea.Model, tea.Cmd)
		if um, ok := updatedModel.(ListModel); ok {
			m.listModel = um
		} else {
			m.logger.Errorf("MainModel: ListModel.Update returned unexpected type %T", updatedModel)
		}
		currentFocusedModelName = "ListModel"
	case stateReportView:
		updatedModel, childCmd = m.reportModel.Update(msg) // reportModel.Update returns (tea.Model, tea.Cmd)
		if um, ok := updatedModel.(ReportModel); ok {
			m.reportModel = um
		} else {
			m.logger.Errorf("MainModel: ReportModel.Update returned unexpected type %T", updatedModel)
		}
		currentFocusedModelName = "ReportModel"
	case stateInitializing, stateRunningTests, stateError:
		// No child model input or handled globally/earlier in switch
		return m, tea.Batch(cmds...) // Batch any commands accumulated so far (e.g. spinner)
	}

	if childCmd != nil {
		m.logger.Debugf("MainModel: Delegated msg %T to %s, received cmd", msg, currentFocusedModelName)
		cmds = append(cmds, childCmd)
	} else {
		m.logger.Debugf("MainModel: Delegated msg %T to %s, no cmd received", msg, currentFocusedModelName)
	}

	return m, tea.Batch(cmds...)
}

// View renders the TUI based on the current model state.
func (m *MainModel) View() string { // Receiver is pointer
	m.logger.Debugf("MainModel: View called. State: %d", m.state)

	if m.fatalErr != nil {
		errorStyle := m.styles.Error.Bold(true).Padding(1, 2).Width(m.width)
		return fmt.Sprintf("\n%s\n\n%s\n\nPress any key to quit.",
			errorStyle.Render("Critical Error:"),
			m.fatalErr.Error(),
		)
	}

	var mainContentView string

	switch m.state {
	case stateInitializing:
		loadingStyle := m.styles.Loading.Width(m.width).Align(lipgloss.Center)
		spin := m.spinner.View() + " Discovering Go tests, please wait..."
		mainContentView = loadingStyle.Render(spin)
	case stateTestList:
		mainContentView = m.listModel.View()
	case stateRunningTests:
		loadingStyle := m.styles.Loading.Width(m.width).Height(m.height - lipgloss.Height(m.footerView())).Align(lipgloss.Center)
		var runDesc string
		if m.currentTestRunConfig != nil {
			switch m.currentTestRunConfig.Type {
			case runner.AllTests:
				runDesc = "all project tests"
			case runner.PackageTests:
				runDesc = fmt.Sprintf("package %s", filepath.Base(m.currentTestRunConfig.PackagePath))
			case runner.SingleTest:
				runDesc = fmt.Sprintf("test %s", m.currentTestRunConfig.TestName)
			default:
				runDesc = "tests"
			}
		} else {
			runDesc = "tests" // Fallback
		}
		content := fmt.Sprintf("%s Running %s...\n\n(Ctrl+C to attempt to quit)", m.spinner.View(), runDesc)
		mainContentView = loadingStyle.Render(content)
	case stateReportView:
		mainContentView = m.reportModel.View()
	default:
		mainContentView = m.styles.Error.Render("Unknown application state. This is a bug.")
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		mainContentView,
		m.footerView(),
	)
}

// footerView renders the status bar/help line at the bottom.
func (m *MainModel) footerView() string { // Receiver is pointer
	helpText := m.statusMessage
	return m.styles.FooterStatus.Width(m.width).Render(helpText)
}

// limitString utility
func limitString(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	if limit < 3 { // Need space for "..."
		if limit < 0 {
			return ""
		}
		return s[:limit]
	}
	return s[:limit-3] + "..."
}

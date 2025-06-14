package tui

import (
	"errors"
	"fmt"
	"gdd/finder"
	"gdd/parser"
	"gdd/runner"
	"os"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
)

var (
	ErrAlreadyRunning error = errors.New("tests already running.")
	ErrWrongPackage   error = errors.New("could not determine selected package.")
	ErrWrongTest      error = errors.New("could not determine selected test.")
)

func updateOnResize(m *MainModel, msg tea.WindowSizeMsg) {
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
}

func updateOnInit(m *MainModel) tea.Cmd {
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

func updateOnRunTests(m *MainModel, msg tea.Msg, cmd tea.Cmd) (tea.Cmd, error) {
	if m.state == stateRunningTests {
		m.logger.Warn("MainModel: Received trigger test run message while already running tests. Ignoring.")
		return nil, ErrAlreadyRunning
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
			return nil, ErrWrongPackage
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
			return nil, ErrWrongTest
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

	m.logger.Debugf("MainModel: Executing tests with config: %+v", runCfg)

	return runner.ExecuteTestsCmd(runCfg), nil
}

func updateOnTestsComplete(m *MainModel, msg runner.TestRunCompleteMsg) tea.Cmd {
	m.logger.Infof("MainModel: TestRunCompleteMsg received. Error: %v", msg.Err)
	m.testOutputChan = nil

	var parsedData []*parser.PackageResult
	var parseErr error

	if m.accumulatedJSONOutput.Len() == 0 && msg.Err != nil {
		m.logger.Errorf("MainModel: Test run completed with error and no JSON output: %v", msg.Err)
		parsedData = []*parser.PackageResult{
			{
				PackageName: "Test Execution Error",
				Status:      parser.StatusFail,
				SummaryOutput: []string{
					"Execution failed to produce JSON output.",
					"Error from `go test`: " + msg.Err.Error(),
					"Check GDD log for `go test stderr` details.",
				},
				Tests: []*parser.TestResult{{
					PackageName: "Test Execution Error",
					Name:        "Overall Execution",
					Status:      parser.StatusFail,
					Output:      []string{"Command `go test` failed: " + msg.Err.Error()},
				}},
			},
		}
	} else {
		parsedData, parseErr = parser.Parse(m.accumulatedJSONOutput.Bytes())

		if parseErr != nil {
			m.logger.Errorf("MainModel: Failed to parse test JSON output: %v", parseErr)
			parsedData = []*parser.PackageResult{
				{
					PackageName: "JSON Parsing Error",
					Status:      parser.StatusFail,
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
		m.currentTestRunConfig = &runner.TestRunConfig{
			Type:        runner.AllTests,
			PackagePath: "./...",
			WorkingDir:  ".",
		}
	}

	return func() tea.Msg {
		return displayReportMsg{
			parsedResults: parsedData,
			runConfig:     *m.currentTestRunConfig,
		}
	}
}

package runner

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
)

// TestOutputLineMsg carries a single line of JSON output from `go test -json`.
// Each line is a self-contained JSON object representing a test event.
type TestOutputLineMsg struct {
	Line string
}

// TestRunCompleteMsg indicates that the `go test` command has finished.
// It includes any error encountered while running the command itself (e.g., build failure, command not found),
// or if the test process exited with a non-zero status (which includes test failures).
// The rawCombinedOutput can be useful for debugging if JSON parsing fails or if there's non-JSON output.
type TestRunCompleteMsg struct {
	Err error
	// RawCombinedOutput string // Could be useful for debugging, but can be very large.
}

// TestTargetType defines the scope of the test run.
type TestTargetType int

const (
	// SingleTest runs a specific test function within its package.
	SingleTest TestTargetType = iota
	// PackageTests runs all tests in a specific package.
	PackageTests
	// AllTests runs all tests in the project (`./...`).
	AllTests
)

func (ttt TestTargetType) String() string {
	switch ttt {
	case SingleTest:
		return "single_test"
	case PackageTests:
		return "package_test"
	case AllTests:
		return "all_tests"
	default:
		return "unknown"
	}
}

// TestRunConfig holds the configuration for a test run.
type TestRunConfig struct {
	Type TestTargetType
	// PackagePath is the package import path or path from module root (e.g., \"./mypkg\", or \".\" for root).
	// For AllTests, this is ignored as \"./...\" is used.
	PackagePath string
	// TestName is the specific function name, e.g., \"TestMyFunction\" (only used if Type is SingleTest).
	TestName string
	// WorkingDir is the directory from which `go test` should be executed. Usually the project root.
	WorkingDir string
}

// StreamMsg is an initial message sent by a command that will stream subsequent messages.
// The `Update` function should store this channel and then listen for messages on it
// by returning a `WaitForStreamMsgCmd(stream)`.
type StreamMsg struct {
	Stream <-chan tea.Msg
}

// ExecuteTestsCmd creates a command to execute `go test -json` based on the provided config.
// It initiates a test run in a goroutine. The returned `tea.Cmd` will send an initial
// `StreamMsg` containing a channel. The goroutine will then send `TestOutputLineMsg`
// for each line of JSON output and a final `TestRunCompleteMsg` on this channel.
func ExecuteTestsCmd(config TestRunConfig) tea.Cmd {
	return func() tea.Msg {
		msgChan := make(chan tea.Msg, 1) // Buffer of 1 for the initial StreamMsg

		go func() {
			defer close(msgChan) // Ensure channel is closed when goroutine finishes

			var finalArgs []string

			// Base arguments for `go test`
			// -json: Output in JSON format.
			// -v: Verbose output, ensures all test events (including pass) are in the JSON stream.
			// -count=1: Disable test caching to ensure tests are always re-run.
			// -short: (Optional) if you want to run tests in short mode.
			baseArgs := []string{"test", "-json", "-v", "-count=1"}

			switch config.Type {
			case SingleTest:
				if config.PackagePath == "" || config.TestName == "" {
					err := fmt.Errorf("ExecuteTestsCmd: SingleTest requires a valid PackagePath and TestName")
					log.Error(err.Error())
					msgChan <- TestRunCompleteMsg{Err: err}
					return
				}
				// Format: go test [baseArgs] <package_path> -run ^TestName$
				// Ensure TestName is properly escaped for regex if it contains special characters.
				// For simplicity, assuming TestName is a valid Go function name without regex metachars.
				finalArgs = append(baseArgs, config.PackagePath, "-run", fmt.Sprintf("^%s$", config.TestName))
			case PackageTests:
				if config.PackagePath == "" {
					err := fmt.Errorf("ExecuteTestsCmd: PackageTests requires a valid PackagePath")
					log.Error(err.Error())
					msgChan <- TestRunCompleteMsg{Err: err}
					return
				}
				// Format: go test [baseArgs] <package_path>
				finalArgs = append(baseArgs, config.PackagePath)
			case AllTests:
				// Format: go test [baseArgs] ./...
				finalArgs = append(baseArgs, "./...")
			default:
				err := fmt.Errorf("ExecuteTestsCmd: unknown test target type: %d", config.Type)
				log.Error(err.Error())
				msgChan <- TestRunCompleteMsg{Err: err}
				return
			}

			log.Infof("Executing test command: go %s (in %s)", strings.Join(finalArgs, " "), config.WorkingDir)

			cmd := exec.Command("go", finalArgs...)
			if config.WorkingDir != "" {
				cmd.Dir = config.WorkingDir
			} else {
				cmd.Dir = "." // Default to current directory if not specified
				log.Warn("ExecuteTestsCmd: WorkingDir not specified, defaulting to '.'")
			}

			// Get stdout and stderr pipes
			stdoutPipe, err := cmd.StdoutPipe()
			if err != nil {
				log.Errorf("Error creating stdout pipe: %v", err)
				msgChan <- TestRunCompleteMsg{Err: fmt.Errorf("stdout pipe: %w", err)}
				return
			}

			stderrPipe, err := cmd.StderrPipe()
			if err != nil {
				log.Errorf("Error creating stderr pipe: %v", err)
				msgChan <- TestRunCompleteMsg{Err: fmt.Errorf("stderr pipe: %w", err)}
				return
			}

			if err := cmd.Start(); err != nil {
				log.Errorf("Error starting command 'go %s': %v", strings.Join(finalArgs, " "), err)
				msgChan <- TestRunCompleteMsg{Err: fmt.Errorf("start command 'go %s': %w", strings.Join(finalArgs, " "), err)}
				return
			}

			// Goroutine to capture and log stderr without mixing with JSON on stdout
			// This ensures that build errors or other non-JSON output from go test's stderr
			// are logged but don't interfere with parsing the JSON from stdout.
			var stderrOutput strings.Builder
			stderrDone := make(chan struct{})
			go func() {
				defer close(stderrDone)
				scanner := bufio.NewScanner(stderrPipe)
				for scanner.Scan() {
					line := scanner.Text()
					log.Warnf("[go test stderr] %s", line)
					stderrOutput.WriteString(line + "\n") // Collect for potential error reporting
				}
				if err := scanner.Err(); err != nil {
					log.Errorf("Error reading stderr: %v", err)
				}
			}()

			// Stream stdout (JSON lines)
			stdoutScanner := bufio.NewScanner(stdoutPipe)
			for stdoutScanner.Scan() {
				msgChan <- TestOutputLineMsg{Line: stdoutScanner.Text()}
			}

			// Check for errors during stdout scanning (e.g., pipe closed unexpectedly)
			if err := stdoutScanner.Err(); err != nil {
				log.Errorf("Error scanning stdout: %v", err)
				// This error might indicate issues reading output. The cmd.Wait() error below
				// will likely also reflect a problem.
			}

			// Wait for stderr goroutine to finish processing all stderr output
			<-stderrDone

			// Wait for the command to complete
			waitErr := cmd.Wait()

			// cmd.Wait() error is important. It's non-nil if tests fail or if there's a build error.
			// This is *expected* if tests fail. The JSON output (parsed by the `parser` package)
			// will detail individual test statuses.
			// A non-nil waitErr when no JSON was produced, or if stdoutScanner.Err() occurred,
			// might indicate a more severe problem (e.g., compilation failed completely).
			if waitErr != nil {
				log.Infof("`go test` command finished with error: %v (This is expected if tests failed or build issues occurred)", waitErr)
				// If stderr had content and waitErr is present, it's likely a build error or similar.
				// The JSON parser will handle empty/malformed JSON.
				// The waitErr itself is the primary signal of overall success/failure of the `go test` process.
			} else {
				log.Info("`go test` command finished successfully (exit code 0).")
			}

			msgChan <- TestRunCompleteMsg{Err: waitErr}
		}()

		// Send the StreamMsg first, so the main Update loop knows which channel to listen on.
		return StreamMsg{Stream: msgChan}
	}
}

// WaitForStreamMsgCmd returns a `tea.Cmd` that waits for the next message on the given stream.
// This should be used in the `Update` loop after receiving a `StreamMsg` to process
// subsequent messages from the test execution goroutine.
// If the channel is closed, it returns `nil` (no message), signaling the end of the stream
// from this command's perspective. The `TestRunCompleteMsg` should be the last actual message.
func WaitForStreamMsgCmd(stream <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		if msg, ok := <-stream; ok {
			return msg
		}
		return nil // Channel closed
	}
}

// Helper to read from a pipe and send lines to a channel (not directly used in the above, but a common pattern)
func streamPipe(pipe io.Reader, msgChan chan<- tea.Msg, lineConstructor func(string) tea.Msg, errorLogger func(string, ...interface{})) {
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		msgChan <- lineConstructor(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		errorLogger("Error reading from pipe: %v", err)
	}
}

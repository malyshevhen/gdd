package main

import (
	"fmt"
	"os"

	"gdd/tui" // This will hold our TUI logic

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
)

func main() {
	// Configure logging for the entire application.
	// TUI applications often benefit from logging to a file to avoid corrupting the display.
	// charmbracelet/log is TUI-aware and can handle this gracefully.

	logFilePath := "gdd.log" // Default log file name
	logLevel := log.InfoLevel  // Default log level

	// Allow overriding log file and level via environment variables for debugging
	if os.Getenv("GDD_LOG_FILE") != "" {
		logFilePath = os.Getenv("GDD_LOG_FILE")
	}
	if os.Getenv("GDD_DEBUG") != "" || os.Getenv("GDD_LOG_LEVEL") == "debug" {
		logLevel = log.DebugLevel
		// For debug mode, also log to a more discoverable file if not overridden
		if os.Getenv("GDD_LOG_FILE") == "" { // only if not explicitly set
			logFilePath = "gdd-debug.log"
		}
	}

	f, err := tea.LogToFile(logFilePath, "gdd")
	if err != nil {
		fmt.Printf("Fatal: could not open log file %s: %v\n", logFilePath, err)
		os.Exit(1)
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Printf("Error closing log file %s: %v\n", logFilePath, err)
		}
	}()

	// Use charmbracelet/log as the global logger for all packages.
	// Packages can import "github.com/charmbracelet/log" and use it directly.
	log.SetOutput(f)
	log.SetLevel(logLevel)
	log.SetReportTimestamp(true)
	log.SetReportCaller(true) // Helpful for debugging

	log.Debugf("Logging initialized. Level: %s, File: %s", logLevel.String(), logFilePath)

	// Initialize the main TUI model.
	// The NewMainModel function should also initialize its own internal logger
	// or use the global one configured here.
	// For this project, tui.NewMainModel() will set up its own logger,
	// but it's also fine for other packages like finder, parser, runner
	// to directly use the global `charmbracelet/log`.
	initialModel, err := tui.NewMainModel()
	if err != nil {
		// Log the error using our configured logger before exiting
		log.Fatalf("Could not initialize TUI model: %v", err)
		// Also print to stderr for immediate visibility if logging isn't obvious
		fmt.Printf("Error: Could not initialize TUI model: %v\n", err)
		os.Exit(1)
	}

	// Create and run the Bubble Tea program.
	// tea.WithAltScreen() provides a dedicated screen buffer for the TUI,
	// restoring the original terminal state on exit.
	// tea.WithMouseCellMotion() enables mouse events, though not strictly required by the prompt.
	p := tea.NewProgram(initialModel, tea.WithAltScreen(), tea.WithMouseCellMotion())

	log.Info("Starting GDD Test Runner TUI...")

	// p.Run() blocks until the program exits.
	// The returned model is the final state, and err contains any error that occurred.
	if _, runErr := p.Run(); runErr != nil {
		log.Errorf("TUI program exited with error: %v", runErr)
		fmt.Printf("Alas, there's been an error: %v\n", runErr)
		os.Exit(1)
	}

	log.Info("GDD Test Runner TUI exited gracefully.")
}
package parser

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/log"
)

// TestEvent is a struct for parsing `go test -json` output.
// See `go help testflag` or `go doc cmd/test2json` for more details.
type TestEvent struct {
	Time    time.Time `json:"Time"`
	Action  string    `json:"Action"`
	Package string    `json:"Package"`
	Test    string    `json:"Test,omitempty"`    // Name of test or benchmark
	Elapsed float64   `json:"Elapsed,omitempty"` // seconds
	Output  string    `json:"Output,omitempty"`
}

// TestStatus represents the status of a test or package.
type TestStatus string

const (
	StatusRunning TestStatus = "RUNNING" // Internal status for tracking active tests
	StatusPass    TestStatus = "PASS"
	StatusFail    TestStatus = "FAIL"
	StatusSkip    TestStatus = "SKIP"
	StatusUnknown TestStatus = "UNKNOWN" // Default status before a final event
)

// TestResult holds processed information for a single test function.
type TestResult struct {
	PackageName string
	Name        string // Test function name
	Status      TestStatus
	Output      []string      // Log output from the test, including failure messages
	Duration    time.Duration // Elapsed time for the test
}

// PackageResult holds all test results for a single package.
type PackageResult struct {
	PackageName   string
	Status        TestStatus    // Overall status of the package
	SummaryOutput []string      // General output from the package not tied to a specific test (e.g., build errors, [no test files])
	Tests         []*TestResult // Slice of individual test results in this package
	Duration      time.Duration // Total time for the package (from package-level pass/fail event)
}

// Parse processes the raw byte output from `go test -json` and returns a slice of PackageResult.
// The results are structured hierarchically: a list of packages, each containing its tests.
func Parse(jsonData []byte) ([]*PackageResult, error) {
	packageResults := make(map[string]*PackageResult)
	var orderedPackageNames []string // To maintain the order in which packages appear

	// currentTestResults tracks active tests before they are finalized and added to a PackageResult.
	// Key: "packageName/testName"
	currentTestResults := make(map[string]*TestResult)

	scanner := bufio.NewScanner(bytes.NewReader(jsonData))
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		lineBytes := scanner.Bytes()
		if len(lineBytes) == 0 {
			continue
		}

		var event TestEvent
		if err := json.Unmarshal(lineBytes, &event); err != nil {
			log.Warnf("Failed to unmarshal test event JSON line %d: %s, error: %v", lineNumber, string(lineBytes), err)
			// Attempt to append to the last known package if possible, as it might be non-JSON output
			if len(orderedPackageNames) > 0 {
				lastPkgName := orderedPackageNames[len(orderedPackageNames)-1]
				if pkgResult, ok := packageResults[lastPkgName]; ok {
					pkgResult.SummaryOutput = append(pkgResult.SummaryOutput, fmt.Sprintf("[Malformed JSON line %d]: %s", lineNumber, string(lineBytes)))
				}
			}
			continue
		}

		// Ensure package exists in our map
		pkgResult, pkgExists := packageResults[event.Package]
		if !pkgExists {
			if event.Package == "" && event.Test == "" && event.Action == "output" {
				// This could be preamble output before any package starts, or output from a failed build.
				// We'll create a placeholder package for such "orphaned" top-level output.
				const orphanedOutputPackage = "_orphaned_output_"
				event.Package = orphanedOutputPackage // Assign to a temporary package
			} else if event.Package == "" {
				log.Debugf("Event with empty package name (Action: %s, Test: %s). Skipping.", event.Action, event.Test)
				continue // Skip events with no package, unless it's an output we can catch above
			}

			pkgResult = &PackageResult{
				PackageName: event.Package,
				Status:      StatusUnknown,
				Tests:       []*TestResult{},
			}
			packageResults[event.Package] = pkgResult
			if !containsString(orderedPackageNames, event.Package) {
				orderedPackageNames = append(orderedPackageNames, event.Package)
			}
		}

		testKey := ""
		if event.Test != "" {
			testKey = event.Package + "/" + event.Test
		}

		switch event.Action {
		case "run": // A test or package has started
			if event.Test != "" { // Test function started
				currentTestResults[testKey] = &TestResult{
					PackageName: event.Package,
					Name:        event.Test,
					Status:      StatusRunning,
					Output:      []string{},
				}
				log.Debugf("Test run: %s/%s", event.Package, event.Test)
			} else { // Package started
				pkgResult.Status = StatusRunning
				log.Debugf("Package run: %s", event.Package)
			}

		case "output":
			outputLine := strings.TrimRight(event.Output, "\n") // Trim trailing newline from go test -json output
			if event.Test != "" {                               // Output belongs to a specific test
				if tr, ok := currentTestResults[testKey]; ok {
					tr.Output = append(tr.Output, outputLine)
				} else {
					// Output for a test that hasn't had a "run" event or has already finished.
					// This can happen with t.Log after t.Fatal, or complex TestMain scenarios.
					// Try to append to a completed test if found.
					var foundAndAppended bool
					for _, t := range pkgResult.Tests {
						if t.Name == event.Test {
							t.Output = append(t.Output, outputLine)
							foundAndAppended = true
							break
						}
					}
					if !foundAndAppended {
						log.Debugf("Output for unknown or completed test '%s' in package '%s'. Appending to package summary. Output: %s", event.Test, event.Package, outputLine)
						pkgResult.SummaryOutput = append(pkgResult.SummaryOutput, fmt.Sprintf("[%s] %s", event.Test, outputLine))
					}
				}
			} else { // Output belongs to the package itself or is general output
				pkgResult.SummaryOutput = append(pkgResult.SummaryOutput, outputLine)
			}

		case "pass", "fail", "skip":
			status := TestStatus(strings.ToUpper(event.Action))
			duration := time.Duration(event.Elapsed * float64(time.Second))

			if event.Test != "" { // A test function has finished
				tr, ok := currentTestResults[testKey]
				if !ok {
					// Test finished without a "run" event (e.g., cached result, or t.SkipNow in init/TestMain)
					log.Debugf("Result for test '%s' in package '%s' without prior 'run' event. Action: %s", event.Test, event.Package, event.Action)
					tr = &TestResult{
						PackageName: event.Package,
						Name:        event.Test,
						Output:      []string{}, // Output might have been missed or logged to package
					}
					// If there was output for this test captured at package level, try to move it.
					// This is a heuristic and might not be perfect.
					var newPkgSummary []string
					for _, line := range pkgResult.SummaryOutput {
						if strings.HasPrefix(line, fmt.Sprintf("[%s]", event.Test)) {
							tr.Output = append(tr.Output, strings.TrimSpace(strings.TrimPrefix(line, fmt.Sprintf("[%s]", event.Test))))
						} else {
							newPkgSummary = append(newPkgSummary, line)
						}
					}
					pkgResult.SummaryOutput = newPkgSummary
				}
				tr.Status = status
				tr.Duration = duration

				pkgResult.Tests = append(pkgResult.Tests, tr)
				delete(currentTestResults, testKey) // Test is complete
				log.Debugf("Test %s: %s/%s (%.2fs)", status, event.Package, event.Test, event.Elapsed)
			} else { // A package has finished
				pkgResult.Status = status
				pkgResult.Duration = duration
				log.Debugf("Package %s: %s (%.2fs)", status, event.Package, event.Elapsed)

				// Consolidate remaining currentTestResults for this package if any (shouldn't happen often if go test is well-behaved)
				for key, unfinishedTest := range currentTestResults {
					if unfinishedTest.PackageName == event.Package {
						log.Warnf("Test %s/%s was 'run' but did not complete before package %s finished. Marking as FAIL.", unfinishedTest.PackageName, unfinishedTest.Name, event.Package)
						unfinishedTest.Status = StatusFail
						unfinishedTest.Output = append(unfinishedTest.Output, "Test did not report completion before package finished.")
						pkgResult.Tests = append(pkgResult.Tests, unfinishedTest)
						delete(currentTestResults, key)
						// If package passed but contains failed test, mark package as failed
						if pkgResult.Status == StatusPass {
							pkgResult.Status = StatusFail
						}
					}
				}

				// Handle cases like "[no test files]" or build failures reported at package level
				if (status == StatusFail || status == StatusSkip || status == StatusPass) && len(pkgResult.Tests) == 0 && len(pkgResult.SummaryOutput) > 0 {
					isNoTestFiles := false
					isBuildFailed := false
					for _, line := range pkgResult.SummaryOutput {
						if strings.Contains(line, "[no test files]") || strings.Contains(line, "no Go files") || strings.Contains(line, "no non-test Go files") {
							isNoTestFiles = true
							break
						}
						if strings.HasPrefix(line, "# ") || strings.Contains(line, "FAIL\t") && strings.Contains(line, "[build failed]") { // Heuristic for build failure
							isBuildFailed = true
						}
					}

					if isNoTestFiles {
						placeholderName := fmt.Sprintf("Package %s Status", event.Package)
						pkgResult.Tests = append(pkgResult.Tests, &TestResult{
							PackageName: event.Package,
							Name:        placeholderName,
							Status:      StatusSkip, // Treat "no test files" as effectively skipped
							Output:      pkgResult.SummaryOutput,
							Duration:    pkgResult.Duration,
						})
						pkgResult.SummaryOutput = []string{} // Cleared as it's now in a "test"
					} else if isBuildFailed && status == StatusFail {
						placeholderName := fmt.Sprintf("Package %s Build Failed", event.Package)
						pkgResult.Tests = append(pkgResult.Tests, &TestResult{
							PackageName: event.Package,
							Name:        placeholderName,
							Status:      StatusFail,
							Output:      pkgResult.SummaryOutput,
							Duration:    pkgResult.Duration,
						})
						pkgResult.SummaryOutput = []string{}
					}
				}
			}
		// Ignoring "cont", "pause", "bench" actions for this tool's scope.
		default:
			log.Debugf("Unhandled event action: %s for package %s, test %s", event.Action, event.Package, event.Test)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading test output stream: %w", err)
	}

	// Handle any tests that were "run" but never received a final "pass/fail/skip" event
	// This can happen if the `go test` process crashes or is killed.
	for _, tr := range currentTestResults {
		if tr.Status == StatusRunning {
			log.Warnf("Test %s in package %s was 'run' but never completed. Marking as FAIL.", tr.Name, tr.PackageName)
			tr.Status = StatusFail
			tr.Output = append(tr.Output, "Test did not complete (process might have crashed, timed out, or was terminated).")

			if pkgResult, ok := packageResults[tr.PackageName]; ok {
				pkgResult.Tests = append(pkgResult.Tests, tr)
				// Ensure package status reflects failure if it wasn't already failed.
				if pkgResult.Status != StatusFail {
					pkgResult.Status = StatusFail // Mark package as failed too
					log.Debugf("Marking package %s as FAIL due to incomplete test %s", tr.PackageName, tr.Name)
				}
			} else {
				// This should be rare if package was created on 'run' event for the test.
				log.Errorf("Orphaned running test %s for package %s found. Cannot associate with a package result.", tr.Name, tr.PackageName)
			}
		}
	}

	// Assemble final results in the order packages were encountered
	finalResults := make([]*PackageResult, 0, len(orderedPackageNames))
	for _, pkgName := range orderedPackageNames {
		if pkg, ok := packageResults[pkgName]; ok {
			// Sort tests within each package alphabetically by name for consistent display
			sort.Slice(pkg.Tests, func(i, j int) bool {
				return pkg.Tests[i].Name < pkg.Tests[j].Name
			})
			finalResults = append(finalResults, pkg)
		}
	}

	return finalResults, nil
}

// containsString checks if a slice of strings contains a specific string.
func containsString(slice []string, str string) bool {
	for _, item := range slice {
		if item == str {
			return true
		}
	}
	return false
}

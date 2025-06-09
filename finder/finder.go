package finder

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/log"
)

// TestInfo holds information about a discovered test function.
type TestInfo struct {
	Name        string // Name of the test function (e.g., TestMyFunction)
	PackageName string // Package name declared in the file (e.g., "mypackage")
	PackageDir  string // Directory containing the test file, relative to rootDir (e.g., "app/server")
	FilePath    string // Full path to the test file
	// The "run" target for 'go test' for a single test is typically '<PackageDir> -run ^TestName$'
	// The "run" target for a package is typically '<PackageDir>'
}

// FindTests scans the given root directory for Go test files and extracts test functions.
// It searches recursively starting from the rootDir.
// rootDir should be the module root. PackageDir will be relative to this root.
func FindTests(rootDir string) ([]TestInfo, error) {
	var tests []TestInfo
	fset := token.NewFileSet()

	absRootDir, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("could not get absolute path for rootDir %s: %w", rootDir, err)
	}

	log.Debugf("Starting test discovery in root: %s (absolute: %s)", rootDir, absRootDir)

	err = filepath.Walk(absRootDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			log.Warnf("Error accessing path %q during walk: %v", path, walkErr)
			return walkErr // Propagate error to stop walking if critical, or return nil to continue
		}

		// Skip vendor directories and hidden directories (like .git, .idea, etc.)
		if info.IsDir() {
			if info.Name() == "vendor" || (len(info.Name()) > 1 && info.Name()[0] == '.') {
				log.Debugf("Skipping directory: %s", path)
				return filepath.SkipDir
			}
		}

		if !info.IsDir() && strings.HasSuffix(info.Name(), "_test.go") {
			log.Debugf("Found test file: %s", path)

			// Parse the Go file
			file, parseErr := parser.ParseFile(fset, path, nil, 0) // No need for comments for test discovery
			if parseErr != nil {
				log.Warnf("Could not parse test file %s: %v", path, parseErr)
				return nil // Continue walking, one corrupt file shouldn't stop all discovery
			}

			declaredPackageName := file.Name.Name // Package name from `package foo` line

			// Determine PackageDir relative to the absRootDir
			// e.g., if absRootDir is /home/user/myproject and path is /home/user/myproject/src/mypkg/foo_test.go
			// relPath should be src/mypkg/foo_test.go
			// packageDir should be src/mypkg
			relPath, err := filepath.Rel(absRootDir, path)
			if err != nil {
				log.Warnf("Could not get relative path for %s against %s: %v", path, absRootDir, err)
				// Fallback or skip this file
				return nil
			}
			packageDir := filepath.Dir(relPath)
			// Convert to forward slashes for Go package paths if on Windows
			packageDir = filepath.ToSlash(packageDir)
			// If the test file is in the root, PackageDir will be "."
			// `go test ./...` handles this, and `go test .` also works.
			// `go test . -run ^TestFoo$`

			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue // Not a function declaration
				}

				// Check if the function name starts with "Test"
				// and if it has the correct signature (*testing.T)
				if strings.HasPrefix(fn.Name.Name, "Test") {
					// Basic check: Ensure it's a test function (takes *testing.T or *testing.B or *testing.F)
					// A more robust check would inspect fn.Type.Params.
					// For example, for `func TestFoo(t *testing.T)`:
					// fn.Type.Params.List should have 1 element.
					// That element's Type should be *ast.StarExpr.
					// The StarExpr.X should be *ast.SelectorExpr.
					// The SelectorExpr.X should be an *ast.Ident with Name "testing".
					// The SelectorExpr.Sel should be an *ast.Ident with Name "T" (or "M", "B", "F").

					// Simplified check (as per initial prompt):
					// Just checking prefix "Test" is often good enough for discovery.
					// `go test` itself will filter out non-test functions.
					if isValidTestFunc(fn) {
						log.Debugf("Discovered test function: %s in package %s (dir: ./%s)", fn.Name.Name, declaredPackageName, packageDir)
						tests = append(tests, TestInfo{
							Name:        fn.Name.Name,
							PackageName: declaredPackageName,
							FilePath:    path,       // Store full path, can be useful
							PackageDir:  packageDir, // Relative path for `go test` command
						})
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error walking directory %q: %w", rootDir, err)
	}

	log.Infof("Test discovery complete. Found %d test functions.", len(tests))
	return tests, nil
}

// isValidTestFunc checks if the function signature matches a valid Go test function.
// Valid signatures are:
// func TestXxx(*testing.T)
// func BenchmarkXxx(*testing.B)
// func FuzzXxx(*testing.F)
// func TestMain(*testing.M)
// For this tool, we are primarily interested in TestXxx(*testing.T).
func isValidTestFunc(fn *ast.FuncDecl) bool {
	if fn.Type.Params == nil || len(fn.Type.Params.List) != 1 {
		return false // Must have exactly one parameter
	}

	paramType := fn.Type.Params.List[0].Type
	starExpr, ok := paramType.(*ast.StarExpr)
	if !ok {
		return false // Parameter must be a pointer type
	}

	selectorExpr, ok := starExpr.X.(*ast.SelectorExpr)
	if !ok {
		return false // Pointer must be to a selected type (e.g., testing.T)
	}

	pkgIdent, ok := selectorExpr.X.(*ast.Ident)
	if !ok {
		return false // Selection must be from an identifier (package name)
	}

	// We only care about *testing.T for runnable tests via `go test -run TestFunc`
	return pkgIdent.Name == "testing" && selectorExpr.Sel.Name == "T"
}

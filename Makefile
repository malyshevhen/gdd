# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GORUN=$(GOCMD) run
GOTEST=$(GOCMD) test
GOCLEAN=$(GOCMD) clean
GOMODTIDY=$(GOCMD) mod tidy
GOGET=$(GOCMD) get

PROJECT_NAME=gdd
BUILD_DIR=build

# Output binary name
BINARY_NAME=$(PROJECT_NAME)
ifeq ($(OS),Windows_NT)
    BINARY_NAME:=$(PROJECT_NAME).exe
endif

# Main package (location of main.go)
MAIN_PACKAGE=./main.go

.DEFAULT_GOAL := help

.PHONY: all build run test clean deps lint help install uninstall

# Build the application
all: build

# Build the application binary
build:
	@echo "Building $(BINARY_NAME)..."
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PACKAGE)

# Run the application
run:
	@echo "Running $(PROJECT_NAME)..."
	$(GORUN) $(MAIN_PACKAGE)

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Clean build artifacts and other generated files
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -f $(BUILD_DIR)/$(BINARY_NAME)
	rm -f coverage.out

# Tidy and vendor dependencies
deps:
	@echo "Tidying and verifying dependencies..."
	$(GOMODTIDY)
	$(GOCMD) mod verify

# Lint the code (requires a linter like golangci-lint)
lint:
	@echo "Linting code... (Please ensure golangci-lint is installed and configured)"
	@golangci-lint run ./...
	@echo "Lint target is a placeholder. Add your linting tool command."

# Install the binary to GOPATH/bin or GOBIN
install: build
	@echo "Installing $(BINARY_NAME) to $(GOPATH)/bin/..."
	cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/$(PROJECT_NAME)

# Uninstall the binary from GOPATH/bin or GOBIN
uninstall:
	@echo "Uninstalling $(PROJECT_NAME) from $(GOPATH)/bin/..."
	rm -f $(GOPATH)/bin/$(PROJECT_NAME)

# Display help message
help:
	@echo "Available targets for Makefile:"
	@echo ""
	@echo "  all         Build the application (default when 'make' is called after 'help')"
	@echo "  build       Compile the application and create the binary ($(BINARY_NAME))"
	@echo "  run         Run the application using 'go run'"
	@echo "  test        Run all tests in the project"
	@echo "  clean       Remove build artifacts and the compiled binary"
	@echo "  deps        Tidy 'go.mod' and 'go.sum' files and verify dependencies"
	@echo "  lint        (Placeholder) Lint the Go code (requires a linter tool)"
	@echo "  install     Install the built binary to your GOPATH/bin"
	@echo "  uninstall   Remove the installed binary from your GOPATH/bin"
	@echo "  help        Display this help message"
	@echo ""
	@echo "To make 'all' the default target, comment out '.DEFAULT_GOAL := help'"

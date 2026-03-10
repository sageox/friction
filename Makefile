.PHONY: all build test test-verbose test-short lint fmt vet check coverage clean help

# Default target
all: check

# Build the library
build:
	go build ./...

# Run tests with race detection
test:
	gotestsum --format=testname -- -race ./...

# Run tests with verbose output
test-verbose:
	gotestsum --format=testdox -- -race -v ./...

# Run short tests only (skip long-running tests)
test-short:
	gotestsum --format=testname -- -race -short ./...

# Run linting
lint:
	golangci-lint run ./...

# Format code
fmt:
	gofmt -s -w .
	goimports -w -local github.com/sageox/frictionx .

# Run go vet
vet:
	go vet ./...

# Run all checks (format, lint, test)
check: fmt lint test

# Generate test coverage report
coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Clean build artifacts
clean:
	go clean ./...
	rm -f coverage.out coverage.html

# Show help
help:
	@echo "Available targets:"
	@echo "  all          - Run check (default)"
	@echo "  build        - Build the library"
	@echo "  test         - Run tests with race detection"
	@echo "  test-verbose - Run tests with verbose output"
	@echo "  test-short   - Run short tests only"
	@echo "  lint         - Run golangci-lint"
	@echo "  fmt          - Format code with gofmt and goimports"
	@echo "  vet          - Run go vet"
	@echo "  check        - Run fmt, lint, and test"
	@echo "  coverage     - Generate test coverage report"
	@echo "  clean        - Clean build artifacts"
	@echo "  help         - Show this help"

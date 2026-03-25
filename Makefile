.PHONY: all build test test-race lint coverage coverage-html run install clean deps fmt help

# Default target: lint, test, and build
all: lint test build

# Build the binary
build:
	go build -o agent-deploy ./internal

# Run all tests
test:
	go test ./...

# Run tests with race detector
test-race:
	go test -race ./...

# Run linter
lint:
	golangci-lint run ./...

# Generate coverage report
coverage:
	go test -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out

# Generate HTML coverage report
coverage-html: coverage
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Build and run the server (stdio mode)
run: build
	./agent-deploy

# Install binary to GOPATH/bin
install:
	go install ./internal

# Clean build artifacts
clean:
	rm -f agent-deploy coverage.out coverage.html
	go clean

# Download and tidy dependencies
deps:
	go mod download
	go mod tidy

# Format code
fmt:
	gofmt -w -s .
	goimports -w .

# Show help
help:
	@echo "Available targets:"
	@echo "  all           - lint, test, and build (default)"
	@echo "  build         - build the binary"
	@echo "  test          - run tests"
	@echo "  test-race     - run tests with race detector"
	@echo "  lint          - run golangci-lint"
	@echo "  coverage      - generate coverage report"
	@echo "  coverage-html - generate HTML coverage report"
	@echo "  run           - build and run (stdio mode)"
	@echo "  install       - install to GOPATH/bin"
	@echo "  clean         - remove build artifacts"
	@echo "  deps          - download and tidy dependencies"
	@echo "  fmt           - format code"
	@echo "  help          - show this help"
.PHONY: test test-unit test-integration test-bench test-all bench lint lint-fix coverage clean all help check ci install-tools install-hooks

.DEFAULT_GOAL := help

## help: Display available commands
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | column -t -s ':'

## test: Run all tests with race detector
test:
	@echo "Running tests..."
	@go test -v -race ./...

## test-unit: Run unit tests only (short mode)
test-unit:
	@echo "Running unit tests..."
	@go test -v -race -short $(shell go list ./... | grep -v '/testing/')

## test-integration: Run integration tests
test-integration:
	@echo "Running integration tests..."
	@go test -v ./testing/integration/...

## test-bench: Run performance benchmarks
test-bench:
	@echo "Running benchmarks..."
	@go test -v -bench=. -benchmem ./testing/benchmarks/...

## test-all: Run all tests (unit + integration)
test-all: test test-integration
	@echo "All tests passed!"

## bench: Run benchmarks (legacy alias)
bench:
	@echo "Running benchmarks..."
	@go test -bench=. -benchmem -benchtime=1s ./...

## lint: Run linters
lint:
	@echo "Running linters..."
	@golangci-lint run --config=.golangci.yml --timeout=5m

## lint-fix: Run linters with auto-fix
lint-fix:
	@echo "Running linters with auto-fix..."
	@golangci-lint run --config=.golangci.yml --fix

## coverage: Generate coverage report (HTML)
coverage:
	@echo "Generating coverage report..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@go tool cover -func=coverage.out | tail -1
	@echo "Coverage report generated: coverage.html"

## clean: Remove generated files
clean:
	@echo "Cleaning..."
	@rm -f coverage.out coverage.html
	@find . -name "*.test" -delete
	@find . -name "*.prof" -delete
	@find . -name "*.out" -delete

## check: Quick validation (test + lint)
check: test lint
	@echo "All checks passed!"

## ci: Full CI simulation
ci: clean lint test test-integration coverage
	@echo "Full CI simulation complete!"

## install-tools: Install development tools
install-tools:
	@echo "Installing development tools..."
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.7.2

## install-hooks: Install git hooks
install-hooks:
	@echo "Installing git hooks..."
	@echo '#!/bin/sh' > .git/hooks/pre-commit
	@echo 'make check' >> .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "Pre-commit hook installed"

## all: Run tests and lint (default)
all: test lint

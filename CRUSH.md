# Crush Context for Simple Agent Go

## Project Overview
Simple Agent Go is a modern, high-performance AI agent framework implemented in Go with a terminal UI.

## Build Commands
```bash
# Build the project
make build

# Build for all platforms
make build-all

# Install dependencies
make deps

# Clean build artifacts
make clean
```

## Development Commands
```bash
# Run tests with coverage
make test

# Run tests and generate coverage report
make test-coverage

# Format code
make fmt

# Run linter
make lint

# Vet code
make vet

# Install development dependencies
make dev-deps
```

## Run Commands
```bash
# Run the agent
make run

# Install the binary to GOPATH
make install

# Run with live reload (requires air)
make watch
```

## Go Commands
```bash
# Run tests manually
go test ./...

# Run tests with race detection
go test -race ./...

# Update dependencies
go mod tidy

# Download dependencies
go mod download
```

## Code Style Preferences
- Use `gofumpt` for formatting
- Use `golangci-lint` for linting
- Follow Go idioms and conventions
- Use struct tags for tool metadata instead of decorators
- Implement interfaces for LLM clients and tools
- Use goroutines for concurrent tool execution

## Project Structure
- `agent/` - Core agent logic
- `llm/` - LLM provider clients
- `tools/` - Tool implementations
- `tui/` - Terminal UI components
- `cmd/` - CLI entry point
- `config/` - Configuration management
- `internal/` - Internal packages

## Key Technologies
- Go 1.24+
- Bubble Tea TUI framework
- Lipgloss styling
- Cobra CLI framework
- godotenv for environment variables

## Pull Request Workflow
When requested to "create a PR" or similar, use the GitHub CLI (gh) to create a pull request with:
- A clear, descriptive title
- A comprehensive description including:
  - Summary of changes
  - Implementation details
  - Benefits and impact
- Proper formatting with headers and bullet points
- Include "💘 Generated with Crush" signature
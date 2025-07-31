# Simple Agent Go - AI Agent Framework

## Overview

Simple Agent Go is a modern, high-performance implementation of the Simple Agent framework in Go. It leverages Go's excellent concurrency model and the Bubble Tea TUI framework to provide a superior terminal experience compared to the Ruby and Python implementations. The framework maintains compatibility with the same tool ecosystem while offering better performance, type safety, and a single-binary distribution.

## Key Technologies

- **Go 1.21+** - Modern Go with generics support
- **Bubble Tea** - Elm-inspired TUI framework for beautiful terminal apps
- **Lipgloss** - Style definitions for TUI components
- **Glamour** - Markdown rendering in the terminal
- **Cobra** - CLI framework for commands and flags
- **Viper** - Configuration management

## Architecture

### Core Components

1. **Agent (`agent/agent.go`)**
   - Main orchestrator managing conversations with LLMs
   - Supports both ReAct-style prompting and function calling
   - Concurrent tool execution with goroutines
   - Context-aware cancellation support

2. **LLM Clients (`llm/`)**
   - Interface: `LLMClient` - Defines the contract
   - Implementations: `OpenAIClient`, `AnthropicClient`, `MoonshotClient`, etc.
   - Streaming support using channels
   - Automatic retry with exponential backoff

3. **Tools (`tools/`)**
   - Interface: `Tool` - All tools implement this
   - `ToolRegistry` - Singleton managing tool discovery
   - `ToolMetadata` - Metadata and JSON schema generation
   - Auto-discovery using reflection
   - Concurrent execution support

4. **TUI (`tui/`)**
   - `App` - Main Bubble Tea application
   - `InputArea` - Advanced input with syntax highlighting
   - `ChatView` - Conversation display with markdown
   - `ToolsPanel` - Live tool execution status
   - `StatusBar` - Connection and model info

### Available Tools

- **CalculateTool** - Safe math evaluation
- **WikipediaTool** - Wikipedia search
- **GoogleSearchTool** - Web search via Custom Search API
- **FileReadTool** - Read file contents
- **FileWriteTool** - Write content to files
- **FileEditTool** - Edit files with string replacement
- **DirectoryListTool** - List directory contents
- **ShellTool** - Execute shell commands (with safeguards)

## Installation & Setup

### From Source

```bash
# Clone the repository
git clone https://github.com/simple-agent/simple-agent-go
cd simple-agent-go

# Build the binary
go build -o simple-agent cmd/simple-agent/main.go

# Or install directly
go install github.com/simple-agent/simple-agent-go/cmd/simple-agent@latest
```

### Using Homebrew

```bash
brew tap simple-agent/tap
brew install simple-agent
```

### Binary Releases

Download pre-built binaries from the GitHub releases page.

## Quick Start

### Basic Usage

```bash
# Start the TUI interface (default)
simple-agent

# Start with a specific provider and model
simple-agent --provider openai --model gpt-4

# Continue your last conversation
simple-agent --continue

# One-shot query (no TUI)
simple-agent query "What is the capital of France?"
```

### Environment Variables

Create a `.env` file or export these variables:

```bash
# OpenAI
export OPENAI_API_KEY="sk-..."

# Anthropic
export ANTHROPIC_API_KEY="sk-ant-..."

# Other providers
export MOONSHOT_API_KEY="..."
export DEEPSEEK_API_KEY="..."

# For Google Search tool
export GOOGLE_API_KEY="..."
export GOOGLE_CX="..."
```

## Creating a New Agent

### Method 1: Custom System Prompt (Recommended)

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/simple-agent/simple-agent-go/agent"
    "github.com/simple-agent/simple-agent-go/llm/openai"
)

const customPrompt = `
You are a helpful coding assistant specialized in Go.
Focus on writing idiomatic Go code with proper error handling.
Use the available tools when needed.
`

func main() {
    // Create LLM client
    client, err := openai.NewClient(openai.WithModel("gpt-4"))
    if err != nil {
        log.Fatal(err)
    }
    
    // Create agent with custom prompt
    ag := agent.New(
        agent.WithLLMClient(client),
        agent.WithSystemPrompt(customPrompt),
        agent.WithMaxIterations(10),
    )
    
    // Query the agent
    response, err := ag.Query(context.Background(), "Help me write a Go HTTP server")
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Println(response)
}
```

### Method 2: Agent Configuration

```go
// config/agents.go
package config

import "github.com/simple-agent/simple-agent-go/agent"

// CodingAssistant returns a pre-configured coding assistant
func CodingAssistant() agent.Config {
    return agent.Config{
        SystemPrompt: `You are an expert Go developer...`,
        MaxIterations: 15,
        Temperature: 0.7,
        Tools: []string{"file_read", "file_write", "shell"},
    }
}

// Usage
ag := agent.New(
    agent.WithConfig(config.CodingAssistant()),
    agent.WithLLMClient(client),
)
```

## Creating a New Tool

Tools are automatically discovered. To create a new tool:

1. Create a file in `tools/` (e.g., `weather_tool.go`)
2. Implement the `Tool` interface
3. Register metadata for JSON schema generation

```go
package tools

import (
    "context"
    "encoding/json"
    "fmt"
    "math/rand"
    
    "github.com/simple-agent/simple-agent-go/tools/base"
)

// WeatherTool gets weather information for a city
type WeatherTool struct {
    base.BaseTool
}

// NewWeatherTool creates a new weather tool instance
func NewWeatherTool() *WeatherTool {
    return &WeatherTool{
        BaseTool: base.BaseTool{
            ToolName: "weather",
            ToolDesc: "Get current weather for a given city. Returns temperature and conditions.",
        },
    }
}

// Schema returns the JSON schema for this tool
func (w *WeatherTool) Schema() map[string]interface{} {
    return map[string]interface{}{
        "type": "function",
        "function": map[string]interface{}{
            "name": w.Name(),
            "description": w.Description(),
            "parameters": map[string]interface{}{
                "type": "object",
                "properties": map[string]interface{}{
                    "city": map[string]interface{}{
                        "type": "string",
                        "description": "The city to get weather for",
                    },
                },
                "required": []string{"city"},
            },
        },
    }
}

// Execute runs the tool with the given parameters
func (w *WeatherTool) Execute(ctx context.Context, params string) (string, error) {
    var args struct {
        City string `json:"city"`
    }
    
    if err := json.Unmarshal([]byte(params), &args); err != nil {
        return "", fmt.Errorf("invalid parameters: %w", err)
    }
    
    if args.City == "" {
        return "", fmt.Errorf("city parameter is required")
    }
    
    // Mock implementation - replace with actual API call
    temp := rand.Intn(35)
    conditions := []string{"sunny", "cloudy", "rainy"}[rand.Intn(3)]
    
    return fmt.Sprintf("Weather in %s: %d°C, %s", args.City, temp, conditions), nil
}

// Register the tool (called automatically by init)
func init() {
    base.RegisterTool("weather", NewWeatherTool)
}
```

## TUI Features

### Keyboard Shortcuts

- **Global**:
  - `Ctrl+C`, `Ctrl+D`: Quit application
  - `Ctrl+L`: Clear chat
  - `Tab`: Switch between panels
  - `F1`: Show help

- **Input Area**:
  - `Enter`: Send message
  - `Shift+Enter`: New line
  - `Ctrl+A`: Select all
  - `Ctrl+K`: Clear input
  - `Up/Down`: Navigate history

- **Chat View**:
  - `j/k`: Scroll down/up
  - `g/G`: Go to top/bottom
  - `Space`: Page down
  - `b`: Page up

### Commands

Commands available in the input area:

- `/help` - Show available commands
- `/tools` - List available tools
- `/model` - Change model interactively
- `/clear` - Clear conversation history
- `/save [name]` - Save current conversation
- `/load` - Load a saved conversation
- `/theme` - Change color theme
- `/export [format]` - Export conversation (md, json, txt)

### Advanced Features

1. **Split-Pane Layout**: Resizable panes with tool status sidebar
2. **Syntax Highlighting**: Automatic language detection for code blocks
3. **Markdown Rendering**: Beautiful formatting for responses
4. **Live Tool Status**: Real-time updates during tool execution
5. **Multi-Tab Support**: Multiple concurrent conversations
6. **Search**: Full-text search in conversation history

## Configuration

### Configuration File

Create `~/.simple-agent/config.yaml`:

```yaml
# Default provider and model
default_provider: openai
default_model: gpt-4

# UI preferences
ui:
  theme: dracula
  show_tool_panel: true
  panel_width: 30
  syntax_theme: monokai

# Tool settings
tools:
  shell:
    allowed_commands: ["ls", "cat", "grep", "find"]
    timeout: 30s
  file_write:
    allowed_paths: ["./workspace", "/tmp"]

# Provider settings
providers:
  openai:
    timeout: 60s
    max_retries: 3
  local:
    lm_studio_url: http://localhost:1234
    ollama_url: http://localhost:11434
```

### Themes

Built-in themes:
- `default` - Balanced light theme
- `dracula` - Popular dark theme
- `nord` - Nordic-inspired theme
- `solarized` - Solarized dark
- `gruvbox` - Retro groove theme

## Development

### Project Structure

```
simple-agent-go/
├── cmd/
│   └── simple-agent/
│       └── main.go         # Entry point
├── agent/
│   ├── agent.go           # Core agent logic
│   ├── config.go          # Configuration types
│   └── memory.go          # Conversation memory
├── llm/
│   ├── client.go          # LLMClient interface
│   ├── openai/            # OpenAI implementation
│   ├── anthropic/         # Anthropic implementation
│   └── ...                # Other providers
├── tools/
│   ├── base/              # Base tool types
│   ├── registry.go        # Tool registry
│   └── ...                # Individual tools
├── tui/
│   ├── app.go            # Main Bubble Tea app
│   ├── components/        # TUI components
│   └── styles/           # Lipgloss styles
├── config/
│   └── config.go         # Configuration management
└── CLAUDE.md             # This documentation
```

### Building a Custom Tool

```go
// Minimal tool implementation
type MyTool struct{}

func (t *MyTool) Name() string { return "my_tool" }
func (t *MyTool) Description() string { return "Does something useful" }
func (t *MyTool) Schema() map[string]interface{} {
    // Return OpenAI function schema
}
func (t *MyTool) Execute(ctx context.Context, params string) (string, error) {
    // Tool implementation
}
```

### Testing

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package tests
go test ./agent

# Run with race detection
go test -race ./...

# Benchmark tests
go test -bench=. ./...
```

## Troubleshooting

### Common Issues

1. **"command not found"**
   - Ensure `$GOPATH/bin` is in your PATH
   - Or use the full path to the binary

2. **TUI rendering issues**
   - Check terminal compatibility (requires 256 color support)
   - Try different terminal emulators
   - Update to latest Bubble Tea version

3. **Tool not discovered**
   - Ensure tool is in `tools/` directory
   - Check that init() registers the tool
   - Verify tool implements the Tool interface

4. **Connection errors**
   - Verify API keys are set correctly
   - Check network connectivity
   - Review provider-specific error messages

5. **High memory usage**
   - Limit conversation history
   - Use streaming for long responses
   - Profile with `pprof` if needed

## Performance Tips

1. **Use streaming**: Enable for real-time responses
2. **Concurrent tools**: Tools execute in parallel by default
3. **Caching**: Enable response caching for repeated queries
4. **Connection pooling**: Reuse HTTP connections
5. **Context cancellation**: Use contexts for proper cleanup

## Advanced Usage

### Programmatic API

```go
// Use agent as a library
import "github.com/simple-agent/simple-agent-go/agent"

// Create agent
ag := agent.New(...)

// Single query
response, _ := ag.Query(ctx, "Hello")

// Streaming query
stream, _ := ag.QueryStream(ctx, "Tell me a story")
for chunk := range stream {
    fmt.Print(chunk)
}

// With tools
ag.RegisterTool(NewCustomTool())
response, _ = ag.Query(ctx, "Use my custom tool")
```

### Plugin System

```go
// Create a plugin
package main

import "github.com/simple-agent/simple-agent-go/plugin"

var Plugin = plugin.Plugin{
    Name: "my-plugin",
    Tools: []plugin.Tool{
        &MyCustomTool{},
    },
}

// Load in main app
simple-agent --plugin ./my-plugin.so
```

## Quick Reference

### Command Line

```bash
simple-agent                    # Start TUI
simple-agent -c                 # Continue last session
simple-agent --provider openai  # Use specific provider
simple-agent query "..."        # One-shot query
simple-agent tools list         # List available tools
simple-agent config set ...     # Update configuration
```

### Environment Variables

```bash
SIMPLE_AGENT_PROVIDER=openai
SIMPLE_AGENT_MODEL=gpt-4
SIMPLE_AGENT_THEME=dracula
SIMPLE_AGENT_DEBUG=true
```

### TUI Commands

```
/help     - Show help
/tools    - List tools
/model    - Change model
/clear    - Clear chat
/save     - Save conversation
/load     - Load conversation
/export   - Export chat
/theme    - Change theme
```

## Contributing

1. Fork the repository
2. Create your feature branch
3. Run tests and linting
4. Submit a pull request

Remember: Simple Agent Go combines the best of both worlds - the simplicity and tool ecosystem of the original Ruby/Python implementations with Go's performance, type safety, and the beautiful Bubble Tea TUI framework!
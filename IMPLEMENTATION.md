# Simple Agent Go - Implementation Plan

## Overview

This document outlines the implementation plan for creating Simple Agent Go, a port of Simple Agent Ruby/Python to Go. The key differentiator is leveraging Go's excellent TUI capabilities through libraries like Bubble Tea, which provides a more robust terminal UI experience compared to Ruby's limited TUI ecosystem and Python Textual's background color limitations.

## Key Advantages of Go Implementation

1. **Bubble Tea TUI Framework**: Elm-inspired architecture with excellent terminal handling
2. **Native Concurrency**: Goroutines and channels for efficient parallel tool execution
3. **Single Binary Distribution**: Easy deployment without runtime dependencies
4. **Type Safety**: Compile-time type checking with Go's type system
5. **Performance**: Compiled language with efficient memory usage

## Phase 1: Core Infrastructure

### 1.1 Project Setup
- [ ] Initialize Go module: `go mod init github.com/simple-agent/simple-agent-go`
- [ ] Set up project structure following Go conventions
- [ ] Configure development tools (golangci-lint, gofumpt, etc.)
- [ ] Create `.env.example` file
- [ ] Set up GitHub Actions for CI/CD
- [ ] Add Makefile for common tasks

### 1.2 Base Types and Interfaces
- [ ] Define `Tool` interface with type-safe parameter handling
- [ ] Create struct tag-based schema generation system
- [ ] Build `ToolRegistry` with explicit registration pattern
- [ ] Implement reflection-based schema generator for OpenAI/Anthropic APIs
- [ ] Define message types (User, Assistant, Tool, System)
- [ ] Create validation framework using struct tags

### 1.3 LLM Client Architecture
- [ ] Create `LLMClient` interface
- [ ] Define common types: `Message`, `ToolCall`, `ChatResponse`
- [ ] Implement streaming response support using channels
- [ ] Add retry logic with exponential backoff
- [ ] Context-based cancellation support

## Phase 2: LLM Provider Implementations

### 2.1 OpenAI Client
- [ ] Implement OpenAI API integration
- [ ] Support function calling with proper JSON marshaling
- [ ] Add streaming responses using SSE
- [ ] Handle rate limiting with retry logic
- [ ] Support multiple models (GPT-4, GPT-3.5, etc.)

### 2.2 Anthropic Client (Claude)
- [ ] Implement Anthropic API integration
- [ ] Support tool use with proper formatting
- [ ] Add streaming responses
- [ ] Handle Claude-specific requirements
- [ ] Support all Claude models

### 2.3 Alternative Providers
- [ ] Moonshot Client (kimi models)
- [ ] DeepSeek Client
- [ ] Perplexity Client
- [ ] LM Studio Client (local models with dynamic discovery)
- [ ] Ollama Client (local models with dynamic discovery)
- [ ] Groq Client (fast inference)

## Phase 3: Tool System Architecture

### 3.0 Go-Idiomatic Tool Design

#### Tool Interface
```go
type Tool interface {
    Name() string
    Description() string
    Execute(ctx context.Context, params json.RawMessage) (string, error)
    Parameters() interface{} // Returns params struct for schema generation
}
```

#### Struct Tag-Based Parameters
```go
type FileReadParams struct {
    Path     string `json:"path" schema:"required" description:"File path to read"`
    Encoding string `json:"encoding,omitempty" schema:"enum:utf-8,ascii" description:"File encoding"`
    MaxLines int    `json:"max_lines,omitempty" schema:"min:1,max:10000" description:"Maximum lines to read"`
}
```

#### Schema Generation
```go
// Automatic schema generation from struct tags
func GenerateSchema(tool Tool) map[string]interface{} {
    return SchemaGenerator{}.FromStruct(tool.Parameters())
}

// Schema generator uses reflection to build OpenAI-compatible schemas
type SchemaGenerator struct {
    // Handles nested structs, arrays, custom types
}
```

#### Registration Pattern
```go
// tools/registry.go
func Register(name string, factory func() Tool) {
    defaultRegistry.Register(name, factory)
}

// Each tool file
func init() {
    Register("file_read", func() Tool {
        return &FileReadTool{}
    })
}
```

#### Type-Safe Execution
```go
func (t *FileReadTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
    var args FileReadParams
    if err := json.Unmarshal(params, &args); err != nil {
        return "", fmt.Errorf("invalid parameters: %w", err)
    }
    
    // Automatic validation via struct tags
    if err := validator.Validate(args); err != nil {
        return "", fmt.Errorf("validation failed: %w", err)
    }
    
    // Tool implementation
    return t.readFile(ctx, args)
}
```

#### Error Types
```go
type ToolError struct {
    Code    string
    Message string
    Details map[string]interface{}
}

func (e *ToolError) Error() string {
    return fmt.Sprintf("%s: %s", e.Code, e.Message)
}
```

### 3.1 File System Tools
- [ ] `FileReadTool` - Read file contents with error handling
- [ ] `FileWriteTool` - Write to files with atomic operations
- [ ] `FileEditTool` - Edit files with string replacement
- [ ] `DirectoryListTool` - List directory contents with filtering

### 3.2 Information Tools
- [ ] `CalculateTool` - Safe math evaluation using govaluate
- [ ] `WikipediaTool` - Wikipedia search with go-wiki
- [ ] `GoogleSearchTool` - Web search using Custom Search API
- [ ] `ShellTool` - Execute shell commands safely

### 3.3 Tool Features
- [ ] Explicit tool registration using init() pattern
- [ ] Struct tag-based validation with meaningful errors
- [ ] Tool usage metrics and structured logging
- [ ] Parallel tool execution with goroutines
- [ ] Tool result caching with context awareness
- [ ] Middleware pipeline for tool execution

## Phase 4: Agent Implementation

### 4.1 Core Agent
- [ ] Main `Agent` struct with configuration
- [ ] ReAct-style prompting support
- [ ] Function calling with automatic tool selection
- [ ] Conversation memory management
- [ ] Custom system prompt support
- [ ] Token counting and limits

### 4.2 Agent Features
- [ ] Tool execution with goroutine pools
- [ ] Response formatting with markdown support
- [ ] Token usage tracking and reporting
- [ ] Conversation export/import (JSON format)
- [ ] Checkpoint and resume support
- [ ] Concurrent tool execution

### 4.3 Specialized Agents
- [ ] `ConfigurableAgent` base type
- [ ] Example: Coding Assistant
- [ ] Example: Research Assistant
- [ ] Example: DevOps Assistant
- [ ] Agent templates system

## Phase 5: Bubble Tea TUI Implementation

**Design Goal**: Create a beautiful, responsive TUI that surpasses both Ruby and Python implementations, taking full advantage of Bubble Tea's capabilities.

### 5.1 Core TUI Architecture
- [ ] Main Bubble Tea application model
- [ ] Component-based architecture with sub-models
- [ ] Event-driven updates using messages
- [ ] Smooth animations and transitions

### 5.2 Advanced Input Component
- [ ] **Dynamic Input Area**: 
  - Resizable input box with smooth animations
  - Multi-line support with proper cursor handling
  - Syntax highlighting for code snippets
  - Auto-completion with fuzzy search
  - Vi/Emacs key bindings support
- [ ] **Input Features**:
  - Command history with persistent storage
  - Smart paste detection and handling
  - Inline file path completion
  - Markdown preview mode
  - Code block detection

### 5.3 Enhanced Output Display
- [ ] **Split-Pane Layout**:
  - Adjustable panes with mouse/keyboard resize
  - Tool execution sidebar with live updates
  - Main conversation view with smooth scrolling
  - Collapsible sections for long outputs
- [ ] **Rich Rendering**:
  - Syntax highlighting with Chroma
  - Markdown rendering with Glamour
  - Tables with Lipgloss styling
  - Image support (ASCII art conversion)
  - Inline charts and graphs

### 5.4 Tool Execution Visualization
- [ ] **Live Tool Status**:
  - Progress bars for long-running tools
  - Spinner animations during execution
  - Color-coded status indicators
  - Execution time tracking
  - Resource usage meters
- [ ] **Tool Inspector**:
  - Expandable tool details
  - Input/output preview
  - Error highlighting
  - Retry options

### 5.5 Advanced TUI Features
- [ ] **Themes and Customization**:
  - Multiple built-in themes
  - Custom color scheme support
  - Font size adjustment
  - Layout preferences
- [ ] **Keyboard Shortcuts**:
  - Customizable key bindings
  - Command palette (Cmd+K style)
  - Quick actions menu
  - Global search
- [ ] **Session Management**:
  - Tab support for multiple conversations
  - Session switching with previews
  - Auto-save with crash recovery
  - Export to various formats
- [ ] **Performance Features**:
  - Virtual scrolling for long conversations
  - Lazy loading of history
  - Efficient re-rendering
  - Background task queue

### 5.6 TUI Layout Mockup
```
┌─────────────────────────────────────────────────────────────────┐
│ Simple Agent Go │ GPT-4 │ Tools: 12 │ ◉ Connected │ 16:42:15   │
├─────────────────┬───────────────────────────────────────────────┤
│ Tools & Status  │                                               │
│ ───────────────│  Chat Conversation                             │
│ 🔍 Search (2s) │  ────────────────                              │
│   ├─ Query: Go │                                                │
│   └─ Results:5 │  You: What's the best way to handle errors   │
│                │       in Go?                                   │
│ 📁 File Read   │                                                │
│   ✓ Complete   │  Assistant: Error handling in Go follows      │
│                │  several best practices:                       │
│ 🧮 Calculate   │                                                │
│   ⚡ Running... │  1. **Return errors explicitly**               │
│                │     ```go                                      │
│ ───────────────│     func doSomething() error {                │
│ Session Info   │         // Return error as last value         │
│ • Messages: 42 │     }                                          │
│ • Tokens: 3.2k │     ```                                        │
│ • Duration: 5m │                                                │
│                │  2. **Check errors immediately**               │
│ ───────────────│     Always handle errors where they occur     │
│ Quick Actions  │                                                │
│ [S]ave [L]oad  │  [Tool Output: FileRead - main.go]             │
│ [C]lear [H]elp │  ┌─────────────────────────────────┐          │
│                │  │ package main                    │          │
├─────────────────┴─│ import "fmt"                    │──────────┤
│ ┌───────────────────────────────────────────────────┐          │
│ │ > Type your message here...                       │          │
│ │   Support multi-line with Shift+Enter             │          │
│ │   /help for commands, Tab for completion          │          │
│ └───────────────────────────────────────────────────┘          │
│ [Enter] Send │ [Ctrl+L] Clear │ [Ctrl+C] Cancel │ [F1] Help    │
└─────────────────────────────────────────────────────────────────┘
```

## Phase 6: Command Line Interface

### 6.1 CLI Framework (Cobra)
- [ ] Main command structure
- [ ] Subcommands for different modes
- [ ] Flag parsing and validation
- [ ] Environment variable support
- [ ] Configuration file support (.simple-agent.yaml)

### 6.2 Command Line Arguments
```bash
# Start TUI (default)
simple-agent

# Start with specific provider/model
simple-agent --provider openai --model gpt-4
simple-agent --provider anthropic --model claude-3-opus
simple-agent --provider local --model llama2

# Continue last session
simple-agent --continue
simple-agent -c

# Resume specific session
simple-agent --resume session-id

# One-shot mode (no TUI)
simple-agent query "What is the capital of France?"

# Tool management
simple-agent tools list
simple-agent tools info wikipedia

# Configuration
simple-agent config set provider openai
simple-agent config get model
```

### 6.3 Model Discovery
- [ ] **Local Model Discovery**:
  - LM Studio: Query `http://localhost:1234/v1/models`
  - Ollama: Query `http://localhost:11434/api/tags`
  - Automatic detection of running services
  - Model capability detection
- [ ] **Model Picker**:
  - Interactive selection with arrow keys
  - Model details preview
  - Performance indicators
  - Favorite models

## Phase 7: Testing & Quality

### 7.1 Test Suite
- [ ] Unit tests with testify
- [ ] Integration tests for providers
- [ ] End-to-end agent tests
- [ ] TUI component tests
- [ ] Benchmark tests
- [ ] Fuzz testing for tools

### 7.2 Documentation
- [ ] API documentation with godoc
- [ ] README with quick start
- [ ] Provider setup guides
- [ ] Tool development guide
- [ ] TUI customization guide
- [ ] Architecture diagrams

### 7.3 Examples
- [ ] Basic agent usage
- [ ] Custom tool creation
- [ ] Provider configuration
- [ ] TUI customization
- [ ] Plugin development

## Phase 8: Advanced Features

### 8.1 Performance
- [ ] Connection pooling for API clients
- [ ] Response caching with TTL
- [ ] Parallel tool execution
- [ ] Token usage optimization
- [ ] Memory-efficient streaming

### 8.2 Observability
- [ ] Structured logging with slog
- [ ] OpenTelemetry integration
- [ ] Prometheus metrics
- [ ] Error tracking (Sentry)
- [ ] Performance profiling

### 8.3 Extensions
- [ ] Plugin system using Go plugins
- [ ] Custom provider SDK
- [ ] Webhook integrations
- [ ] REST API server mode
- [ ] GraphQL API support

### 8.4 Security
- [ ] Secret management (no hardcoded keys)
- [ ] Input sanitization
- [ ] Tool execution sandboxing
- [ ] API key encryption
- [ ] Audit logging

## Phase 9: Distribution

### 9.1 Packaging
- [ ] GitHub releases with goreleaser
- [ ] Homebrew formula
- [ ] AUR package
- [ ] Snap package
- [ ] Docker images
- [ ] Nix package

### 9.2 Installation
- [ ] Install script
- [ ] Binary verification
- [ ] Auto-update mechanism
- [ ] Version management

## Technical Decisions

### Architecture Choices
1. **Interface-First Design**: Define interfaces before implementations
2. **Dependency Injection**: Use interfaces for testability
3. **Context Everywhere**: Proper cancellation and timeout support
4. **Error Wrapping**: Rich error context with errors.Wrap

### Library Choices
1. **charmbracelet/bubbletea**: TUI framework
2. **charmbracelet/lipgloss**: Styling
3. **charmbracelet/glamour**: Markdown rendering
4. **alecthomas/chroma**: Syntax highlighting
5. **spf13/cobra**: CLI framework
6. **spf13/viper**: Configuration
7. **stretchr/testify**: Testing utilities
8. **go-resty/resty**: HTTP client
9. **tidwall/gjson**: Fast JSON parsing
10. **go-playground/validator**: Struct validation via tags
11. **invopop/jsonschema**: JSON Schema generation from Go types
12. **pkg/errors**: Error wrapping with stack traces
13. **uber-go/zap**: Structured logging
14. **golang.org/x/sync/errgroup**: Concurrent execution

### Design Patterns
1. **Repository Pattern**: For conversation storage
2. **Strategy Pattern**: For LLM providers  
3. **Observer Pattern**: For TUI events
4. **Factory Pattern**: For tool creation
5. **Middleware Pattern**: For tool execution pipeline
6. **Functional Options**: For configurable components
7. **Embedded Types**: For composition and shared behavior
8. **Interface Segregation**: Small, focused interfaces

### Go-Specific Patterns
1. **Struct Tags for Metadata**: Replace decorators with tags
2. **Init Registration**: Tools self-register on import
3. **Context Propagation**: First-class cancellation/timeout
4. **Error Wrapping**: Rich error context with custom types
5. **Channel-Based Streaming**: For LLM responses
6. **Table-Driven Tests**: For schema generation validation

## Success Criteria

1. **Feature Parity**: All Ruby/Python features implemented
2. **Superior TUI**: Best-in-class terminal experience
3. **Performance**: Faster response times than Python/Ruby
4. **Reliability**: Comprehensive error handling
5. **Distribution**: Single binary, easy installation
6. **Documentation**: Clear, comprehensive guides

## Key Differentiators from Ruby/Python

1. **TUI Excellence**: Bubble Tea provides superior terminal handling
2. **Compilation**: Type safety and performance
3. **Concurrency**: Native goroutines for parallel operations
4. **Distribution**: Single binary without dependencies
5. **Memory Efficiency**: Lower resource usage
6. **Type-Safe Tools**: Struct-based parameters with compile-time checks
7. **Native Schema Generation**: No manual JSON building required

## Example: Complete Tool Implementation

```go
// tools/weather/weather.go
package weather

import (
    "context"
    "encoding/json"
    "fmt"
    
    "github.com/simple-agent/simple-agent-go/tools"
    "github.com/simple-agent/simple-agent-go/tools/base"
)

// Parameters with struct tags for schema generation
type Params struct {
    City    string `json:"city" schema:"required" description:"City name"`
    Country string `json:"country,omitempty" description:"Country code (ISO)"`
    Units   string `json:"units,omitempty" schema:"enum:metric,imperial" default:"metric"`
}

// WeatherTool implements the Tool interface
type WeatherTool struct {
    base.BaseTool
    apiKey string
}

// NewWeatherTool creates a new weather tool instance
func NewWeatherTool(apiKey string) *WeatherTool {
    return &WeatherTool{
        BaseTool: base.BaseTool{
            ToolName: "weather",
            ToolDesc: "Get current weather for a city",
        },
        apiKey: apiKey,
    }
}

// Parameters returns the params struct for schema generation
func (w *WeatherTool) Parameters() interface{} {
    return &Params{}
}

// Execute runs the tool with type-safe parameters
func (w *WeatherTool) Execute(ctx context.Context, rawParams json.RawMessage) (string, error) {
    var params Params
    if err := json.Unmarshal(rawParams, &params); err != nil {
        return "", &tools.ToolError{
            Code:    "INVALID_PARAMS",
            Message: "Failed to parse parameters",
            Details: map[string]interface{}{"error": err.Error()},
        }
    }
    
    // Validation happens automatically
    if err := tools.Validate(params); err != nil {
        return "", err
    }
    
    // Implementation
    weather, err := w.fetchWeather(ctx, params)
    if err != nil {
        return "", fmt.Errorf("fetch weather: %w", err)
    }
    
    return weather, nil
}

// Self-registration on import
func init() {
    tools.Register("weather", func() tools.Tool {
        return NewWeatherTool(os.Getenv("WEATHER_API_KEY"))
    })
}
```

## Implementation Priority

1. Core infrastructure and interfaces
2. OpenAI provider with basic tools
3. Bubble Tea TUI with essential features
4. Additional providers and tools
5. Advanced TUI features
6. Distribution and packaging

## Next Steps

1. Set up Go module and project structure
2. Define core interfaces (Tool, LLMClient)
3. Implement OpenAI client
4. Create basic Bubble Tea TUI
5. Port essential tools
6. Add provider discovery
7. Enhance TUI with advanced features
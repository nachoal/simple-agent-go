# Simple Agent Go

A modern, high-performance AI agent framework written in Go. This is a Go implementation of the Simple Agent framework, maintaining compatibility with the Ruby and Python versions while offering superior performance through Go's concurrency model and a beautiful terminal UI built with Bubble Tea.

## ✨ Features

- 🚀 **High Performance** - Leverages Go's concurrency model for parallel tool execution
- 🎨 **Beautiful TUI** - Minimal, bordered interface matching Python's design
- 🤖 **Multiple LLM Support** - 8 providers including OpenAI, Anthropic, and local models
- 🔧 **Rich Tool Ecosystem** - 8 built-in tools with automatic discovery
- 📦 **Single Binary** - No runtime dependencies, easy distribution
- 🔄 **Framework Compatible** - Works with the same patterns as Ruby/Python versions
- 💬 **Natural Chat Flow** - Messages push input down, no alt-screen mode
- 🎯 **Smart Features** - Auto-growing input box, text wrapping, responsive design

## 🚀 Installation

```bash
# From source
go install github.com/nachoal/simple-agent-go/cmd/simple-agent@latest

# Or clone and build
git clone https://github.com/nachoal/simple-agent-go
cd simple-agent-go
go build -o simple-agent cmd/simple-agent/main.go

# Or use make
make build
```

## 🔑 Configuration

Create a `.env` file with your API keys:

```bash
# OpenAI
OPENAI_API_KEY=sk-...

# Anthropic  
ANTHROPIC_API_KEY=sk-ant-...

# Other providers
MOONSHOT_API_KEY=...
DEEPSEEK_API_KEY=...
PERPLEXITY_API_KEY=...
GROQ_API_KEY=...

# For Google Search tool
GOOGLE_API_KEY=...
GOOGLE_CX=...

# Local model endpoints (optional)
OLLAMA_BASE_URL=http://localhost:11434
LM_STUDIO_BASE_URL=http://localhost:1234
```

## 📖 Usage

### Interactive TUI Mode (Default)

```bash
# Start with default provider (OpenAI)
./simple-agent

# Use a specific provider and model
./simple-agent --provider anthropic --model claude-3-opus-20240229

# Available providers:
# - openai (default)
# - anthropic / claude
# - moonshot / kimi  
# - deepseek
# - perplexity
# - groq
# - ollama
# - lmstudio / lm-studio
```

### TUI Commands

- `/help` - Show available commands
- `/tools` - List available tools
- `/clear` - Clear conversation history (or Ctrl+L)
- `/exit` - Exit application (or Ctrl+C, Ctrl+Q, Esc)

### TUI Features

- **Transparent Input Box** - Clean bordered input that grows as you type
- **Natural Scrolling** - Conversation flows naturally, pushing input down
- **Responsive Design** - Adapts to terminal resizing without artifacts
- **Message Formatting** - Markdown rendering with syntax highlighting
- **Smart Text Wrapping** - Long messages wrap properly within terminal bounds

### One-Shot Query Mode

```bash
# Quick query without entering TUI
./simple-agent query "What is the capital of France?"

# With specific provider
./simple-agent --provider anthropic query "Explain quantum computing"
```

### List Available Tools

```bash
./simple-agent tools list
```

## 🛠️ Available Tools

1. **🧮 calculate** - Evaluate mathematical expressions safely
2. **📄 file_read** - Read contents of files
3. **💾 file_write** - Write content to files  
4. **📝 file_edit** - Edit files by replacing strings
5. **📁 directory_list** - List directory contents
6. **🖥️ shell** - Execute shell commands (with safeguards)
7. **📚 wikipedia** - Search Wikipedia articles
8. **🔍 google_search** - Search Google (requires API key)

## 🤖 Supported LLM Providers

| Provider | Models | Notes |
|----------|--------|-------|
| **OpenAI** | gpt-4-turbo-preview, gpt-3.5-turbo | Default provider |
| **Anthropic** | claude-3-opus, claude-3-sonnet | High quality responses |
| **Moonshot/Kimi** | moonshot-v1-8k | Chinese language support |
| **DeepSeek** | deepseek-chat | Code-focused model |
| **Perplexity** | llama-3.1-sonar-huge-128k-online | Web-aware responses |
| **Groq** | mixtral-8x7b-32768 | Fast inference |
| **Ollama** | Any local model | Requires Ollama running |
| **LM Studio** | Any local model | Requires LM Studio running |

## 🏗️ Architecture

Simple Agent Go uses a clean, modular architecture:

- **Agent Core** - Orchestrates conversations with ReAct prompting or function calling
- **LLM Clients** - Unified interface for all providers with streaming support
- **Tool System** - Struct tag-based schema generation (Go-idiomatic approach)
- **TUI Layer** - Bubble Tea components for beautiful terminal interface
- **CLI Layer** - Cobra commands for flexible usage patterns

## 🧑‍💻 Development

### Creating a New Tool

Tools are automatically discovered. Create a new file in `tools/`:

```go
package tools

type MyTool struct{}

func NewMyTool() *MyTool {
    return &MyTool{}
}

func (t *MyTool) Name() string { return "my_tool" }
func (t *MyTool) Description() string { return "Does something useful" }

func (t *MyTool) Execute(ctx context.Context, input string) (string, error) {
    var params struct {
        Query string `json:"query" schema:"required" description:"Search query"`
    }
    if err := json.Unmarshal([]byte(input), &params); err != nil {
        return "", err
    }
    
    // Tool implementation
    return "Result", nil
}

// Register in exports.go
func NewMyToolFunc() Tool { return NewMyTool() }
```

### Building

```bash
# Build binary
make build

# Run tests  
make test

# Install locally
make install
```

## 📚 Documentation

- [CLAUDE.md](CLAUDE.md) - Comprehensive project documentation
- [IMPLEMENTATION.md](IMPLEMENTATION.md) - Technical implementation details

## 🤝 Contributing

Contributions are welcome! The codebase is well-structured and documented. Key areas:

- Adding new LLM providers (see `llm/` directory)
- Creating new tools (see `tools/` directory)  
- Enhancing the TUI (see `tui/` directory)
- Improving agent capabilities (see `agent/` directory)

## 📄 License

MIT License - see LICENSE file for details

## 🙏 Acknowledgments

This project is inspired by the original Simple Agent implementations in Ruby and Python, bringing the same simplicity and power to the Go ecosystem with enhanced performance and user experience.
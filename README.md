# Simple Agent Go

A powerful AI agent framework for Go with multiple LLM providers and tool support.

## Quick Start

### 1. Install

```bash
go build ./cmd/simple-agent
```

### 2. Configure

Create a `.env` file with your API keys:

```bash
# OpenAI
OPENAI_API_KEY=sk-...

# Anthropic  
ANTHROPIC_API_KEY=sk-ant-...

# Other providers...
```

### 3. Run

**Interactive TUI (default):**
```bash
./simple-agent
```

**One-shot query:**
```bash
./simple-agent query "What is the capital of France?"
```

**With specific provider:**
```bash
./simple-agent --provider anthropic --model claude-3-opus-20240229
```

**List available tools:**
```bash
./simple-agent tools list
```

## Available Providers

- OpenAI (gpt-4, gpt-3.5-turbo)
- Anthropic (claude-3-opus, claude-3-sonnet, claude-3-haiku)
- Moonshot/Kimi (moonshot-v1-8k, moonshot-v1-32k)
- DeepSeek (deepseek-chat, deepseek-coder)
- Ollama (local models)
- LM Studio (local models)
- Perplexity (online models with web search)
- Groq (fast inference)

## Available Tools

- 🧮 **calculate** - Evaluate mathematical expressions
- 📁 **directory_list** - List directory contents
- 📝 **file_edit** - Edit files by replacing strings
- 📄 **file_read** - Read file contents
- 💾 **file_write** - Write content to files
- 🔍 **google_search** - Search Google (requires API key)
- 🖥️ **shell** - Execute shell commands
- 📚 **wikipedia** - Search Wikipedia

## Examples

**Using tools:**
```bash
./simple-agent query "Calculate the square root of 144"
./simple-agent query "Search Wikipedia for quantum computing"
./simple-agent query "List files in the current directory"
```

**Local models with Ollama:**
```bash
# First install Ollama and pull a model
ollama pull llama2

# Use it with simple-agent
./simple-agent --provider ollama --model llama2
```

## TUI Controls

- **Enter**: Send message
- **Ctrl+C**: Quit
- **Tab**: Switch panels
- **j/k**: Scroll messages
- **Ctrl+L**: Clear chat

## Commands in TUI

- `/help` - Show available commands
- `/tools` - List available tools
- `/model` - Change model
- `/clear` - Clear conversation
- `/save [name]` - Save conversation
- `/load` - Load conversation

## Documentation

See [CLAUDE.md](CLAUDE.md) for detailed documentation.
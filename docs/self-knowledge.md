# Self Knowledge

`simple-agent-go` now injects a self-knowledge section into the system prompt so the agent can reason about its own implementation.

## What it does

- Discovers the project root from:
  - `SIMPLE_AGENT_SOURCE_DIR` (if set)
  - current working directory and ancestors
  - executable-relative paths
- Adds documentation/source anchors to the prompt:
  - `README.md`
  - `docs/`
  - key implementation files (`cmd/simple-agent/main.go`, `agent/agent.go`, `tui/bordered.go`, etc.)
- Instructs the model to read and cite file paths for self-referential questions.

## Code

- Discovery: `internal/selfknowledge/selfknowledge.go`
- Prompt assembly: `internal/runtimeprompt/builder.go`

## Why

Without explicit doc paths, models tend to answer from generic prior knowledge. This section nudges the agent to use local source-of-truth files.

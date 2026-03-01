# Resource Loader

Runtime resources are loaded from disk and can be refreshed with `/reload`.

## Loaded resources

- Context files:
  - `~/.simple-agent/agent/AGENTS.md` or `~/.simple-agent/agent/CLAUDE.md`
  - nearest `AGENTS.md` or `CLAUDE.md` found across cwd ancestor chain (root -> cwd)
- Prompt fragments:
  - `~/.simple-agent/agent/prompts/*.md|*.txt`
  - `<cwd>/.simple-agent/prompts/*.md|*.txt`

## Reload behavior

`/reload` triggers:

1. context/prompt fragment reload
2. models registry reload
3. provider list refresh for model selector
4. system prompt rebuild + `agent.SetSystemPrompt(...)`

## Code

- Loader: `internal/resources/loader.go`
- Prompt build: `internal/runtimeprompt/builder.go`
- TUI command: `tui/bordered.go` (`/reload`)

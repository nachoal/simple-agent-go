# Auto Improve (`/improve`)

`/improve <goal>` runs a guarded self-improvement loop.

## Opt-in gate

Disabled by default. Enable with:

```bash
export SIMPLE_AGENT_ENABLE_IMPROVE=1
```

## Workflow

1. Build a restricted helper agent (`directory_list`, `read`, `edit`, `write` only).
2. Ask it to implement the goal with safety constraints.
3. Enforce changed-file safety threshold.
4. Run verification commands:
   - `go test ./...`
   - `go build -o ./simple-agent ./cmd/simple-agent`
   - `./simple-agent tools list`

## Guardrails

- No destructive git operations in prompt constraints.
- No `bash` tool in improve phase (file-edit tools only).
- Hard cap on changed files.
- Verification must pass for success.

## Code

- Runner: `internal/improve/runner.go`
- TUI command: `tui/bordered.go` (`/improve`)

# Architecture

This repository is split into four surfaces:

## 1. Product Runtime

- `cmd/simple-agent`
- `agent/`
- `history/`
- `llm/`
- `tools/`
- `tui/`
- public-safe internals such as `internal/resources`, `internal/runtimeprompt`, `internal/selfknowledge`, `internal/userpaths`

This is the shipped OSS product surface.

Operational notes:

- The agent applies a per-request LLM timeout from runtime config/CLI (`--timeout`) instead of relying on provider defaults.
- File mutation/read tools are workspace-scoped to the current working directory so repo-local runs do not escape into unrelated paths.

## 2. Public OSS Harness

- `go test ./...`
- `go build -o ./simple-agent ./cmd/simple-agent`
- `./simple-agent tools list`
- `./simple-agent --help`
- `scripts/run_public_evals`
- `.github/workflows/ci.yml`

This surface is safe to run in CI and safe to document publicly.

Public harness runs should emit the captured output of any failing check directly to stderr so CI failures are diagnosable from the Actions log.

## 3. Private Local Harness

- `scripts/analyze_codex_sessions`
- `scripts/run_harness`
- `internal/codexreport`
- local outputs under `~/.simple-agent/harness/<repo-slug>/`

This surface is for maintainers only and must not leak transcript-derived artifacts into the repository.

## 4. Documentation System Of Record

- `AGENTS.md`
- `docs/AGENTS.md`
- `docs/codex-analysis.md`
- `docs/harness-engineering-task-list.md`
- `docs/validation-matrix.md`
- `docs/runtime-state.md`

## Boundary Rules

- Product runtime packages must not depend on private maintainer analysis code.
- Core runtime packages must not depend on the TUI unless they are themselves in `tui/` or `cmd/`.
- Private transcript analysis outputs stay outside the repository by default.

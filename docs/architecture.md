# Architecture

This repository is split into five surfaces:

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
- Session resume/continue lives in the product runtime, with persisted sessions under `~/.simple-agent/sessions/`; resuming a session re-enters the saved workspace before the TUI starts.
- The bordered TUI now owns the visible transcript as viewport state instead of printing completed turns above the app, so historical messages and streamed assistant output can be reflowed and rerendered correctly on terminal resize.

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

The private harness manifest now includes a compact `summary` block so local research tooling can compare attempts without scraping human-readable console text.

## 4. Local Research Controller

- `research/program.md`
- `research/allowed_paths.txt`
- `research/import_bench_case.py`
- `research/run_bench_case.py`
- `research/evaluate.sh`
- `research/score.py`
- `research/loop.sh`
- ignored local artifacts under `research/runs/`, `research/results.tsv`, and `research/cases/`

This surface is a repo-local optimize/evaluate loop for Codex-driven improvement work. It may read sanitized harness manifests and write local diffs, scores, and controller logs, but it must not copy transcript-derived Codex-analysis artifacts out of `~/.simple-agent/harness/...`.

## 5. Documentation System Of Record

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

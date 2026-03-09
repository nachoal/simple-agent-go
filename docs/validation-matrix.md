# Validation Matrix

## Public OSS Validation

- `go test ./...`
- `go test ./agent -run Test(Query_RecoversMalformedToolCallFromContent|Query_UsesConfiguredRequestTimeout)$`
- `go test ./tools -run Test(WriteTool_BlocksPathsOutsideWorkspace|ReadTool_BlocksPathsOutsideWorkspace|EditTool_BlocksPathsOutsideWorkspace|DirectoryListTool_BlocksPathsOutsideWorkspace|WriteTool_UsesWorkspaceRelativePath)$`
- `go build -o ./simple-agent ./cmd/simple-agent`
- `./simple-agent tools list`
- `./simple-agent tools list --json`
- `./simple-agent models list --json`
- `./simple-agent doctor --json`
- `./simple-agent --help`
- `go run ./scripts/run_tui_smoke --binary ./simple-agent`

The PTY smoke now covers:

- cancel and rollback semantics
- `--continue` restoring the latest session globally
- `--resume <session-id>` restoring a specific session and its saved workspace
- `go run ./scripts/run_public_evals --json`

## Private Local Validation

- `go run ./scripts/run_harness`
- private Codex session analysis written to `~/.simple-agent/harness/<repo-slug>/codex-analysis/`
- private harness manifest written to `~/.simple-agent/harness/<repo-slug>/latest.json`
- structured JSONL run logs written to `~/.simple-agent/harness/<repo-slug>/runs/**/*.jsonl`
- optional live LM Studio canary when `SIMPLE_AGENT_ENABLE_LIVE_CANARIES=1`
- `research/evaluate.sh --mode both`
- `research/score.py <evaluation.json>`
- `research/import_bench_case.py <artifact-dir> --passing-result <result.json>`
- `research/run_bench_case.py <case.json>`
- `research/loop.sh --attempts <N>`

## Fast Path

- `go run ./scripts/run_harness --mode fast --skip-codex-analysis`

Use this when iterating locally and only a subset of Go packages changed.

## Full Public Path

- `go run ./scripts/run_harness --mode public`

Use this to mirror the public CI surface without private transcript analysis.
When a check fails, the harness prints the failing command and captured output to stderr so CI logs retain the root cause.

## Full Private Path

- `go run ./scripts/run_harness --mode private`

Use this when you want the full local maintainer harness, including private transcript analysis.

## Live Canary

- `SIMPLE_AGENT_ENABLE_LIVE_CANARIES=1 go run ./scripts/run_harness --mode private`
- optional model override: `LM_STUDIO_CANARY_MODEL=<model-id>`

## Research Loop

- `research/loop.sh` uses Codex CLI as the outer research agent.
- Imported benchmark cases under ignored `research/cases/` can add scenario-specific prompt context and a targeted rerun objective.
- The loop mutates only allowlisted paths from `research/allowed_paths.txt`.
- Accepted and discarded attempt summaries are logged locally under ignored `research/` artifacts, while transcript-derived Codex analysis remains under `~/.simple-agent/harness/...`.

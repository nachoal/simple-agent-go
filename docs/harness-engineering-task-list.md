# Harness Engineering Task List

This repo now follows a harness-oriented split: public OSS code and CI stay in the repository, while private transcript analysis and generated Codex retrospectives stay in local harness storage under `~/.simple-agent/harness/...`.

## Completed

- [x] Move Codex session analysis out of the `simple-agent` product CLI and into local maintainer scripts.
- [x] Keep private session-derived artifacts out of the repo by default.
- [x] Add a reusable Codex analysis package for local retrospectives and failure taxonomy extraction.
- [x] Add a local harness runner that executes build, tests, smoke checks, and private Codex analysis.
- [x] Add public-safe CI for build, test, and smoke verification.
- [x] Make harness failures print captured command output directly in CI logs.
- [x] Add public documentation that distinguishes OSS-safe artifacts from private local-only artifacts.
- [x] Add structured JSONL run logs for autonomous verification.
- [x] Add machine-readable CLI surfaces for tools, models, and runtime diagnostics.
- [x] Add public eval fixtures and a public eval runner.
- [x] Add an explicit fast/public/private harness split.
- [x] Add explicit per-session run status persistence.
- [x] Add architecture boundary tests.
- [x] Add a PTY-backed TUI smoke for cancel and continue flows.
- [x] Add an opt-in live LM Studio canary to the private harness.

## Public OSS Harness Surface

- `go test ./...`
- `go build -o ./simple-agent ./cmd/simple-agent`
- `go run ./scripts/run_public_evals --json`
- `./simple-agent tools list`
- `./simple-agent tools list --json`
- `./simple-agent models list --json`
- `./simple-agent doctor --json`
- `go run ./scripts/run_tui_smoke --binary ./simple-agent`
- `./simple-agent --help`
- `.github/workflows/ci.yml`
- `docs/runtime-state.md`
- `docs/harness-benchmark.md`
- `docs/codex-analysis.md`
- `docs/architecture.md`
- `docs/validation-matrix.md`

## Private Local Harness Surface

- `go run ./scripts/run_harness --mode fast`
- `go run ./scripts/run_harness --mode public`
- `go run ./scripts/run_harness --mode private`
- `SIMPLE_AGENT_ENABLE_LIVE_CANARIES=1 go run ./scripts/run_harness --mode private`
- `go run ./scripts/analyze_codex_sessions`
- `go run ./scripts/run_live_canary --binary ./simple-agent --provider lmstudio`
- `~/.simple-agent/harness/<repo-slug>/latest.json`
- `~/.simple-agent/harness/<repo-slug>/codex-analysis/`
- `~/.simple-agent/harness/<repo-slug>/runs/**/*.jsonl`

## Sanitized Scenario Families

These are the public scenario families the harness is meant to cover without leaking transcript contents:

1. Provider startup fallback
2. Malformed tool-call recovery
3. Hang and timeout detection
4. Cancel and continue semantics
5. PTY-level TUI survivability
6. Machine-readable CLI/state legibility
7. Verification and completion discipline

## Next Public Extensions

- Add richer PTY fixtures for session picker navigation and `/resume` list flows.
- Add a dedicated harness report viewer that summarizes regressions from `latest.json` without reading raw JSON.

# Harness Benchmark

Measured on March 7, 2026 on the local development machine in this repository.

## Baseline Before Harness Expansion

These were the original shallow validation fragments:

- `go test ./...` -> `0.33s`
- `go build -o ./simple-agent ./cmd/simple-agent` -> `0.23s`
- `./simple-agent tools list` -> `0.01s`

Approximate total shallow validation time: `0.57s`

What that baseline did **not** provide:

- structured JSONL run traces
- machine-readable model/runtime diagnostics
- explicit public eval fixtures
- fast/public/private harness modes
- private transcript analysis separation
- explicit run-status persistence

## Final Harness Measurements

- `go run ./scripts/run_public_evals --json` -> `14.24s`
- `go run ./scripts/run_harness --mode fast` -> `1.67s`
- `go run ./scripts/run_harness --mode public` -> `14.08s`
- `SIMPLE_AGENT_ENABLE_LIVE_CANARIES=1 go run ./scripts/run_harness --mode private` -> `29.11s`

## Interpretation

- The public eval layer is now much deeper because it includes a real PTY-backed TUI smoke, not only unit-style checks.
- The fast harness still stays under two seconds by using a reduced eval set plus changed-package testing.
- The full public harness is now intentionally slower because it exercises PTY TUI behavior and richer public eval coverage.
- The private harness is slower again because it adds local transcript mining, and optionally a live LM Studio canary.

## Net Effect

The migration improves autonomy more than raw single-command speed:

- Codex can now inspect tools, models, and runtime state without scraping prose.
- Codex can run explicit public eval fixtures instead of relying on generic smoke checks.
- The harness now stores structured local artifacts and compares runs over time.
- Private session-derived artifacts stay outside the repository by default.
- There is now a deterministic harness-only fake LLM for no-network PTY smoke and an opt-in live LM Studio canary for real-provider validation.
- `latest.json` now includes a compact summary block (`passed_checks`, `failed_checks`, `score_pct`, `total_duration_ms`) so repo-local research loops can score attempts mechanically.

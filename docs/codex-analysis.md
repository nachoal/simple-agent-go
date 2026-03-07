# Codex Session Analysis

`go run ./scripts/analyze_codex_sessions` mines local Codex rollout history and writes a repo-local retrospective.

## Inputs

- Codex sessions from `$CODEX_HOME` or `~/.codex`
- `sessions/` and `archived_sessions/` rollout files (`.jsonl` and legacy `.json`)
- The repository root passed with `--repo` or the current working directory

## Outputs

By default the script writes to a private local harness directory under `~/.simple-agent/harness/<repo-slug>/codex-analysis/`, not into the repository:

- `summary.md`
- `action-plan.md`
- `report.json`
- `relevant-sessions.jsonl`

## What It Extracts

- Common themes across relevant sessions
- Repeated user petitions
- Failure signals such as hangs, panics, malformed tool calls, and missing observability
- "Loops that should have continued" when the user had to push the agent back into unfinished work

## Relevance Filter

The analyzer does not treat `cwd == repo` as sufficient by itself.

It prefers sessions with stronger repo evidence such as:

- repo-scoped commands
- explicit repo path mentions
- direct references to repo files or binaries

Weaker matches are downgraded to `secondary`, `path_only`, or `cross_project` so the generated report is less noisy.

This keeps transcript-derived artifacts out of the OSS repo by default.

Malformed legacy files are skipped instead of failing the whole analysis run.

## Usage

```bash
go run ./scripts/analyze_codex_sessions
```

Override the defaults if needed:

```bash
go run ./scripts/analyze_codex_sessions \
  --repo /path/to/repo \
  --codex-home /path/to/.codex \
  --out-dir /path/to/reports
```

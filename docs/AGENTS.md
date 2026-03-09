# Documentation Working Agreement

This file applies when editing anything under `docs/` or public-facing maintainer guidance.

## 1. Public Documentation Only

- Assume `docs/` is public.
- Do not paste or summarize private session transcripts into public docs.
- Do not record repo weaknesses in a way that exposes private historical conversations.
- It is acceptable to document sanitized scenario families such as "hangs", "malformed tool calls", or "cancel/continue semantics".

## 2. Harness Documentation Rules

- Public docs may describe the public OSS harness surface:
  - `go test ./...`
  - `go build -o ./simple-agent ./cmd/simple-agent`
  - `./simple-agent tools list`
  - `./simple-agent --help`
  - `.github/workflows/ci.yml`
- Private transcript analysis must be documented as local-only and outside the repo by default.
- Repo-local ignored research artifacts under `research/` may be documented as local-only maintainer workflow surfaces, including imported benchmark case packs, but they must remain sanitized and must not embed private transcript-derived outputs from `~/.simple-agent/harness/...`.
- If defaults change, update the exact command examples and exact output location.

## 3. Required Sync Points

When the harness or maintainer workflow changes, review and update as needed:

- `README.md`
- `AGENTS.md`
- `docs/architecture.md`
- `docs/validation-matrix.md`
- `docs/runtime-state.md`
- `docs/harness-benchmark.md`
- `docs/codex-analysis.md`
- `docs/harness-engineering-task-list.md`

## 4. Style

- Prefer concrete commands and paths over abstract descriptions.
- Separate public-safe behavior from private maintainer-only behavior explicitly.
- Keep docs concise and operational.

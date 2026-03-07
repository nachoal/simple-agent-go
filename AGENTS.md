# Agent Working Agreement

This file defines mandatory execution hygiene for code changes in this repository.

## 1. Public vs Private Harness Boundary

- Treat this repository as public OSS.
- Do not commit or leave behind transcript-derived artifacts from local Codex or Claude sessions.
- Private harness outputs must stay under `~/.simple-agent/harness/...`, not under the repo.
- Public docs may describe scenario families, failure classes, and harness workflows, but must not expose raw private session transcripts, prompts, or internal issue narratives.

## 2. No Stale Artifact Assumptions

- Do not assume source changes are active in previously built binaries.
- If a user-facing behavior can be affected by code edits, verify the runnable artifact is rebuilt.

## 3. Required Post-Change Verification

After any behavior-changing edit:

1. Rebuild the executable used by the user.
2. Run relevant automated checks (`go test ./...` minimum unless scoped alternative is justified).
3. Run a smoke test using the same binary path the user is expected to run.
4. If the change touches harness, verification, or maintainer workflow code, run the relevant harness entrypoint too.
5. Report the exact artifact path and modification time in the final update.

Harness failures must also print the captured failing command output so CI logs are actionable without opening local harness artifacts.

Example public verification:

```bash
go build -o ./simple-agent ./cmd/simple-agent
stat -f "%Sm %N" ./simple-agent
go test ./...
./simple-agent tools list
```

Example private maintainer verification:

```bash
go run ./scripts/run_harness
```

## 4. Documentation Sync Is Mandatory

When changing behavior, also update the docs that define or explain that behavior.

At minimum, review:

- `README.md`
- `AGENTS.md`
- `docs/AGENTS.md` when changing docs or harness workflow
- `docs/architecture.md`
- `docs/validation-matrix.md`
- `docs/runtime-state.md`
- `docs/harness-benchmark.md` when changing validation cost or harness speed
- `docs/codex-analysis.md` when changing private session analysis behavior
- `docs/harness-engineering-task-list.md` when changing the harness architecture or maintainer workflow

Do not leave public docs describing a repo-local artifact path if the real output is private and local-only.

## 5. Response Contract

When delivering results after code changes, always include:

- What was changed.
- What was rebuilt (exact path).
- What was tested (exact commands).
- Whether private harness verification was run.
- Any remaining risks or unverified paths.

## 6. If Rebuild Is Not Needed

If no runnable artifact is affected, explicitly state why rebuild was skipped.

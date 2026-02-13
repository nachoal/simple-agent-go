# Agent Working Agreement

This file defines mandatory execution hygiene for code changes in this repository.

## 1. No Stale Artifact Assumptions

- Do not assume source changes are active in previously built binaries.
- If a user-facing behavior can be affected by code edits, verify the runnable artifact is rebuilt.

## 2. Required Post-Change Verification

After any behavior-changing edit:

1. Rebuild the executable used by the user.
2. Run relevant automated checks (`go test ./...` minimum unless scoped alternative is justified).
3. Run a smoke test using the same binary path the user is expected to run.
4. Report the exact artifact path and modification time in the final update.

Example (for this repo):

```bash
go build -o ./simple-agent ./cmd/simple-agent
stat -f "%Sm %N" ./simple-agent
go test ./...
```

## 3. Response Contract

When delivering results after code changes, always include:

- What was changed.
- What was rebuilt (exact path).
- What was tested (exact commands).
- Any remaining risks or unverified paths.

## 4. If Rebuild Is Not Needed

If no runnable artifact is affected, explicitly state why rebuild was skipped.

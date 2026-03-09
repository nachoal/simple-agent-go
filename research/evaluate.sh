#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR=""
MODE="both"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --out-dir)
      OUT_DIR="$2"
      shift 2
      ;;
    --mode)
      MODE="$2"
      shift 2
      ;;
    *)
      echo "Unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

if [[ -z "$OUT_DIR" ]]; then
  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  OUT_DIR="$ROOT_DIR/research/runs/manual_${stamp}"
fi

mkdir -p "$OUT_DIR"

compute_harness_latest() {
  python3 - "$ROOT_DIR" <<'PY'
import os
import sys

repo = os.path.abspath(sys.argv[1])
norm = os.path.normpath(repo)
h = 0x811C9DC5
for byte in norm.encode():
    h ^= byte
    h = (h * 0x01000193) & 0xFFFFFFFF
base = os.path.basename(norm).lower().replace(" ", "-").replace(os.sep, "-") or "repo"
print(os.path.join(os.path.expanduser("~"), ".simple-agent", "harness", f"{base}-{h:08x}", "latest.json"))
PY
}

write_harness_snapshot() {
  local mode_name="$1"
  local exit_code="$2"
  local log_path="$3"
  local latest_path
  latest_path="$(compute_harness_latest)"

  python3 - "$latest_path" "$mode_name" "$exit_code" "$log_path" <<'PY' > "$OUT_DIR/harness_${mode_name}.json"
import json
import os
import sys

latest_path, mode_name, exit_code, log_path = sys.argv[1:]
exit_code = int(exit_code)

payload = {
    "mode": mode_name,
    "command": f"go run ./scripts/run_harness --mode {mode_name}",
    "status": "passed" if exit_code == 0 else "failed",
    "exit_code": exit_code,
    "log_path": log_path,
    "harness_latest_path": latest_path,
    "summary": {
        "status": "failed" if exit_code else "passed",
        "total_checks": 0,
        "passed_checks": 0,
        "failed_checks": 0,
        "score_pct": 0.0,
        "total_duration_ms": 0,
        "failed_check_names": [],
    },
    "checks": [],
}

if os.path.exists(latest_path):
    try:
        with open(latest_path, "r", encoding="utf-8") as fh:
            latest = json.load(fh)
        if latest.get("mode") == mode_name:
            payload["summary"] = latest.get("summary", payload["summary"])
            payload["checks"] = latest.get("checks", [])
    except Exception as exc:
        payload["read_error"] = str(exc)
else:
    payload["read_error"] = f"missing harness manifest at {latest_path}"

json.dump(payload, sys.stdout, indent=2)
sys.stdout.write("\n")
PY
}

run_harness_mode() {
  local mode_name="$1"
  local log_path="$OUT_DIR/harness_${mode_name}.log"
  local exit_code=0

  pushd "$ROOT_DIR" >/dev/null
  if ! go run ./scripts/run_harness --mode "$mode_name" >"$log_path" 2>&1; then
    exit_code=$?
  fi
  popd >/dev/null

  write_harness_snapshot "$mode_name" "$exit_code" "$log_path"
}

run_extra_eval() {
  if [[ -z "${RESEARCH_EXTRA_EVAL_CMD:-}" ]]; then
    return
  fi

  local stdout_path="$OUT_DIR/extra_eval.stdout.log"
  local stderr_path="$OUT_DIR/extra_eval.stderr.log"
  local exit_code=0

  pushd "$ROOT_DIR" >/dev/null
  if ! bash -lc "$RESEARCH_EXTRA_EVAL_CMD" >"$stdout_path" 2>"$stderr_path"; then
    exit_code=$?
  fi
  popd >/dev/null

  python3 - "$stdout_path" "$stderr_path" "$exit_code" "${RESEARCH_EXTRA_EVAL_NAME:-extra}" <<'PY' > "$OUT_DIR/extra_eval.json"
import json
import os
import re
import sys

stdout_path, stderr_path, exit_code, name = sys.argv[1:]
exit_code = int(exit_code)

stdout = ""
stderr = ""
if os.path.exists(stdout_path):
    stdout = open(stdout_path, "r", encoding="utf-8").read()
if os.path.exists(stderr_path):
    stderr = open(stderr_path, "r", encoding="utf-8").read()

score = None
details = None
try:
    parsed = json.loads(stdout) if stdout.strip() else None
    if isinstance(parsed, dict):
        details = parsed
        raw = parsed.get("score_pct")
        if isinstance(raw, (int, float)):
            score = float(raw)
except Exception:
    pass

if score is None:
    match = re.search(r"-?\d+(?:\.\d+)?", stdout)
    if match:
        score = float(match.group(0))

payload = {
    "name": name,
    "command": os.environ.get("RESEARCH_EXTRA_EVAL_CMD", ""),
    "status": "passed" if exit_code == 0 else "failed",
    "exit_code": exit_code,
    "score_pct": score,
    "stdout_path": stdout_path,
    "stderr_path": stderr_path,
}
if details is not None:
    payload["details"] = details

json.dump(payload, sys.stdout, indent=2)
sys.stdout.write("\n")
PY
}

case "$MODE" in
  fast)
    run_harness_mode fast
    ;;
  public)
    run_harness_mode public
    ;;
  both)
    run_harness_mode fast
    run_harness_mode public
    ;;
  *)
    echo "Invalid mode: $MODE (expected fast, public, or both)" >&2
    exit 1
    ;;
esac

run_extra_eval

python3 - "$ROOT_DIR" "$OUT_DIR" "$MODE" <<'PY' > "$OUT_DIR/evaluation.json"
import json
import os
import sys
from datetime import datetime, timezone

repo_root, out_dir, mode = sys.argv[1:]

def load_json(path):
    if not os.path.exists(path):
        return None
    with open(path, "r", encoding="utf-8") as fh:
        return json.load(fh)

payload = {
    "generated_at": datetime.now(timezone.utc).isoformat(),
    "repo_root": repo_root,
    "mode": mode,
    "out_dir": out_dir,
    "fast": load_json(os.path.join(out_dir, "harness_fast.json")),
    "public": load_json(os.path.join(out_dir, "harness_public.json")),
    "extra": load_json(os.path.join(out_dir, "extra_eval.json")),
}

json.dump(payload, sys.stdout, indent=2)
sys.stdout.write("\n")
PY

echo "$OUT_DIR/evaluation.json"

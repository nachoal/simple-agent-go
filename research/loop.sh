#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROGRAM_FILE="$ROOT_DIR/research/program.md"
ALLOWED_FILE="$ROOT_DIR/research/allowed_paths.txt"
RUNS_DIR="$ROOT_DIR/research/runs"
RESULTS_FILE="$ROOT_DIR/research/results.tsv"

ATTEMPTS=1
EVAL_MODE="${RESEARCH_EVAL_MODE:-both}"
GOAL="${RESEARCH_GOAL:-}"
CODEX_PROFILE="${RESEARCH_CODEX_PROFILE:-}"
CODEX_MODEL="${RESEARCH_CODEX_MODEL:-}"
CASE_INPUT="${RESEARCH_CASE:-}"
CASE_JSON=""
CASE_CONTEXT_MD=""
CASE_EXTRA_ALLOWED=""
ACTIVE_ALLOWED_FILE="$ALLOWED_FILE"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --attempts)
      ATTEMPTS="$2"
      shift 2
      ;;
    --goal)
      GOAL="$2"
      shift 2
      ;;
    --eval-mode)
      EVAL_MODE="$2"
      shift 2
      ;;
    --codex-profile)
      CODEX_PROFILE="$2"
      shift 2
      ;;
    --codex-model)
      CODEX_MODEL="$2"
      shift 2
      ;;
    --case)
      CASE_INPUT="$2"
      shift 2
      ;;
    *)
      echo "Unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

mkdir -p "$RUNS_DIR"

resolve_case_paths() {
  if [[ -z "$CASE_INPUT" ]]; then
    return
  fi

  local candidate="$CASE_INPUT"
  if [[ -d "$candidate" ]]; then
    CASE_JSON="$(cd "$candidate" && pwd)/case.json"
  else
    CASE_JSON="$(cd "$(dirname "$candidate")" && pwd)/$(basename "$candidate")"
  fi

  if [[ ! -f "$CASE_JSON" ]]; then
    echo "Case file not found: $CASE_JSON" >&2
    exit 1
  fi

  local case_dir
  case_dir="$(cd "$(dirname "$CASE_JSON")" && pwd)"
  CASE_CONTEXT_MD="$case_dir/prompt_context.md"
  CASE_EXTRA_ALLOWED="$case_dir/allowed_paths_extra.txt"
}

build_active_allowed_file() {
  ACTIVE_ALLOWED_FILE="$(mktemp "$RUNS_DIR/allowed_paths.XXXXXX")"
  cat "$ALLOWED_FILE" >"$ACTIVE_ALLOWED_FILE"
  if [[ -n "$CASE_EXTRA_ALLOWED" && -f "$CASE_EXTRA_ALLOWED" ]]; then
    cat "$CASE_EXTRA_ALLOWED" >>"$ACTIVE_ALLOWED_FILE"
  fi
  python3 - "$ACTIVE_ALLOWED_FILE" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
entries = []
seen = set()
for line in path.read_text().splitlines():
    line = line.strip()
    if not line or line in seen:
        continue
    seen.add(line)
    entries.append(line)
path.write_text("".join(f"{entry}\n" for entry in entries))
PY
}

case_json_field() {
  python3 - "$CASE_JSON" "$1" <<'PY'
import json
import sys

case_path, field = sys.argv[1:]
payload = json.loads(open(case_path, "r", encoding="utf-8").read())
value = payload
for part in field.split("."):
    if not isinstance(value, dict):
        value = None
        break
    value = value.get(part)
if value is None:
    print("")
elif isinstance(value, (dict, list)):
    print(json.dumps(value))
else:
    print(value)
PY
}

ensure_results_file() {
  if [[ ! -f "$RESULTS_FILE" ]]; then
    cat >"$RESULTS_FILE" <<'EOF'
timestamp	attempt	commit	status	overall_score	public_score	fast_score	extra_score	description
EOF
  fi
}

append_result() {
  local timestamp="$1"
  local attempt="$2"
  local commit="$3"
  local status="$4"
  local overall="$5"
  local public="$6"
  local fast="$7"
  local extra="$8"
  local description="$9"
  printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n" \
    "$timestamp" "$attempt" "$commit" "$status" "$overall" "$public" "$fast" "$extra" "$description" >>"$RESULTS_FILE"
}

ensure_clean_tree() {
  if [[ -n "$(git status --porcelain)" ]]; then
    echo "Working tree must be clean before starting research loop." >&2
    exit 1
  fi

  local branch
  branch="$(git branch --show-current)"
  if [[ "$branch" == "main" || "$branch" == "master" ]]; then
    echo "Run this loop from a dedicated research branch, not $branch." >&2
    exit 1
  fi
}

collect_changed_files() {
  {
    git diff --name-only
    git diff --cached --name-only
    git ls-files --others --exclude-standard
  } | awk 'NF { print }' | sort -u
}

path_is_allowed() {
  local path="$1"
  while IFS= read -r allowed || [[ -n "$allowed" ]]; do
    [[ -z "$allowed" ]] && continue
    if [[ "$path" == "$allowed" || "$path" == "$allowed"* ]]; then
      return 0
    fi
  done < "$ACTIVE_ALLOWED_FILE"
  return 1
}

restore_paths() {
  local source_commit="$1"
  shift
  if [[ "$#" -eq 0 ]]; then
    return
  fi

  git restore --source "$source_commit" --staged --worktree -- "$@" >/dev/null 2>&1 || true
  git clean -fd -- "$@" >/dev/null 2>&1 || true
}

json_field() {
  python3 - "$1" "$2" <<'PY'
import json
import sys

path, field = sys.argv[1:]
with open(path, "r", encoding="utf-8") as fh:
    payload = json.load(fh)
value = payload.get(field)
if value is None:
    print("")
else:
    print(value)
PY
}

write_score_file() {
  local evaluation_path="$1"
  local output_path="$2"
  python3 "$ROOT_DIR/research/score.py" --json "$evaluation_path" >"$output_path"
}

write_attempt_prompt() {
  local prompt_path="$1"
  local best_score_path="$2"
  local attempt="$3"

  {
    cat "$PROGRAM_FILE"
    echo
    echo "## Controller Context"
    echo
    echo "- Attempt: $attempt / $ATTEMPTS"
    echo "- Current branch: $(git branch --show-current)"
    echo "- Current best commit: $(git rev-parse --short HEAD)"
    echo "- Allowed path prefixes:"
    sed 's/^/  - /' "$ACTIVE_ALLOWED_FILE"
    if [[ -n "$GOAL" ]]; then
      echo "- Additional focus: $GOAL"
    fi
    if [[ -n "$CASE_JSON" ]]; then
      echo "- Imported case: $(case_json_field title)"
    fi
    echo
    echo "Current best score snapshot:"
    echo '```json'
    cat "$best_score_path"
    echo '```'
    if [[ -n "$CASE_CONTEXT_MD" && -f "$CASE_CONTEXT_MD" ]]; then
      echo
      echo "## Imported Benchmark Case"
      echo
      cat "$CASE_CONTEXT_MD"
    fi
    echo
    echo "Make one focused patch and then stop."
    echo "Do not run git commit or the full harness."
  } >"$prompt_path"
}

run_baseline() {
  local run_dir="$1"
  local eval_path
  eval_path="$("$ROOT_DIR/research/evaluate.sh" --mode "$EVAL_MODE" --out-dir "$run_dir")"
  echo "$eval_path"
}

ensure_results_file
resolve_case_paths
build_active_allowed_file
ensure_clean_tree

if [[ -n "$CASE_JSON" ]]; then
  export RESEARCH_EXTRA_EVAL_CMD="python3 \"$ROOT_DIR/research/run_bench_case.py\" \"$CASE_JSON\""
  export RESEARCH_EXTRA_EVAL_NAME="bench-case"
fi

baseline_dir="$RUNS_DIR/$(date -u +%Y%m%dT%H%M%SZ)_baseline"
mkdir -p "$baseline_dir"
best_eval="$(run_baseline "$baseline_dir")"
best_commit="$(git rev-parse HEAD)"
best_score_path="$baseline_dir/score.json"
write_score_file "$best_eval" "$best_score_path"

append_result \
  "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  "baseline" \
  "$(git rev-parse --short "$best_commit")" \
  "baseline" \
  "$(json_field "$best_score_path" overall_score)" \
  "$(json_field "$best_score_path" public_score)" \
  "$(json_field "$best_score_path" fast_score)" \
  "$(json_field "$best_score_path" extra_score)" \
  "baseline"

for (( attempt=1; attempt<=ATTEMPTS; attempt++ )); do
  run_dir="$RUNS_DIR/$(date -u +%Y%m%dT%H%M%SZ)_attempt_$(printf %02d "$attempt")"
  mkdir -p "$run_dir"
  prompt_path="$run_dir/prompt.md"
  message_path="$run_dir/last_message.txt"
  codex_log="$run_dir/codex_exec.log"

  write_attempt_prompt "$prompt_path" "$best_score_path" "$attempt"

  codex_args=(exec --full-auto --sandbox workspace-write -C "$ROOT_DIR" -o "$message_path")
  if [[ -n "$CODEX_PROFILE" ]]; then
    codex_args+=(-p "$CODEX_PROFILE")
  fi
  if [[ -n "$CODEX_MODEL" ]]; then
    codex_args+=(-m "$CODEX_MODEL")
  fi
  codex_args+=(-)

  codex_exit=0
  if ! codex "${codex_args[@]}" <"$prompt_path" >"$codex_log" 2>&1; then
    codex_exit=$?
  fi

  mapfile -t changed_files < <(collect_changed_files)

  if [[ "$codex_exit" -ne 0 ]]; then
    if [[ "${#changed_files[@]}" -gt 0 ]]; then
      git diff --binary >"$run_dir/rejected.diff" || true
      restore_paths "$best_commit" "${changed_files[@]}"
    fi
    append_result "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$attempt" "$(git rev-parse --short "$best_commit")" "crash" "0" "0" "0" "" "codex exec failed"
    continue
  fi

  if [[ "${#changed_files[@]}" -eq 0 ]]; then
    append_result "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$attempt" "$(git rev-parse --short "$best_commit")" "nochange" "0" "0" "0" "" "no code changes"
    continue
  fi

  disallowed=()
  for path in "${changed_files[@]}"; do
    if ! path_is_allowed "$path"; then
      disallowed+=("$path")
    fi
  done

  if [[ "${#disallowed[@]}" -gt 0 ]]; then
    git diff --binary >"$run_dir/rejected.diff" || true
    restore_paths "$best_commit" "${changed_files[@]}"
    append_result "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$attempt" "$(git rev-parse --short "$best_commit")" "rejected" "0" "0" "0" "" "disallowed paths: ${disallowed[*]}"
    continue
  fi

  git diff --binary >"$run_dir/candidate.diff" || true
  candidate_eval="$("$ROOT_DIR/research/evaluate.sh" --mode "$EVAL_MODE" --out-dir "$run_dir")"
  candidate_score_path="$run_dir/score.json"
  write_score_file "$candidate_eval" "$candidate_score_path"

  if python3 "$ROOT_DIR/research/score.py" --compare "$best_eval" "$candidate_eval" >/dev/null; then
    summary_line=""
    if [[ -f "$message_path" ]]; then
      summary_line="$(head -n 1 "$message_path" | tr '\t\r' ' ' | cut -c1-120)"
    fi
    if [[ -z "$summary_line" ]]; then
      summary_line="accepted attempt ${attempt}"
    fi

    git add -- "${changed_files[@]}"
    git commit -m "research: ${summary_line}" >/dev/null
    best_commit="$(git rev-parse HEAD)"
    best_eval="$candidate_eval"
    best_score_path="$candidate_score_path"

    append_result \
      "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
      "$attempt" \
      "$(git rev-parse --short "$best_commit")" \
      "keep" \
      "$(json_field "$candidate_score_path" overall_score)" \
      "$(json_field "$candidate_score_path" public_score)" \
      "$(json_field "$candidate_score_path" fast_score)" \
      "$(json_field "$candidate_score_path" extra_score)" \
      "$summary_line"
  else
    restore_paths "$best_commit" "${changed_files[@]}"
    append_result \
      "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
      "$attempt" \
      "$(git rev-parse --short "$best_commit")" \
      "discard" \
      "$(json_field "$candidate_score_path" overall_score)" \
      "$(json_field "$candidate_score_path" public_score)" \
      "$(json_field "$candidate_score_path" fast_score)" \
      "$(json_field "$candidate_score_path" extra_score)" \
      "no improvement"
  fi
done

echo "Research loop finished."
echo "Best commit: $(git rev-parse --short "$best_commit")"
echo "Best score:"
cat "$best_score_path"

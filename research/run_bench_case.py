#!/usr/bin/env python3
import argparse
import json
import subprocess
import sys
from pathlib import Path


def result_filename(model: str, harness: str, task_name: str) -> str:
    return f"{model}_{harness}_{task_name}.json"


def main() -> int:
    parser = argparse.ArgumentParser(description="Run the benchmark slice associated with a research case.")
    parser.add_argument("case", help="Path to case.json or case directory")
    args = parser.parse_args()

    case_path = Path(args.case).resolve()
    if case_path.is_dir():
        case_path = case_path / "case.json"
    case = json.loads(case_path.read_text())
    bench = case["benchmark"]

    bench_root = Path(bench["repo_root"]).resolve()
    script = bench["script"]
    results_dir = bench_root / bench["results_dir"]
    result_path = results_dir / result_filename(bench["model"], bench["harness"], bench["task_name"])

    cmd = [
        "uv",
        "run",
        script,
        "--model",
        bench["model"],
        "--task",
        str(bench["task_id"]),
        "--harness",
        bench["harness"],
        "--profile",
        bench.get("profile", "all"),
        "--force",
    ]

    proc = subprocess.run(
        cmd,
        cwd=str(bench_root),
        capture_output=True,
        text=True,
    )

    payload = {
        "name": "bench-case",
        "command": " ".join(cmd),
        "status": "passed" if proc.returncode == 0 else "failed",
        "exit_code": proc.returncode,
        "result_file": str(result_path),
        "stdout": proc.stdout[-4000:],
        "stderr": proc.stderr[-4000:],
    }

    if result_path.exists():
        result = json.loads(result_path.read_text())
        payload["details"] = result
        max_score = result.get("max_score", 0) or 0
        score = result.get("score", 0) or 0
        payload["score_pct"] = float(score) / float(max_score) * 100 if max_score else 0.0
        payload["passed"] = result.get("passed", False)
        payload["timed_out"] = result.get("timed_out", False)
    else:
        payload["score_pct"] = 0.0

    print(json.dumps(payload, indent=2))
    return proc.returncode


if __name__ == "__main__":
    raise SystemExit(main())

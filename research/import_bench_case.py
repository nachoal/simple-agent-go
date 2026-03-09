#!/usr/bin/env python3
import argparse
import json
import os
import shutil
from datetime import datetime, timezone
from pathlib import Path
DEFAULT_EXTRA_ALLOWED = [
    "agent/",
    "cmd/",
    "history/",
    "internal/",
    "llm/",
    "tools/",
    "tui/",
    "evals/",
    "scripts/run_harness/",
    "scripts/run_public_evals/",
    "scripts/run_tui_smoke/",
]


def slugify(text: str) -> str:
    out = []
    prev_dash = False
    for ch in text.lower():
        if ch.isalnum():
            out.append(ch)
            prev_dash = False
        else:
            if not prev_dash:
                out.append("-")
                prev_dash = True
    slug = "".join(out).strip("-")
    return slug or "case"


def determine_focus(failure: dict) -> str:
    if failure.get("timed_out"):
        return "timeout-cluster"
    if not failure.get("passed") and failure.get("score", 0) == 0:
        return "wrong-output"
    return "benchmark-gap"


def infer_benchmark_layout(artifact_dir: Path, bench_root: Path) -> tuple[str, str]:
    platform = artifact_dir.parts[-2] if len(artifact_dir.parts) >= 2 else "mac"
    if platform == "ialab":
        return "bench_ialab.py", "results_ialab"
    return "bench.py", "results"


def build_prompt_context(case: dict) -> str:
    failure = case["failure"]
    lines = [
        f"# Case: {case['title']}",
        "",
        f"- Focus: `{case['focus']}`",
        f"- Benchmark source: `{case['benchmark']['script']}`",
        f"- Model: `{failure['model']}`",
        f"- Task: `{failure['task_name']}` (`{failure['task_id']}`)",
        f"- Failing harness: `{failure['harness']}`",
        f"- Failing score: `{failure['score']}/{failure['max_score']}`",
        f"- Timed out: `{failure['timed_out']}`",
        f"- Error: `{failure['error']}`",
    ]

    reference = case.get("passing_reference")
    if isinstance(reference, dict):
        lines.extend([
            f"- Passing comparison harness: `{reference['harness']}`",
            f"- Passing comparison score: `{reference['score']}/{reference['max_score']}`",
            f"- Passing comparison wall time: `{reference['wall_time']}`",
        ])

    lines.extend([
        "",
        "Use the imported benchmark case evidence in this directory:",
        "",
        "- `failure_artifact.json`",
        "- `workspace/`",
    ])
    if reference:
        lines.append("- `passing_result.json`")
    lines.extend([
        "",
        "Primary question:",
        case["summary"],
    ])
    return "\n".join(lines) + "\n"


def copy_into(src: Path, dst: Path) -> None:
    if src.is_dir():
        if dst.exists():
            shutil.rmtree(dst)
        shutil.copytree(src, dst)
        return
    dst.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(src, dst)


def main() -> int:
    parser = argparse.ArgumentParser(description="Import a benchmark failure into a local research case pack.")
    parser.add_argument("artifact_dir", help="Path to benchmark failure artifact directory")
    parser.add_argument("--passing-result", help="Optional passing comparison result JSON (for example pi)")
    parser.add_argument("--bench-root", help="Benchmark repo root (defaults to inferring from artifact_dir or $LLM_AGENTIC_BENCH_ROOT)")
    parser.add_argument("--out-dir", help="Destination case directory (defaults under research/cases/)")
    args = parser.parse_args()

    root = Path(__file__).resolve().parent.parent
    artifact_dir = Path(args.artifact_dir).resolve()
    if args.bench_root:
        bench_root = Path(args.bench_root).resolve()
    elif os.environ.get("LLM_AGENTIC_BENCH_ROOT"):
        bench_root = Path(os.environ["LLM_AGENTIC_BENCH_ROOT"]).resolve()
    else:
        bench_root = artifact_dir.parent.parent

    artifact_json = artifact_dir / "artifact.json"
    workspace_dir = artifact_dir / "workspace"
    if not artifact_json.exists():
        raise SystemExit(f"Missing artifact.json in {artifact_dir}")
    if not workspace_dir.exists():
        raise SystemExit(f"Missing workspace/ in {artifact_dir}")
    if not (bench_root / "bench.py").exists():
        raise SystemExit(f"Could not resolve benchmark repo root from {bench_root}; pass --bench-root explicitly")

    failure = json.loads(artifact_json.read_text())
    passing = None
    if args.passing_result:
        passing = json.loads(Path(args.passing_result).read_text())

    focus = determine_focus(failure)
    case_slug = slugify(f"{failure['model']}_{failure['task_name']}_{focus}")
    case_dir = Path(args.out_dir).resolve() if args.out_dir else root / "research" / "cases" / case_slug
    case_dir.mkdir(parents=True, exist_ok=True)

    script_name, results_dir = infer_benchmark_layout(artifact_dir, bench_root)
    summary = (
        f"`{failure['harness']}` scored {failure['score']}/{failure['max_score']}"
        f"{' and timed out' if failure.get('timed_out') else ''} on `{failure['task_name']}` "
        f"with `{failure['model']}`."
    )
    if passing:
        summary += (
            f" Compare against `{passing['harness']}`, which scored "
            f"{passing['score']}/{passing['max_score']} in {passing['wall_time']:.1f}s on the same task."
        )

    case_payload = {
        "title": f"{failure['model']} {failure['task_name']} {focus}",
        "created_at": datetime.now(timezone.utc).isoformat(),
        "focus": focus,
        "summary": summary,
        "benchmark": {
            "repo_root": str(bench_root),
            "script": script_name,
            "results_dir": results_dir,
            "model": failure["model"],
            "harness": failure["harness"],
            "task_id": failure["task_id"],
            "task_name": failure["task_name"],
            "profile": failure.get("profile", "all"),
        },
        "failure": failure,
        "passing_reference": passing,
        "allowed_paths_extra": DEFAULT_EXTRA_ALLOWED,
        "source_paths": {
            "failure_artifact_dir": str(artifact_dir),
            "passing_result": args.passing_result or "",
        },
    }

    (case_dir / "case.json").write_text(json.dumps(case_payload, indent=2) + "\n")
    (case_dir / "allowed_paths_extra.txt").write_text("".join(f"{entry}\n" for entry in DEFAULT_EXTRA_ALLOWED))
    (case_dir / "prompt_context.md").write_text(build_prompt_context(case_payload))

    copy_into(artifact_json, case_dir / "failure_artifact.json")
    copy_into(workspace_dir, case_dir / "workspace")
    if args.passing_result:
        copy_into(Path(args.passing_result), case_dir / "passing_result.json")

    print(case_dir)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

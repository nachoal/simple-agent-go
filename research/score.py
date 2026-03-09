#!/usr/bin/env python3
import argparse
import json
import math


def load(path: str) -> dict:
    with open(path, "r", encoding="utf-8") as fh:
        return json.load(fh)


def summary_score(component: dict | None) -> float:
    if not isinstance(component, dict):
        return 0.0
    summary = component.get("summary")
    if not isinstance(summary, dict):
        return 0.0
    value = summary.get("score_pct", 0.0)
    try:
        return float(value)
    except (TypeError, ValueError):
        return 0.0


def summary_failed(component: dict | None) -> int:
    if not isinstance(component, dict):
        return 0
    summary = component.get("summary")
    if not isinstance(summary, dict):
        return 0
    value = summary.get("failed_checks", 0)
    try:
        return int(value)
    except (TypeError, ValueError):
        return 0


def summary_duration(component: dict | None) -> int:
    if not isinstance(component, dict):
        return 0
    summary = component.get("summary")
    if not isinstance(summary, dict):
        return 0
    value = summary.get("total_duration_ms", 0)
    try:
        return int(value)
    except (TypeError, ValueError):
        return 0


def score_details(payload: dict) -> dict:
    fast = summary_score(payload.get("fast"))
    public_component = payload.get("public") or payload.get("fast")
    public = summary_score(public_component)

    extra_score = None
    extra = payload.get("extra")
    if isinstance(extra, dict):
        raw = extra.get("score_pct")
        if isinstance(raw, (int, float)):
            extra_score = float(raw)

    if extra_score is None:
        overall = 0.75 * public + 0.25 * fast
    else:
        overall = 0.60 * public + 0.20 * fast + 0.20 * extra_score

    details = {
        "overall_score": round(overall, 6),
        "public_score": round(public, 6),
        "fast_score": round(fast, 6),
        "extra_score": None if extra_score is None else round(extra_score, 6),
        "failed_checks": summary_failed(payload.get("fast")) + summary_failed(payload.get("public")),
        "total_duration_ms": summary_duration(payload.get("fast")) + summary_duration(payload.get("public")),
    }
    return details


def compare_payloads(old_payload: dict, new_payload: dict) -> tuple[bool, dict, dict]:
    old = score_details(old_payload)
    new = score_details(new_payload)

    keys = [
        ("overall_score", 1),
        ("public_score", 1),
        ("fast_score", 1),
        ("extra_score", 1),
        ("failed_checks", -1),
        ("total_duration_ms", -1),
    ]

    for key, direction in keys:
        old_value = old.get(key)
        new_value = new.get(key)

        if old_value is None and new_value is None:
            continue
        if old_value is None:
            return True, old, new
        if new_value is None:
            continue

        if isinstance(old_value, float) or isinstance(new_value, float):
            if math.isclose(float(old_value), float(new_value), rel_tol=1e-9, abs_tol=1e-9):
                continue
            if direction > 0:
                return float(new_value) > float(old_value), old, new
            return float(new_value) < float(old_value), old, new

        if old_value == new_value:
            continue
        if direction > 0:
            return new_value > old_value, old, new
        return new_value < old_value, old, new

    return False, old, new


def main() -> int:
    parser = argparse.ArgumentParser(description="Score or compare research evaluations.")
    parser.add_argument("evaluation", nargs="?", help="Path to evaluation.json")
    parser.add_argument("--json", action="store_true", help="Print structured score details")
    parser.add_argument("--compare", nargs=2, metavar=("OLD", "NEW"), help="Compare two evaluation.json files")
    args = parser.parse_args()

    if args.compare:
        old_payload = load(args.compare[0])
        new_payload = load(args.compare[1])
        improved, old_details, new_details = compare_payloads(old_payload, new_payload)
        payload = {
            "improved": improved,
            "old": old_details,
            "new": new_details,
        }
        if args.json:
            print(json.dumps(payload, indent=2))
        else:
            print("improved" if improved else "not-improved")
        return 0 if improved else 1

    if not args.evaluation:
        parser.error("evaluation path is required unless --compare is used")

    details = score_details(load(args.evaluation))
    if args.json:
        print(json.dumps(details, indent=2))
    else:
        print(f"{details['overall_score']:.6f}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

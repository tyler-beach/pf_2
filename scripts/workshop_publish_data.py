#!/usr/bin/env python3
"""Merge a workshop snapshot into the workshop-data layout (append-only metrics)."""

from __future__ import annotations

import argparse
import hashlib
import json
import shutil
from datetime import datetime, timezone
from pathlib import Path


def load_json(path: Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def write_json(path: Path, value: object) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--snapshot", required=True, type=Path)
    parser.add_argument("--data-dir", required=True, type=Path)
    parser.add_argument("--retain-snapshots", type=int, default=180)
    args = parser.parse_args()

    snapshot = args.snapshot
    data_dir = args.data_dir
    data_dir.mkdir(parents=True, exist_ok=True)

    latest_path = snapshot / "latest.json"
    metrics_path = snapshot / "metrics.json"
    run_path = snapshot / "run.json"
    if not latest_path.exists():
        raise SystemExit(f"missing {latest_path}")

    latest = load_json(latest_path)
    metrics = load_json(metrics_path) if metrics_path.exists() else latest.get("metrics", {})
    run = load_json(run_path) if run_path.exists() else latest.get("run", {})
    run_id = str(run.get("run_id") or latest.get("run", {}).get("run_id") or "unknown")

    dest = data_dir / "snapshots" / run_id
    if dest.exists():
        shutil.rmtree(dest)
    shutil.copytree(snapshot, dest)

    write_json(data_dir / "latest.json", latest)

    # Promote SPA docs that ship beside the snapshot so Sample/Pipeline stay available at data root.
    for name in ("sample-journey.json", "pipeline.json", "corpus.json"):
        src = dest / name
        if src.is_file():
            shutil.copy2(src, data_dir / name)

    row = {"run": run, "metrics": metrics}
    runs_path = data_dir / "metrics" / "runs.jsonl"
    runs_path.parent.mkdir(parents=True, exist_ok=True)
    # Upsert by run_id so republishes replace prior rows (avoids duplicate
    # pre-calib / wrong-KPI history for the same snapshot id).
    existing: list[dict] = []
    if runs_path.exists():
        for line in runs_path.read_text(encoding="utf-8").splitlines():
            if not line.strip():
                continue
            item = json.loads(line)
            if str(item.get("run", {}).get("run_id", "")) == run_id:
                continue
            existing.append(item)
    existing.append(row)
    runs_path.write_text(
        "".join(json.dumps(item, sort_keys=True) + "\n" for item in existing),
        encoding="utf-8",
    )

    # Daily rollups from full jsonl (rewrite touched days)
    days: set[str] = set()
    for item in existing:
        ts = str(item.get("run", {}).get("timestamp", ""))
        if len(ts) >= 10:
            days.add(ts[:10])
    day = (run.get("timestamp") or datetime.now(timezone.utc).isoformat())[:10]
    days.add(day)
    for d in sorted(days):
        daily = {
            "date": d,
            "runs": 0,
            "candidate_total": 0,
            "confidence_high": 0,
            "confidence_medium": 0,
            "confidence_low": 0,
        }
        for item in existing:
            ts = str(item.get("run", {}).get("timestamp", ""))
            if not ts.startswith(d):
                continue
            m = item.get("metrics", {})
            daily["runs"] += 1
            daily["candidate_total"] += int(m.get("candidate_total") or 0)
            daily["confidence_high"] += int(m.get("confidence_high") or 0)
            daily["confidence_medium"] += int(m.get("confidence_medium") or 0)
            daily["confidence_low"] += int(m.get("confidence_low") or 0)
        write_json(data_dir / "metrics" / "daily" / f"{d}.json", daily)

    # Retention
    snap_root = data_dir / "snapshots"
    if snap_root.exists():
        snaps = sorted([p for p in snap_root.iterdir() if p.is_dir()], key=lambda p: p.name)
        for old in snaps[: max(0, len(snaps) - args.retain_snapshots)]:
            shutil.rmtree(old)

    snapshots: list[dict[str, str]] = []
    if snap_root.exists():
        for p in sorted([x for x in snap_root.iterdir() if x.is_dir()], key=lambda x: x.name):
            if (p / "latest.json").is_file():
                snapshots.append({"run_id": p.name, "path": f"snapshots/{p.name}"})

    manifest = {
        "latest_run_id": run_id,
        "updated_at": datetime.now(timezone.utc).isoformat(),
        "data_sha": hashlib.sha256((data_dir / "latest.json").read_bytes()).hexdigest(),
        "snapshots": snapshots,
    }
    write_json(data_dir / "manifest.json", manifest)
    print(json.dumps(manifest, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

#!/usr/bin/env bash
# Emit one calibrated workshop snapshot per submission (unique runs), then merge
# into the workshop Pages data dir. Reuses testdata/workshop-submissions/results.
#
# Prefer refreshing assessments first with applicable-evidence + scanners:
#   ./scripts/assess_workshop_submissions.sh   # OpenGrep/Checkov when OPENGREP_BIN/CHECKOV_BIN set
# then:
#   scripts/emit_unique_workshop_runs.sh [--wipe] [--data-dir workshop/public/data]
#
# Usage:
#   scripts/emit_unique_workshop_runs.sh [--wipe] [--data-dir workshop/public/data]
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

PACK="${PACK:-testdata/workshop-submissions}"
OUT="${OUT:-$PACK/results}"
MANIFEST="$PACK/manifest.json"
REQ="${REQ:-testdata/corpus/requirements.json}"
DATA_DIR="${DATA_DIR:-workshop/public/data}"
WORKDIR="${WORKDIR:-/tmp/pf-unique-runs}"
WIPE=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --wipe) WIPE=1; shift ;;
    --data-dir) DATA_DIR="$2"; shift 2 ;;
    --pack) PACK="$2"; MANIFEST="$PACK/manifest.json"; OUT="$PACK/results"; shift 2 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

test -f "$MANIFEST"
test -d "$OUT"
GIT_SHA="$(git rev-parse HEAD)"
mkdir -p "$WORKDIR" "$DATA_DIR/metrics"

if [[ "$WIPE" -eq 1 ]]; then
  echo "==> wiping run history under $DATA_DIR (keeping shared SPA fixtures)"
  rm -rf "$DATA_DIR/snapshots"
  mkdir -p "$DATA_DIR/snapshots"
  mkdir -p "$DATA_DIR/metrics"
  : > "$DATA_DIR/metrics/runs.jsonl"
  # Drop root evidence that belonged to prior batch/fixture publishes.
  for f in latest.json candidates.json proposals.json assessment.json costs.json \
           promptfoo.json adr_flagging.json rule_hits.json submissions.json \
           metrics.json run.json; do
    rm -f "$DATA_DIR/$f"
  done
fi

export PACK OUT MANIFEST DATA_DIR WORKDIR GIT_SHA ROOT
python3 <<'PY'
import json, os, shutil, subprocess, tempfile
from pathlib import Path
from datetime import datetime, timezone

root = Path(os.environ.get("ROOT", "."))
pack = Path(os.environ["PACK"])
out = Path(os.environ["OUT"])
manifest = json.loads(Path(os.environ["MANIFEST"]).read_text())
data_dir = Path(os.environ["DATA_DIR"])
workdir = Path(os.environ["WORKDIR"])
git_sha = os.environ["GIT_SHA"]
workdir.mkdir(parents=True, exist_ok=True)

subs = manifest["submissions"]
print(f"emitting {len(subs)} unique runs…", flush=True)

def run(cmd, **kw):
    print("+", " ".join(cmd), flush=True)
    subprocess.run(cmd, check=True, **kw)

for i, sub in enumerate(subs, 1):
    sid = sub["id"]
    kind = sub["submission_kind"]
    run_id = sid
    workflow = f"Workshop · {kind} · {sid}"
    dest = workdir / sid
    if dest.exists():
        shutil.rmtree(dest)
    dest.mkdir(parents=True)
    snap = dest / "snapshot"
    snap.mkdir()
    assess_src = out / sid / "assessment.json"
    if not assess_src.exists():
        raise SystemExit(f"missing assessment for {sid}: {assess_src}")

    cands_path = dest / "candidates.json"
    if kind == "tech_spec":
        adr_root = dest / "adrs"
        adr_root.mkdir()
        src = pack / sub["path"]
        (adr_root / f"{sid}.md").write_text(src.read_text(encoding="utf-8"), encoding="utf-8")
        index_path = dest / "ingested.json"
        extract_path = dest / "extracted.json"
        run([
            "go", "run", "./cmd/adrctl", "ingest-adrs",
            "--root", str(adr_root),
            "--json-out", str(index_path),
        ])
        # extract may yield zero candidates for sparse ADRs; still OK.
        subprocess.run([
            "go", "run", "./cmd/adrctl", "extract-obligations",
            "--index", str(index_path),
            "--root", str(adr_root),
            "--json-out", str(extract_path),
            "--min-confidence-report",
        ], check=False)
        if extract_path.exists():
            payload = json.loads(extract_path.read_text())
        else:
            payload = {"schema_version": "obligation-candidates-v1", "candidates": []}
        for c in payload.get("candidates") or []:
            c["submission_kind"] = kind
        # Always include a submission-level row so Runs/Confidence show the body-of-evidence id.
        assessment = json.loads(assess_src.read_text())
        band = {"PASS": "high", "FAIL": "low"}.get(assessment.get("verdict"), "medium")
        payload.setdefault("candidates", []).append({
            "id": f"sub-{sid}",
            "adr_id": sid,
            "statement": f"{sub['title']} — assessed verdict {assessment.get('verdict')} (coverage {assessment.get('coverage_status')})",
            "modality": "INFORMATIONAL",
            "kind": "workshop_submission",
            "submission_kind": kind,
            "pattern_hint": "workshop.submission",
            "confidence": band,
            "approval_status": "pending_review",
            "enforcement_mode": "review_required",
            "executable": False,
            "source_path": sub.get("path"),
            "domains": sub.get("domains"),
            "source_url": sub.get("source_url"),
        })
        cands_path.write_text(json.dumps(payload, indent=2) + "\n")
    else:
        assessment = json.loads(assess_src.read_text())
        band = {"PASS": "high", "FAIL": "low"}.get(assessment.get("verdict"), "medium")
        payload = {
            "schema_version": "obligation-candidates-v1",
            "candidates": [{
                "id": f"sub-{sid}",
                "adr_id": sid,
                "statement": f"{sub['title']} — assessed verdict {assessment.get('verdict')} (coverage {assessment.get('coverage_status')})",
                "modality": "INFORMATIONAL",
                "kind": "workshop_submission",
                "submission_kind": kind,
                "pattern_hint": "workshop.submission",
                "confidence": band,
                "approval_status": "pending_review",
                "enforcement_mode": "review_required",
                "executable": False,
                "source_path": sub.get("path"),
                "domains": sub.get("domains"),
                "source_url": sub.get("source_url"),
            }],
        }
        cands_path.write_text(json.dumps(payload, indent=2) + "\n")

    # Calibrate confidence_p / bands (platt_conformal).
    run(["go", "run", "scripts/rescore_workshop_candidates.go", str(cands_path)])

    # Single-submission index for ADR flagging + Submissions page.
    assessment = json.loads(assess_src.read_text())
    if not assessment.get("submission_kind"):
        assessment["submission_kind"] = kind
        assess_src.write_text(json.dumps(assessment, indent=2) + "\n")
    sub_row = {
        "id": sid,
        "title": sub["title"],
        "submission_kind": kind,
        "domains": sub.get("domains", []),
        "path": sub["path"],
        "source_url": sub.get("source_url"),
        "repo": sub.get("repo"),
        "license": sub.get("license"),
        "verdict": assessment.get("verdict"),
        "coverage_status": assessment.get("coverage_status"),
        "requirement_count": assessment.get("requirement_count"),
        "runtime_llm_calls": assessment.get("runtime_llm_calls"),
        "assessment_path": str(assess_src),
    }
    subs_index = {
        "schema_version": "workshop-submission-results-v1",
        "run_id": run_id,
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "counts": {
            "total": 1,
            "tech_spec": 1 if kind == "tech_spec" else 0,
            "code_snippet": 1 if kind == "code_snippet" else 0,
        },
        "verdicts": {
            "PASS": 1 if assessment.get("verdict") == "PASS" else 0,
            "FAIL": 1 if assessment.get("verdict") == "FAIL" else 0,
            "INDETERMINATE": 1 if assessment.get("verdict") == "INDETERMINATE" else 0,
        },
        "submissions": [sub_row],
    }
    subs_path = dest / "submissions.json"
    subs_path.write_text(json.dumps(subs_index, indent=2) + "\n")

    # Ensure assessment carries kind for KPI emit.
    assess_emit = dest / "assessment.json"
    shutil.copy2(assess_src, assess_emit)

    run([
        "go", "run", "./cmd/adrctl", "workshop-emit",
        "--out-dir", str(snap),
        "--run-id", run_id,
        "--workflow", workflow,
        "--git-sha", git_sha,
        "--candidates", str(cands_path),
        "--assessment", str(assess_emit),
        "--submissions", str(subs_path),
        "--corpus-labels", "testdata/corpus/labels.json",
        "--enforce-assess-llm",
    ])
    shutil.copy2(subs_path, snap / "submissions.json")

    # Stamp kind KPIs explicitly for honesty on single-kind runs.
    for name in ("metrics.json", "latest.json"):
        p = snap / name
        data = json.loads(p.read_text())
        metrics = data.get("metrics", data)
        metrics["kpi.submission.tech_spec_count"] = 1 if kind == "tech_spec" else 0
        metrics["kpi.submission.code_snippet_count"] = 1 if kind == "code_snippet" else 0
        metrics["kpi.assess.submission_kind"] = kind
        if "metrics" in data:
            data["metrics"] = metrics
            data["summary"] = f"Unique workshop run for {sid} ({kind})."
            # Stable-ish timestamps so Runs order follows submission id order within kind.
            ts = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
            data["run"]["timestamp"] = data["run"].get("timestamp") or ts
            p.write_text(json.dumps(data, indent=2) + "\n")
        else:
            p.write_text(json.dumps(metrics, indent=2) + "\n")

    run([
        "python3", "scripts/workshop_publish_data.py",
        "--snapshot", str(snap),
        "--data-dir", str(data_dir),
        "--retain-snapshots", "200",
    ])
    print(f"[{i}/{len(subs)}] published {run_id}", flush=True)

# Recount
runs = (data_dir / "metrics" / "runs.jsonl").read_text().strip().splitlines()
snaps = list((data_dir / "snapshots").iterdir()) if (data_dir / "snapshots").exists() else []
print(f"DONE runs.jsonl={len(runs)} snapshots={len([p for p in snaps if p.is_dir()])}")
PY

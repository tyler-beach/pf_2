#!/usr/bin/env bash
# Assess all workshop body-of-evidence submissions and emit a workshop snapshot.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
export PACK="${PACK:-testdata/workshop-submissions}"
export REQ="${REQ:-testdata/corpus/requirements.json}"
export OUT="${OUT:-$PACK/results}"
export SNAP="${SNAP:-$PACK/workshop-snapshot}"
export RUN_ID="${RUN_ID:-workshop-submissions-$(date -u +%Y%m%dT%H%M%SZ)}"
export MANIFEST="$PACK/manifest.json"
test -f "$MANIFEST"
mkdir -p "$OUT" "$SNAP" /tmp/ws-adr-extract

# Build once; per-submission `go run` dominated batch wall time without changing verdicts.
ADRCTL_BIN="${ADRCTL_BIN:-}"
if [[ -z "$ADRCTL_BIN" ]]; then
  ADRCTL_BIN="$(mktemp -t adrctl.XXXXXX)"
  go build -o "$ADRCTL_BIN" ./cmd/adrctl
  trap 'rm -f "$ADRCTL_BIN"' EXIT
fi
export ADRCTL_BIN

python3 <<'PY'
import json, os, shutil, subprocess
from pathlib import Path
from datetime import datetime, timezone

manifest_path = Path(os.environ["MANIFEST"])
out_dir = Path(os.environ["OUT"])
req = Path(os.environ["REQ"])
run_id = os.environ["RUN_ID"]
adrctl = os.environ["ADRCTL_BIN"]
pack = manifest_path.parent
manifest = json.loads(manifest_path.read_text())
out_dir.mkdir(parents=True, exist_ok=True)
results_index = []
adr_root = Path("/tmp/ws-adr-extract/adrs")
if adr_root.exists():
    shutil.rmtree(adr_root)
adr_root.mkdir(parents=True)

for sub in manifest["submissions"]:
    sid = sub["id"]
    kind = sub["submission_kind"]
    src = pack / sub["path"]
    dest = out_dir / sid
    dest.mkdir(parents=True, exist_ok=True)
    shutil.copy2(src, dest / "issue.md")
    (dest / "meta.json").write_text(json.dumps(sub, indent=2) + "\n")
    cmd = [
        adrctl, "assess",
        "--input", str(dest / "issue.md"),
        "--requirements", str(req),
        "--issue-surface",
        "--submission-kind", kind,
        "--report", str(dest / "report.md"),
        "--json-out", str(dest / "assessment.json"),
        "--sarif-out", str(dest / "results.sarif"),
    ]
    print(f"==> assess {sid} ({kind})", flush=True)
    subprocess.run(cmd, check=True)
    assessment = json.loads((dest / "assessment.json").read_text())
    if not assessment.get("submission_kind"):
        assessment["submission_kind"] = kind
        (dest / "assessment.json").write_text(json.dumps(assessment, indent=2) + "\n")
    row = {
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
        "assessment_path": str(dest / "assessment.json"),
    }
    results_index.append(row)
    if kind == "tech_spec":
        (adr_root / f"{sid}.md").write_text(src.read_text(encoding="utf-8"), encoding="utf-8")

(out_dir / "index.json").write_text(json.dumps({
    "schema_version": "workshop-submission-results-v1",
    "run_id": run_id,
    "timestamp": datetime.now(timezone.utc).isoformat(),
    "counts": {
        "total": len(results_index),
        "tech_spec": sum(1 for r in results_index if r["submission_kind"] == "tech_spec"),
        "code_snippet": sum(1 for r in results_index if r["submission_kind"] == "code_snippet"),
    },
    "verdicts": {
        "PASS": sum(1 for r in results_index if r["verdict"] == "PASS"),
        "FAIL": sum(1 for r in results_index if r["verdict"] == "FAIL"),
        "INDETERMINATE": sum(1 for r in results_index if r["verdict"] == "INDETERMINATE"),
    },
    "submissions": results_index,
}, indent=2) + "\n")
print(f"wrote {out_dir / 'index.json'}", flush=True)
PY

"$ADRCTL_BIN" ingest-adrs --root /tmp/ws-adr-extract/adrs --json-out "$OUT/ingested-tech-specs.json"
"$ADRCTL_BIN" extract-obligations \
  --index "$OUT/ingested-tech-specs.json" \
  --root /tmp/ws-adr-extract/adrs \
  --json-out "$OUT/candidates-tech-specs.json" \
  --min-confidence-report || true

python3 <<'PY'
import json, os
from pathlib import Path

out = Path(os.environ["OUT"])
pack = Path(os.environ["PACK"])
run_id = os.environ["RUN_ID"]
results = json.loads((out / "index.json").read_text())
cands = {
    "schema_version": "obligation-candidates-v1",
    "candidates": [],
    "candidate_count": 0,
    "confidence_summary": {"high": 0, "medium": 0, "low": 0},
}
tech_path = out / "candidates-tech-specs.json"
if tech_path.exists():
    tech = json.loads(tech_path.read_text())
    for c in tech.get("candidates") or []:
        c["submission_kind"] = c.get("submission_kind") or "tech_spec"
        cands["candidates"].append(c)
for sub in results["submissions"]:
    band = "medium"
    if sub.get("verdict") == "FAIL":
        band = "low"
    elif sub.get("verdict") == "PASS":
        band = "high"
    # Batch rollup rows are not extract-calibrated; mark heuristic so KPIs stay honest.
    score = {"high": 0.85, "medium": 0.55, "low": 0.25}.get(band, 0.5)
    cands["candidates"].append({
        "id": f"sub-{sub['id']}",
        "adr_id": sub["id"],
        "statement": f"{sub['title']} — assessed verdict {sub.get('verdict')} (coverage {sub.get('coverage_status')})",
        "modality": "INFORMATIONAL",
        "kind": "workshop_submission",
        "submission_kind": sub["submission_kind"],
        "pattern_hint": "workshop.submission",
        "confidence": band,
        "confidence_score": score,
        "confidence_p": score,
        "confidence_method": "heuristic",
        "approval_status": "pending_review",
        "enforcement_mode": "review_required",
        "executable": False,
        "source_path": sub.get("path"),
        "domains": sub.get("domains"),
        "source_url": sub.get("source_url"),
    })
for c in cands["candidates"]:
    b = c.get("confidence") or "medium"
    if b in cands["confidence_summary"]:
        cands["confidence_summary"][b] += 1
cands["candidate_count"] = len(cands["candidates"])
(out / "candidates.json").write_text(json.dumps(cands, indent=2) + "\n")
assessment = {
    "verdict": "INDETERMINATE",
    "coverage_status": "PARTIAL",
    "requirement_count": sum(int(s.get("requirement_count") or 0) for s in results["submissions"]) // max(1, len(results["submissions"])),
    "runtime_llm_calls": 0,
    "submission_kind": "",
    "workshop_batch": {
        "run_id": run_id,
        "tech_spec_count": results["counts"]["tech_spec"],
        "code_snippet_count": results["counts"]["code_snippet"],
        "total": results["counts"]["total"],
        "verdicts": results["verdicts"],
    },
    "requirements": [],
}
(out / "assessment-rollup.json").write_text(json.dumps(assessment, indent=2) + "\n")
(out / "submissions.json").write_text(json.dumps(results, indent=2) + "\n")
print(
    "candidates", cands["candidate_count"],
    "tech_spec", sum(1 for c in cands["candidates"] if c.get("submission_kind") == "tech_spec"),
    "code_snippet", sum(1 for c in cands["candidates"] if c.get("submission_kind") == "code_snippet"),
)
PY

GIT_SHA="$(git rev-parse HEAD)"
go run ./cmd/adrctl workshop-emit \
  --out-dir "$SNAP" \
  --run-id "$RUN_ID" \
  --workflow "Workshop Submissions Batch" \
  --git-sha "$GIT_SHA" \
  --candidates "$OUT/candidates.json" \
  --assessment "$OUT/assessment-rollup.json" \
  --submissions "$OUT/submissions.json" \
  --corpus-labels testdata/corpus/labels.json \
  --enforce-assess-llm

cp "$OUT/submissions.json" "$SNAP/submissions.json"
cp "$PACK/manifest.json" "$SNAP/workshop-submissions-manifest.json"
# Keep a copy of the ADR flagging rollup next to results for offline reuse.
if [[ -f "$SNAP/adr_flagging.json" ]]; then
  cp "$SNAP/adr_flagging.json" "$OUT/adr_flagging.json"
fi

python3 <<'PY'
import json, os
from pathlib import Path
snap = Path(os.environ["SNAP"])
subs = json.loads((Path(os.environ["OUT"]) / "submissions.json").read_text())
for name in ("metrics.json", "latest.json"):
    p = snap / name
    data = json.loads(p.read_text())
    metrics = data.get("metrics", data)
    metrics["kpi.submission.tech_spec_count"] = subs["counts"]["tech_spec"]
    metrics["kpi.submission.code_snippet_count"] = subs["counts"]["code_snippet"]
    if not metrics.get("kpi.assess.submission_kind"):
        metrics["kpi.assess.submission_kind"] = "mixed"
    if "metrics" in data:
        data["metrics"] = metrics
        data["summary"] = (
            f"Workshop submissions batch: {subs['counts']['tech_spec']} tech_spec + "
            f"{subs['counts']['code_snippet']} code_snippet assessed."
        )
        p.write_text(json.dumps(data, indent=2) + "\n")
    else:
        p.write_text(json.dumps(metrics, indent=2) + "\n")
print("SNAPSHOT", snap, "RUN_ID", os.environ["RUN_ID"])
PY

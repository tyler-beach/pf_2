#!/usr/bin/env bash
# Assess all workshop body-of-evidence submissions and emit a workshop snapshot.
#
# Prefer applicable-evidence assess (Epic 2): when OpenGrep/Checkov binaries are
# available and a submission yields sandbox snippets, extract fences, scan, then
# assess-repo with --issue-surface / --submission-kind / --scope (no --assess-all).
# Without scanners or snippets, fall back to fence-only assess with the same
# applicability filter — tech_spec prose stays mostly N/A (documented, not flooded).
#
# Scanner env (optional; auto-detected under ~/.cache/pathfinder-tools when present):
#   OPENGREP_BIN, OPENGREP_SHA256, OPENGREP_VERSION (default 1.33.0)
#   CHECKOV_BIN, CHECKOV_SHA256, CHECKOV_VERSION (default 3.3.30)
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
export PACK="${PACK:-testdata/workshop-submissions}"
export REQ="${REQ:-testdata/corpus/requirements.json}"
export OUT="${OUT:-$PACK/results}"
export SNAP="${SNAP:-$PACK/workshop-snapshot}"
export RUN_ID="${RUN_ID:-workshop-submissions-$(date -u +%Y%m%dT%H%M%SZ)}"
export MANIFEST="$PACK/manifest.json"
export SCOPE="${SCOPE:-.}"
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

TOOLS_CACHE="${PATHFINDER_TOOLS_CACHE:-$HOME/.cache/pathfinder-tools}"
if [[ -z "${OPENGREP_BIN:-}" && -x "$TOOLS_CACHE/opengrep-1.33.0" ]]; then
  OPENGREP_BIN="$TOOLS_CACHE/opengrep-1.33.0"
fi
if [[ -z "${CHECKOV_BIN:-}" && -x "$TOOLS_CACHE/checkov-venv/bin/checkov" ]]; then
  CHECKOV_BIN="$TOOLS_CACHE/checkov-venv/bin/checkov"
fi
OPENGREP_VERSION="${OPENGREP_VERSION:-1.33.0}"
CHECKOV_VERSION="${CHECKOV_VERSION:-3.3.30}"
if [[ -n "${OPENGREP_BIN:-}" && -z "${OPENGREP_SHA256:-}" && -x "${OPENGREP_BIN}" ]]; then
  OPENGREP_SHA256="$(shasum -a 256 "$OPENGREP_BIN" | awk '{print $1}')"
fi
if [[ -n "${CHECKOV_BIN:-}" && -z "${CHECKOV_SHA256:-}" && -x "${CHECKOV_BIN}" ]]; then
  CHECKOV_SHA256="$(shasum -a 256 "$CHECKOV_BIN" | awk '{print $1}')"
fi

SCANNERS_READY=0
if [[ -n "${OPENGREP_BIN:-}" && -x "${OPENGREP_BIN}" && -n "${OPENGREP_SHA256:-}" \
   && -n "${CHECKOV_BIN:-}" && -x "${CHECKOV_BIN}" && -n "${CHECKOV_SHA256:-}" ]]; then
  SCANNERS_READY=1
  echo "==> scanners ready: opengrep=$OPENGREP_BIN checkov=$CHECKOV_BIN"
else
  echo "==> scanners unavailable — assess-only with applicability filter (OpenGrep/Checkov N/A)"
  echo "    set OPENGREP_BIN/CHECKOV_BIN (and SHA256) to enable sandbox analyzer evidence"
fi
export SCANNERS_READY OPENGREP_BIN OPENGREP_SHA256 OPENGREP_VERSION
export CHECKOV_BIN CHECKOV_SHA256 CHECKOV_VERSION SCOPE ROOT

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
root = Path(os.environ["ROOT"])
scope = os.environ.get("SCOPE") or "."
scanners_ready = os.environ.get("SCANNERS_READY") == "1"
opengrep_bin = os.environ.get("OPENGREP_BIN") or ""
opengrep_sha = os.environ.get("OPENGREP_SHA256") or ""
opengrep_ver = os.environ.get("OPENGREP_VERSION") or "1.33.0"
checkov_bin = os.environ.get("CHECKOV_BIN") or ""
checkov_sha = os.environ.get("CHECKOV_SHA256") or ""
checkov_ver = os.environ.get("CHECKOV_VERSION") or "3.3.30"

manifest = json.loads(manifest_path.read_text())
out_dir.mkdir(parents=True, exist_ok=True)
results_index = []
adr_root = Path("/tmp/ws-adr-extract/adrs")
if adr_root.exists():
    shutil.rmtree(adr_root)
adr_root.mkdir(parents=True)

scanner_stats = {
    "scanners_ready": scanners_ready,
    "with_analyzer_evidence": 0,
    "assess_only_applicability": 0,
    "sandbox_snippets_total": 0,
    "notes": [],
}

def run(cmd, **kw):
    print("+", " ".join(str(c) for c in cmd), flush=True)
    subprocess.run(cmd, check=True, **kw)

def try_run(cmd, **kw):
    print("+", " ".join(str(c) for c in cmd), flush=True)
    return subprocess.run(cmd, check=False, **kw).returncode == 0

def snippet_count(sandbox: Path) -> int:
    if not sandbox.exists():
        return 0
    n = 0
    for p in sandbox.rglob("*"):
        if p.is_file() and p.name != ".pathfinder-sandbox" and "rules" not in p.parts and "analyzer-evidence" not in p.parts:
            n += 1
    return n

def assess_only(dest: Path, kind: str, why):
    print(
        f"==> assess {dest.name} ({kind}) assess-only ({','.join(why) or 'fallback'})",
        flush=True,
    )
    run([
        adrctl, "assess",
        "--input", str(dest / "issue.md"),
        "--requirements", str(req),
        "--issue-surface",
        "--submission-kind", kind,
        "--scope", scope,
        "--report", str(dest / "report.md"),
        "--json-out", str(dest / "assessment.json"),
        "--sarif-out", str(dest / "results.sarif"),
    ])
    scanner_stats["assess_only_applicability"] += 1
    if why:
        scanner_stats["notes"].append({"id": dest.name, "mode": "assess_only", "why": why})

for sub in manifest["submissions"]:
    sid = sub["id"]
    kind = sub["submission_kind"]
    src = pack / sub["path"]
    dest = out_dir / sid
    dest.mkdir(parents=True, exist_ok=True)
    shutil.copy2(src, dest / "issue.md")
    (dest / "meta.json").write_text(json.dumps(sub, indent=2) + "\n")

    sandbox = dest / "sandbox"
    if sandbox.exists():
        shutil.rmtree(sandbox)
    run([
        "python3", str(root / "scripts" / "issue_sandbox_extract.py"),
        "--input", str(dest / "issue.md"),
        "--out-dir", str(sandbox),
    ])
    snippets = snippet_count(sandbox)
    scanner_stats["sandbox_snippets_total"] += snippets
    mode = "assess_only"

    # Corpus analyzer-rule applies_to defaults to code_snippet; scanning tech_spec
    # fences cannot produce assessed signal under --submission-kind tech_spec.
    use_analyzers = scanners_ready and snippets > 0 and kind == "code_snippet"
    if use_analyzers:
        # Mirror .github/workflows/adr-assessment.yml sandbox path.
        rules_dir = sandbox / "rules"
        rules_dir.mkdir(parents=True, exist_ok=True)
        shutil.copy2(root / "rules" / "pathfinder-security.yml", rules_dir / "pathfinder-security.yml")
        shutil.copy2(root / "rules" / "checkov.yml", rules_dir / "checkov.yml")
        ev_dir = dest / "analyzer-evidence"
        if ev_dir.exists():
            shutil.rmtree(ev_dir)
        ev_dir.mkdir(parents=True)

        og_ok = try_run([
            adrctl, "scan-opengrep",
            "--root", str(sandbox),
            "--config", "rules/pathfinder-security.yml",
            "--scope", scope,
            "--binary", opengrep_bin,
            "--binary-sha256", opengrep_sha,
            "--version", opengrep_ver,
            "--json-out", str(ev_dir / "opengrep.json"),
            "--sarif-out", str(ev_dir / "opengrep.sarif"),
            "--manifest-out", str(ev_dir / "opengrep-manifest.json"),
            "--cache-dir", "",
        ])
        ckv_ok = try_run([
            adrctl, "scan-checkov",
            "--root", str(sandbox),
            "--config", "rules/checkov.yml",
            "--scope", scope,
            "--binary", checkov_bin,
            "--binary-sha256", checkov_sha,
            "--version", checkov_ver,
            "--json-out", str(ev_dir / "checkov.json"),
            "--sarif-out", str(ev_dir / "checkov.sarif"),
            "--manifest-out", str(ev_dir / "checkov-manifest.json"),
            "--cache-dir", "",
        ])
        if og_ok and ckv_ok:
            evidence_paths = [str(ev_dir / "opengrep.json"), str(ev_dir / "checkov.json")]
            sarif_paths = [str(ev_dir / "opengrep.sarif"), str(ev_dir / "checkov.sarif")]
            print(f"==> assess-repo {sid} ({kind}) snippets={snippets} +analyzers", flush=True)
            assess_ok = try_run([
                adrctl, "assess-repo",
                "--root", str(sandbox),
                "--requirements", str(req),
                "--issue-surface",
                "--submission-kind", kind,
                "--scope", scope,
                "--analyzer-evidence", ",".join(evidence_paths),
                "--analyzer-sarif", ",".join(sarif_paths),
                "--report", str(dest / "report.md"),
                "--json-out", str(dest / "assessment.json"),
                "--sarif-out", str(dest / "results.sarif"),
            ])
            if assess_ok:
                mode = "analyzer_evidence"
                scanner_stats["with_analyzer_evidence"] += 1
            else:
                assess_only(dest, kind, ["assess_repo_failed"])
        else:
            why = []
            if not og_ok:
                why.append("opengrep_incomplete")
            if not ckv_ok:
                why.append("checkov_failed")
            assess_only(dest, kind, why)
    else:
        why = []
        if not scanners_ready:
            why.append("scanners_unavailable")
        if snippets == 0:
            why.append("no_sandbox_snippets")
        elif kind != "code_snippet":
            why.append("tech_spec_analyzer_na")
        assess_only(dest, kind, why)
    assessment = json.loads((dest / "assessment.json").read_text())
    if not assessment.get("submission_kind"):
        assessment["submission_kind"] = kind
        (dest / "assessment.json").write_text(json.dumps(assessment, indent=2) + "\n")
    (dest / "assess_meta.json").write_text(json.dumps({
        "id": sid,
        "mode": mode,
        "sandbox_snippets": snippets,
        "scanners_ready": scanners_ready,
        "scope": scope,
        "applicable_requirement_count": assessment.get("applicable_requirement_count"),
        "verdict": assessment.get("verdict"),
        "coverage_status": assessment.get("coverage_status"),
        "counts": assessment.get("counts"),
    }, indent=2) + "\n")
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
        "applicable_requirement_count": assessment.get("applicable_requirement_count"),
        "runtime_llm_calls": assessment.get("runtime_llm_calls"),
        "assess_mode": mode,
        "sandbox_snippets": snippets,
        "assessment_path": str(dest / "assessment.json"),
    }
    results_index.append(row)
    if kind == "tech_spec":
        (adr_root / f"{sid}.md").write_text(src.read_text(encoding="utf-8"), encoding="utf-8")

if not scanners_ready:
    scanner_stats["notes"].append({
        "global": "OpenGrep/Checkov not configured; code_snippet and tech_spec runs used assess-only applicability filter. Analyzer-backed PASS/FAIL remain N/A until scanners are provided.",
    })
else:
    scanner_stats["notes"].append({
        "global": "Submissions without extractable fences (typical tech_spec prose) stay assess-only; analyzer rules for missing extensions remain NOT_APPLICABLE by applicability.",
    })

(out_dir / "index.json").write_text(json.dumps({
    "schema_version": "workshop-submission-results-v1",
    "run_id": run_id,
    "timestamp": datetime.now(timezone.utc).isoformat(),
    "scanner": scanner_stats,
    "counts": {
        "total": len(results_index),
        "tech_spec": sum(1 for r in results_index if r["submission_kind"] == "tech_spec"),
        "code_snippet": sum(1 for r in results_index if r["submission_kind"] == "code_snippet"),
        "with_analyzer_evidence": scanner_stats["with_analyzer_evidence"],
        "assess_only_applicability": scanner_stats["assess_only_applicability"],
        "applicable_gt_zero": sum(1 for r in results_index if int(r.get("applicable_requirement_count") or 0) > 0),
    },
    "verdicts": {
        "PASS": sum(1 for r in results_index if r["verdict"] == "PASS"),
        "FAIL": sum(1 for r in results_index if r["verdict"] == "FAIL"),
        "INDETERMINATE": sum(1 for r in results_index if r["verdict"] == "INDETERMINATE"),
    },
    "submissions": results_index,
}, indent=2) + "\n")
print(f"wrote {out_dir / 'index.json'}", flush=True)
print(
    "assess modes: analyzer_evidence="
    f"{scanner_stats['with_analyzer_evidence']} assess_only={scanner_stats['assess_only_applicability']} "
    f"applicable>0={sum(1 for r in results_index if int(r.get('applicable_requirement_count') or 0) > 0)}",
    flush=True,
)
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
    "applicable_requirement_count": sum(int(s.get("applicable_requirement_count") or 0) for s in results["submissions"]),
    "runtime_llm_calls": 0,
    "submission_kind": "",
    "workshop_batch": {
        "run_id": run_id,
        "tech_spec_count": results["counts"]["tech_spec"],
        "code_snippet_count": results["counts"]["code_snippet"],
        "total": results["counts"]["total"],
        "verdicts": results["verdicts"],
        "with_analyzer_evidence": results["counts"].get("with_analyzer_evidence"),
        "assess_only_applicability": results["counts"].get("assess_only_applicability"),
        "applicable_gt_zero": results["counts"].get("applicable_gt_zero"),
        "scanner": results.get("scanner"),
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
            f"{subs['counts']['code_snippet']} code_snippet assessed "
            f"(analyzer_evidence={subs['counts'].get('with_analyzer_evidence', 0)}, "
            f"assess_only={subs['counts'].get('assess_only_applicability', 0)}, "
            f"applicable>0={subs['counts'].get('applicable_gt_zero', 0)})."
        )
        p.write_text(json.dumps(data, indent=2) + "\n")
    else:
        p.write_text(json.dumps(metrics, indent=2) + "\n")
print("SNAPSHOT", snap, "RUN_ID", os.environ["RUN_ID"])
PY

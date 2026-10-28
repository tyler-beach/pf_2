#!/usr/bin/env bash
# Local E2E for Pathfinder PR-scan converter + workflow input shapes.
# Does not call GitHub APIs or execute untrusted PR code.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
FIX="$ROOT/testdata/pr-scan/known-fail"
OUT="${OUT:-$ROOT/testdata/pr-scan/known-fail/out}"
mkdir -p "$OUT"

ADRCTL_BIN="${ADRCTL_BIN:-}"
if [[ -z "$ADRCTL_BIN" ]]; then
  ADRCTL_BIN="$(mktemp -t adrctl.XXXXXX)"
  go build -o "$ADRCTL_BIN" ./cmd/adrctl
  trap 'rm -f "$ADRCTL_BIN"' EXIT
fi

echo "==> parse golden issue body"
"$ADRCTL_BIN" parse-pr-scan-issue --body "$FIX/issue-body.md" | tee "$OUT/request.json"
python3 - <<'PY'
import json, sys
from pathlib import Path
req = json.loads(Path("testdata/pr-scan/known-fail/out/request.json").read_text())
assert req["PR"]["Owner"] == "tyler-beach", req
assert req["PR"]["Repo"] == "pf_2", req
assert req["UploadSARIF"] is True
assert "github-pr-review" in req["Reporters"]
assert req["Catalog"] == "live"
assert req["FailLevel"] == "any"
print("issue parse OK")
PY

echo "==> synthesize workflow_dispatch request shape"
python3 - <<'PY'
import json
from pathlib import Path
# Mirrors workflow_dispatch → GITHUB_OUTPUT fields consumed by later steps.
dispatch = {
    "PRURL": "https://github.com/tyler-beach/pf_2/pull/1",
    "PR": {"Owner": "tyler-beach", "Repo": "pf_2", "Number": 1},
    "Reporters": ["github-pr-review", "github-pr-check"],
    "UploadSARIF": True,
    "FilterMode": "added",
    "Catalog": "live",
    "FailLevel": "any",
}
Path("testdata/pr-scan/known-fail/out/dispatch-request.json").write_text(
    json.dumps(dispatch, indent=2) + "\n"
)
print("dispatch shape OK")
PY

echo "==> convert known-FAIL assessment → rdjson + Security SARIF + summary"
"$ADRCTL_BIN" assessment-to-rdjson \
  --assessment "$FIX/assessment-fail.json" \
  --rdjson-out "$OUT/pathfinder.rdjson" \
  --sarif-out "$OUT/pathfinder-security.sarif" \
  --summary-out "$OUT/pathfinder-pr-summary.md" \
  --max-inline 25 \
  --pr-url "https://github.com/tyler-beach/pf_2/pull/1" \
  --head-sha "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef" \
  --run-id "e2e-local-run" \
  --docs-base "https://tyler-beach.github.io/pf_2/#/docs/pr-scan"

python3 - <<'PY'
import json
from pathlib import Path
rd = json.loads(Path("testdata/pr-scan/known-fail/out/pathfinder.rdjson").read_text())
sarif = json.loads(Path("testdata/pr-scan/known-fail/out/pathfinder-security.sarif").read_text())
summary = Path("testdata/pr-scan/known-fail/out/pathfinder-pr-summary.md").read_text()
diags = rd.get("diagnostics") or []
assert len(diags) >= 1, diags
for d in diags:
    msg = d.get("message", "")
    assert "N/A" not in msg and "NOT_APPLICABLE" not in msg, msg
    assert "Why:" in msg and "Evidence:" in msg
results = sarif["runs"][0]["results"]
assert len(results) >= 1
for r in results:
    assert r.get("ruleId", "").startswith("pathfinder/")
assert "pathfinder-pr-scan:" in summary or "deadbeef" in summary
assert "FAIL" in summary or "deviation" in summary.lower() or "Finding" in summary or "REQ-003" in summary
assert "#/runs?run_id=e2e-local-run" in summary
assert "#/submissions" in summary
print(f"converter OK: {len(diags)} rdjson diagnostic(s), {len(results)} SARIF result(s)")
PY

echo "==> pass-issue / deviation-issue still oracle PASS/FAIL under applicability"
"$ADRCTL_BIN" assess --input testdata/pass-issue.md --requirements adr/requirements.json \
  --submission-kind code_snippet \
  --report "$OUT/pass.md" --json-out "$OUT/pass.json" --sarif-out "$OUT/pass.sarif" >/dev/null
"$ADRCTL_BIN" assess --input testdata/deviation-issue.md --requirements adr/requirements.json \
  --submission-kind code_snippet \
  --report "$OUT/dev.md" --json-out "$OUT/dev.json" --sarif-out "$OUT/dev.sarif" >/dev/null
python3 - <<'PY'
import json
from pathlib import Path
assert json.loads(Path("testdata/pr-scan/known-fail/out/pass.json").read_text())["verdict"] == "PASS"
assert json.loads(Path("testdata/pr-scan/known-fail/out/dev.json").read_text())["verdict"] == "FAIL"
print("pass/deviation oracles OK")
PY

echo "PR-scan local E2E passed."

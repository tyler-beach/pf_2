# Statistical confidence artifacts

| File | Role |
|---|---|
| `gold-extract-v1.json` | Seed gold labels for extract quality (`extract_correct`, `expected_band`, assess/assist hooks). |
| `calibration-v1.json` | Fitted Platt + conformal params (mirror of embedded `internal/obligations/calibration/params-v1.json`). |

## Methods

- **Platt / logistic** (`confidence_p`): calibrated P(extract_correct) from a small feature vector (internal heuristic triage is one *input* feature; not emitted as `confidence_score`).
- **Heuristic fallback** (`confidence_method=heuristic`): only when gold/params are missing — labeled honestly; still no public `confidence_score`.
- **Inductive conformal bands** (`confidence_band`, `confidence_coverage`): prediction sets at target coverage (default 90%) under an **exchangeability** assumption with the gold calibration set. Singleton `{high}` → high; otherwise widen.

Refit after growing **extract** gold (including adjudicated workshop labels):

```bash
PF_WRITE_CALIBRATION=1 go test ./internal/obligations/calibration/ -run TestFitAndWriteParams -count=1
```

That rewrites `internal/obligations/calibration/params-v1.json` and `testdata/confidence/calibration-v1.json`. Do **not** refit on N/A-only assess batches. Assess oracles (pass/deviation fixtures) live in `gold-assess-oracles-v1.json` and do not drive Platt params.

Do not claim calibrated meaning when method falls back to heuristic.

## Refresh ADR Flagging after applicable-evidence (Epic 2)

Workshop `adr_flagging.json` only shows `assessed > 0` when assessments produce PASS/FAIL (not all-N/A). After Epic 2:

1. Re-assess submissions **with** OpenGrep/Checkov evidence when binaries are available (`OPENGREP_BIN` / `CHECKOV_BIN`, optional SHA env). `./scripts/assess_workshop_submissions.sh` extracts fences, scans, then `assess-repo --issue-surface --submission-kind --scope` (no `--assess-all`). Without scanners, the same script still re-emits **assess-only with applicability filtering** — better than the pre-epic EVIDENCE_MISSING flood, but analyzer-backed rows stay N/A (typical for prose `tech_spec`).
2. Emit unique runs: `./scripts/emit_unique_workshop_runs.sh --wipe`, or publish the batch snapshot via `python3 scripts/workshop_publish_data.py --snapshot testdata/workshop-submissions/workshop-snapshot --data-dir workshop/public/data`.
3. Confirm `#/adr-flags` leaderboards show non-zero aligned/deviate for analyzer-backed `run_id`s (`results/index.json` → `counts.applicable_gt_zero` / `with_analyzer_evidence`).

PR-scan summaries deep-link to `#/runs?run_id=<GITHUB_RUN_ID>` and `#/submissions`.

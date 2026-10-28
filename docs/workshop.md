# Pathfinder Workshop (GitHub Pages)

The workshop is the GitHub-era **command center** for Pathfinder: polished internal SaaS UI over GitOps snapshots. It includes KPI drilldowns, pipeline deep-dives, ADR dig-ins, LLM cost/usage, run compare, and an interactive **Sample** technical journey. There is no live backend; GitHub Actions compute, the `workshop-data` branch stores history, and Pages serves a cached SPA shell with short-lived data.

## SPA routes

| Route | Purpose |
|---|---|
| `#/` | Overview / SLO tiles / pipeline glance |
| `#/metrics` | KPI command deck with focus tabs + trends |
| `#/confidence` | Statistical confidence dashboard (calibrated P, conformal bands, Wilson CIs) |
| `#/docs/confidence` | Docs article: heuristic vs calibrated vs conformal |
| `#/pipeline/:step` | Per-stage deep analysis (I/O, failures, sample artifacts) |
| `#/sample/:step` | Interactive technical journey through Pathfinder |
| `#/adrs/:id` | Dig into published ADR bodies + linked candidates |
| `#/candidates` | Band filters, citations, low-confidence checklist issue |
| `#/proposals` | Diff view + accept semantics |
| `#/assess` | Deterministic assess + LLM invariant |
| `#/costs` | Assist token/cost rollups (offline=$0) |
| `#/runs` | History + A/B metric compare |
| `#/evals` / `#/corpus` / `#/docs` | Promptfoo, harness, docs |

## Data flow

1. **CI**, **ADR Assessment**, **Labeled corpus evaluation**, and **LLM Assist** each call `adrctl workshop-emit` and upload a `workshop-inputs*` artifact (snapshot + source JSON).
2. **Workshop Publish** (`workflow_run` on those workflows, or manual dispatch) downloads the artifact, appends to the `workshop-data` branch (`metrics/runs.jsonl`, `snapshots/<run_id>/`, `latest.json`, `manifest.json`), builds [`workshop/`](../workshop/), and deploys GitHub Pages.
3. Humans use deep links (Issues / PR templates / `workflow_dispatch`) to write back; the static site never holds secrets.

## First-time Pages bootstrap

Do this once per repository (requires admin):

1. **Settings → Pages → Build and deployment → Source:** GitHub Actions.
2. Ensure an Environment named **`github-pages`** exists (Actions creates it on first `deploy-pages`, or create it under Settings → Environments).
3. Merge workshop workflows to the default branch, then either:
   - wait for a successful **CI** / **LLM Assist** / **Corpus Eval** / **ADR Assessment** run (triggers Workshop Publish), or
   - run **Workshop Publish** via `workflow_dispatch` (uses fixtures if no `source_run_id`).
4. Site URL: `https://<owner>.github.io/<repo>/` (project site) after the deploy job finishes (usually a few minutes).

SPA shell assets are content-hashed (long cache). Data files (`data/latest.json`, `data/manifest.json`) are fetched with a cache-busting query so trends refresh without a hard reload.

## Local development

```bash
# fixtures are seeded under workshop/public/data (and testdata/workshop-fixtures)
cd workshop && npm ci && npm run dev
```

Regenerate a snapshot:

```bash
go run ./cmd/adrctl workshop-emit \
  --out-dir /tmp/snapshot \
  --run-id local-1 \
  --workflow local \
  --candidates path/to/candidates.json
```

## Calibrated-only run history

`metrics/runs.jsonl` and published snapshots keep **only** runs with calibrated confidence fields (`kpi.confidence.method` / `confidence_p` / `platt_conformal`). Pre-calibration batch `workshop-submissions-20261028T015800Z` and incomplete fixture stubs without `confidence_p` are excluded. Republishing the same `run_id` upserts the metrics row instead of appending a duplicate.

## Interpreting metrics

See [kpi-dictionary.md](kpi-dictionary.md). **Promptfoo scores ≠ compliance verdicts.** Assess paths must keep `runtime_llm_calls == 0`.

## Follow-on (not this iteration)

Joern / CPG adapters when **≥10** accepted ADRs require cross-file data-flow evidence OpenGrep cannot supply (`N=10`). AWS comes after the GitHub workshop proves product value.

## Workshop submissions body of evidence

`testdata/workshop-submissions/` holds **50** real third-party excerpts (25 `tech_spec` + 25 `code_snippet`) with provenance metadata. Assess and publish:

```bash
./scripts/assess_workshop_submissions.sh
python3 scripts/workshop_publish_data.py \
  --snapshot testdata/workshop-submissions/workshop-snapshot \
  --data-dir workshop/public/data
```

Workshop UI: **Submissions** (`#/submissions`) and Candidates kind filter (`#/candidates?kind=tech_spec`).

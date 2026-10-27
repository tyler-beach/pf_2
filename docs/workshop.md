# Pathfinder Workshop (GitHub Pages)

The workshop is the GitHub-era **operational console + KPI observability** surface for Pathfinder. It is rebuilt from workflow snapshots within minutes of each run. There is no live backend; GitHub Actions compute, the `workshop-data` branch stores history, and Pages serves a cached SPA shell with short-lived data.

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

## Interpreting metrics

See [kpi-dictionary.md](kpi-dictionary.md). **Promptfoo scores ≠ compliance verdicts.** Assess paths must keep `runtime_llm_calls == 0`.

## Follow-on (not this iteration)

Joern / CPG adapters when ≥N accepted ADRs require cross-file data-flow evidence OpenGrep cannot supply. AWS comes after the GitHub workshop proves product value.

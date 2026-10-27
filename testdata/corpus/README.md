# Architect-labeled corpus (GitHub-native)

Target scale: **≥100** labeled ADRs under `adrs/` with matching rows in `labels.json`.
Generate or refresh labels with `python3 scripts/generate_corpus_labels.py` when expanding the set.

Submit additional ADR Markdown under `adrs/` and extend `labels.json` via pull request (`[corpus-label]` title/body).

Label fields:
- `statement_contains`: unique marker expected in an extracted candidate
- `modality`, `pattern_id`, `confidence_band`
- `escalate_expected`: true when the candidate should land in the low-confidence LLM assist band

Harness output includes `recall_by_confidence` (high/medium/low bands).

## Run

```bash
go run ./cmd/adrctl ingest-adrs --root testdata/corpus/adrs --json-out /tmp/corpus-ingested.json
go run ./cmd/adrctl extract-obligations --index /tmp/corpus-ingested.json --root testdata/corpus/adrs --json-out /tmp/corpus-candidates.json --min-confidence-report
python3 scripts/reuse_pilot_harness.py testdata/corpus/labels.json /tmp/corpus-candidates.json
```

Default CI always runs the synthetic reuse-pilot. The labeled corpus workflow is opt-in via `workflow_dispatch` or path filters.

Harness output includes `recall_by_confidence` (and `precision_by_confidence`) so workshop-emit can publish band-level `kpi.harness.recall.*`.

## Joern deferral (N=10)

Do **not** add Joern/CPG adapters until at least **10 accepted ADRs** need cross-file or data-flow evidence that OpenGrep and local AST checks cannot supply. Until then, prefer confidence-gated batch LLM assist + human review for interpretation gaps.

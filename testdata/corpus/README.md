# Architect-labeled corpus (GitHub-native)

Submit additional ADR Markdown under `adrs/` and extend `labels.json` via pull request.

Label fields:
- `statement_contains`: unique marker expected in an extracted candidate
- `modality`, `pattern_id`, `confidence_band`
- `escalate_expected`: true when the candidate should land in the low-confidence LLM assist band

## Run

```bash
go run ./cmd/adrctl ingest-adrs --root testdata/corpus/adrs --json-out /tmp/corpus-ingested.json
go run ./cmd/adrctl extract-obligations --index /tmp/corpus-ingested.json --root testdata/corpus/adrs --json-out /tmp/corpus-candidates.json --min-confidence-report
python3 scripts/reuse_pilot_harness.py testdata/corpus/labels.json /tmp/corpus-candidates.json
```

Default CI always runs the synthetic reuse-pilot. The labeled corpus workflow is opt-in via `workflow_dispatch` or path filters.

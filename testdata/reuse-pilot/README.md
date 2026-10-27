# ADR reuse pilot corpus

Synthetic 120-ADR corpus for Step D / scalable Phase 3 measurement.

## Run

```bash
go run ./cmd/adrctl ingest-adrs --root testdata/reuse-pilot/adrs --json-out /tmp/pilot-ingested.json
go run ./cmd/adrctl extract-obligations --index /tmp/pilot-ingested.json --root testdata/reuse-pilot/adrs --json-out /tmp/pilot-candidates.json
python3 scripts/reuse_pilot_harness.py testdata/reuse-pilot/labels.json /tmp/pilot-candidates.json
```

Replace or extend this corpus with organization ADRs before claiming Phase 5 scale readiness. Thresholds in `labels.json` gate precision/recall for the harness only.

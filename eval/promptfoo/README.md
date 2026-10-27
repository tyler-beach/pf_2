# Promptfoo assist eval

Evaluates Pathfinder's **batch LLM assist** prompts and structured outputs.

**Promptfoo scores are not a compliance verdict.** Runtime `assess` / `assess-repo` / PR gates remain zero-LLM and deterministic.

## Offline (default CI)

```bash
cd eval/promptfoo
npm ci
npm run eval:offline
```

Uses `provider-offline.mjs` (deterministic fixture; no provider API calls).

## Regenerate cases from a real extract

```bash
go run ./cmd/adrctl ingest-adrs --root testdata/corpus/adrs --json-out /tmp/ingested.json
go run ./cmd/adrctl extract-obligations --index /tmp/ingested.json --root testdata/corpus/adrs --json-out /tmp/candidates.json
python3 eval/scripts/regen_promptfoo_cases.py /tmp/candidates.json 8
```

`cases.json` must cite real `source_sha256` / line ranges from extract (fail-closed assist citation checks).

## Live (manual / llm-assist.yml)

Requires `LLM_API_KEY` and optional `LLM_MODEL` / `LLM_ENDPOINT`.

- No key → live Promptfoo is **skipped** (offline remains the gate).
- Key present + `provider=openai-compatible` → live eval **fail-closed** (must pass).

```bash
cd eval/promptfoo
npm ci
npm run eval:live
```

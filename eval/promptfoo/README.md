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

## Live (manual / llm-assist.yml)

Requires `LLM_API_KEY` and optional `LLM_MODEL` / `LLM_ENDPOINT`.

```bash
cd eval/promptfoo
npm ci
npm run eval:live
```

# Pathfinder KPI dictionary

Machine schema: [`schemas/workshop-metrics-v1.json`](../schemas/workshop-metrics-v1.json).  
Emitted by `adrctl workshop-emit` into `metrics.json` and appended to `workshop-data` as `metrics/runs.jsonl`.

**Compliance note:** Promptfoo and assist KPIs measure *batch assist quality*. They are **not** Pathfinder compliance verdicts. Runtime assessment remains deterministic with `runtime_llm_calls == 0`.

| KPI ID | Question | Formula | Unit | Dimensions | Freshness |
|---|---|---|---|---|---|
| `kpi.confidence.share` | How are candidates distributed by confidence? | high/medium/low counts and shares of `candidate_total` | count / ratio | workflow, ref, band | every extract |
| `kpi.escalation.rate` | How often do we escalate to assist? | `confidence_low / candidate_total` | ratio 0–1 | workflow, ref | every extract |
| `kpi.harness.precision` | Are labeled extractions candidates precise? | harness matched / (matched + FP), overall and by band | ratio | corpus, band | corpus-eval / reuse-pilot |
| `kpi.harness.recall` | Do we recover labeled statements? | matched markers / expected markers | ratio | corpus, band | corpus-eval / reuse-pilot |
| `kpi.review.backlog` | How many candidates await human review? | `pending_review_count` | count | workflow | validate-obligation-reviews |
| `kpi.review.pending_by_band` | Where is backlog concentrated? | pending_high / medium / low | count | band | validate-obligation-reviews |
| `kpi.review.accept_rate` | Do humans accept assist proposals? | accepted proposals / proposals reviewed (when labeled) | ratio | provider_mode | review registry vs proposals |
| `kpi.assess.verdict` | Did the deterministic assess pass? | assessment `verdict` | enum | workflow, ref | assess / assess-repo |
| `kpi.assess.coverage` | Binding coverage health | covered / unbound / stale counts | count | workflow | assessment + review |
| `kpi.invariant.llm_calls` | Did assess stay zero-LLM? | `runtime_llm_calls` (must be 0 on assess paths) | count | workflow | assess |
| `kpi.assist.proposal_count` | Assist volume | proposal_count | count | provider_mode, max_confidence | assist-obligations |
| `kpi.promptfoo.pass_rate` | Did assist eval assertions pass? | successes / (successes+failures); **not a compliance verdict** | ratio | offline\|live | Promptfoo |
| `kpi.evidence.gap_rate` | Share needing manual / non-executable patterns | `manual_review` candidates / total | ratio | workflow | extract |
| `kpi.corpus.size` | Labeled corpus scale | labeled ADR count | count | corpus | labels.json |

## SLO tiles (workshop home)

| SLO | Gate |
|---|---|
| Assess runtime LLM invariant | `kpi.invariant.llm_calls == 0` on assess/CI assess paths |
| Offline Promptfoo | `kpi.promptfoo.pass_rate == 1` with `promptfoo_mode=offline` |
| Publish freshness | `manifest.updated_at` within minutes of source workflow |
| High-confidence share | informational trend from confidence mix |

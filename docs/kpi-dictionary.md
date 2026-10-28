# Pathfinder KPI dictionary

Machine schema: [`schemas/workshop-metrics-v1.json`](../schemas/workshop-metrics-v1.json).  
Emitted by `adrctl workshop-emit` into `metrics.json` and appended to `workshop-data` as `metrics/runs.jsonl`.

**Compliance note:** Promptfoo and assist KPIs measure *batch assist quality*. They are **not** Pathfinder compliance verdicts. Runtime assessment remains deterministic with `runtime_llm_calls == 0`.

| KPI ID | Question | Formula | Unit | Dimensions | Freshness |
|---|---|---|---|---|---|
| `kpi.confidence.share` | How are candidates distributed by confidence band? | Emitted as `kpi.confidence.share.{high,medium,low}` = band_count / `candidate_total` (plus raw counts). Bands are conformal when `kpi.confidence.method=platt_conformal`, else heuristic. | count / ratio | workflow, ref, band | every extract |
| `kpi.confidence.share.*.ci_low/ci_high` | Sampling uncertainty on band shares | Wilson 95% score interval on band_count / n | ratio | workflow, ref | every extract |
| `kpi.confidence.method` | Which confidence stack produced bands? | `platt_conformal` or `heuristic` (honest fallback) | enum | workflow | every extract |
| `kpi.confidence.coverage_target` | Conformal target coverage | e.g. 0.90 when conformal active | ratio | workflow | every extract |
| `kpi.confidence.mean_p` | Mean calibrated P(extract_correct) | mean of candidate `confidence_p` | ratio | workflow | every extract |
| `kpi.confidence.calibrated_count` / `heuristic_count` | Method mix | counts of candidates by `confidence_method` | count | workflow | every extract |
| `kpi.escalation.rate` | How often do we escalate to assist? | `confidence_low / candidate_total` (± Wilson `kpi.escalation.rate.ci_low/ci_high`) | ratio 0–1 | workflow, ref | every extract |
| `kpi.escalation.rate.tech_spec` / `code_snippet` | Escalation by submission kind | low_band_count / kind_count among candidates tagged `submission_kind` (± Wilson CIs) | ratio 0–1 | submission_kind | every extract |
| `kpi.ci.level` / `kpi.ci.method` | Interval metadata | typically `0.95` / `wilson` | enum | workflow | every extract |
| `kpi.submission.tech_spec_count` / `code_snippet_count` | Candidate volume by input shape | count of candidates with `submission_kind` | count | submission_kind | every extract |
| `kpi.harness.precision` | Are labeled extractions candidates precise? | harness matched / (matched + FP), overall and by band; Wilson `kpi.harness.precision.ci_*` when counts present | ratio | corpus, band | corpus-eval / reuse-pilot |
| `kpi.harness.recall` | Do we recover labeled statements? | matched markers / expected markers; also `kpi.harness.recall.{high,medium,low}` from harness `recall_by_confidence` | ratio | corpus, band | corpus-eval / reuse-pilot |
| `kpi.review.backlog` | How many candidates await human review? | `pending_review_count` | count | workflow | validate-obligation-reviews |
| `kpi.review.pending_by_band` | Where is backlog concentrated? | pending_high / medium / low | count | band | validate-obligation-reviews |
| `kpi.review.accept_rate` | Do humans accept assist proposals? | approved / (approved + rejected) among proposal parents with a non-pending latest review (`adrctl accept-assist-proposal`) | ratio | provider_mode | review registry vs proposals |
| `kpi.assess.verdict` | Did the deterministic assess pass? | assessment `verdict` | enum | workflow, ref | assess / assess-repo |
| `kpi.assess.submission_kind` | Was this assess run tech-spec prose or code/IaC? | assessment `submission_kind` (`tech_spec` \| `code_snippet` \| empty) | enum | workflow, ref | assess / assess-repo |
| `kpi.assess.coverage` | Binding coverage health | covered / unbound / stale counts | count | workflow | assessment + review |
| `kpi.invariant.llm_calls` | Did assess stay zero-LLM? | `runtime_llm_calls` (must be 0 on assess paths) | count | workflow | assess |
| `kpi.assist.proposal_count` | Assist volume | proposal_count | count | provider_mode, max_confidence | assist-obligations |
| `kpi.assist.prompt_tokens` / `completion_tokens` / `cost_usd` | LLM usage & estimated spend | tokens from provider usage; USD estimate for live (`gpt-4o-mini` list rates); offline = 0 | count / USD | provider, model | assist-obligations |
| `kpi.promptfoo.pass_rate` | Did assist eval assertions pass? | successes / (successes+failures); **not a compliance verdict** | ratio | offline\|live | Promptfoo |
| `kpi.evidence.gap_rate` | Share needing manual / non-executable patterns | `manual_review` candidates / total | ratio | workflow | extract |
| `kpi.corpus.size` | Labeled corpus scale | labeled ADR count | count | corpus | labels.json |
| `kpi.adr.touched` | How many ADRs appear in assessment requirements? | distinct `adr_id` in rolled requirement observations | count | workflow, ref | assess / workshop-emit |
| `kpi.adr.assessed` | How many ADRs have ≥1 applicable (non-N/A) result? | distinct ADR with aligned or deviate observations | count | workflow, ref | assess / workshop-emit |
| `kpi.adr.flag_rate` | How often do applicable ADR checks deviate? | deviate / assessed (± Wilson `kpi.adr.flag_rate.ci_low/ci_high`). **Aligned** = `PASS`. **Deviated** = `FAIL`/`WARN`/`INDETERMINATE`/`EVIDENCE_MISSING`/`ERROR`/`REVIEW_REQUIRED`/`UNSUPPORTED`. **Excluded** = `NOT_APPLICABLE` (not in denominator). | ratio 0–1 | workflow, submission_kind | assess / `--submissions` batch |
| `kpi.adr.top_flag_rate` / `top_flag_rate_adr` | Which ADR flags most? | max per-ADR flag rate + id | ratio / id | adr | assess / workshop-emit |
| `kpi.adr.fail_count` / `pass_count` / `indeterminate_count` | Observation totals | counts of FAIL / PASS / INDETERMINATE requirement results | count | workflow | assess / workshop-emit |
| `kpi.adr.most_fail` / `most_aligned` | Leaderboard anchors | ADR id with most FAIL / most PASS observations | id | adr | assess / workshop-emit |

Static dig-in artifact: `adr_flagging.json` (`adr-flagging-v1`) with per-ADR counts, Wilson CIs, `by_submission_kind`, aligned/deviated run samples, and fired rule ids. Workshop route: `#/adr-flags`.

## SLO tiles (workshop home)

| SLO | Gate |
|---|---|
| Assess runtime LLM invariant | `kpi.invariant.llm_calls == 0` on assess/CI assess paths |
| Offline Promptfoo | `kpi.promptfoo.pass_rate == 1` with `promptfoo_mode=offline` |
| Publish freshness | `manifest.updated_at` within minutes of source workflow |
| High-confidence share | informational trend from confidence mix |

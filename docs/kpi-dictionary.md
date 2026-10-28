# Pathfinder KPI dictionary

Machine schema: [`schemas/workshop-metrics-v1.json`](../schemas/workshop-metrics-v1.json).  
Emitted by `adrctl workshop-emit` into `metrics.json` and appended to `workshop-data` as `metrics/runs.jsonl`.

**Compliance note:** Promptfoo and assist KPIs measure *batch assist quality*. They are **not** Pathfinder compliance verdicts. Runtime assessment remains deterministic with `runtime_llm_calls == 0`.

Workshop UI: every KPI tile has an ℹ control (keyboard/touch) with plain-English help linking to `#/docs/...`.

| KPI ID | Question | Formula | Unit | Dimensions | Freshness |
|---|---|---|---|---|---|
| `kpi.confidence.share` | How are candidates distributed by confidence band? | Emitted as `kpi.confidence.share.{high,medium,low}` = band_count / `candidate_total` (plus raw counts). Bands are conformal when `kpi.confidence.method=platt_conformal`, else heuristic. | count / ratio | workflow, ref, band | every extract |
| `kpi.confidence.share.*.ci_low/ci_high` | Sampling uncertainty on band shares | Wilson 95% score interval on band_count / n (high, **medium**, and low) | ratio | workflow, ref | every extract |
| `kpi.confidence.method` | Which confidence stack produced bands? | `platt_conformal` or `heuristic` (honest fallback) | enum | workflow | every extract |
| `kpi.confidence.coverage_target` | Conformal target coverage | e.g. 0.90 when conformal active | ratio | workflow | every extract |
| `kpi.confidence.mean_p` | Mean calibrated P(extract_correct) | mean of candidate `confidence_p` | ratio | workflow | every extract |
| `kpi.confidence.mean_p.tech_spec` / `code_snippet` | Mean P by submission kind | mean `confidence_p` among candidates of that kind | ratio | submission_kind | every extract |
| `kpi.confidence.calibrated_count` / `heuristic_count` | Method mix | counts of candidates by `confidence_method` | count | workflow | every extract |
| `kpi.confidence.params_version` | Which fitted params file/version? | `candidates.calibration.params_version` | enum | workflow | every extract |
| `kpi.confidence.gold_size` | How large was the gold set at fit? | `candidates.calibration.gold_example_count` | count | workflow | every extract |
| `kpi.confidence.empirical_coverage` | Do prediction sets contain the assigned band? | among candidates with `prediction_set`, share where band ∈ set | ratio | workflow | every extract |
| `kpi.confidence.coverage_gap` | Empirical vs target | `empirical_coverage − coverage_target` | delta | workflow | every extract |
| `kpi.escalation.rate` | How often do we escalate to assist? | `confidence_low / candidate_total` (± Wilson `kpi.escalation.rate.ci_low/ci_high`) | ratio 0–1 | workflow, ref | every extract |
| `kpi.escalation.rate.tech_spec` / `code_snippet` | Escalation by submission kind | low_band_count / kind_count among candidates tagged `submission_kind` (± Wilson CIs) | ratio 0–1 | submission_kind | every extract |
| `kpi.ci.level` / `kpi.ci.method` | Interval metadata | typically `0.95` / `wilson` | enum | workflow | every extract |
| `kpi.submission.tech_spec_count` / `code_snippet_count` | Candidate volume by input shape | count of candidates with `submission_kind` | count | submission_kind | every extract |
| `kpi.submission.unspecified_count` / `unspecified_rate` | Missing kind tags | count / rate of empty `submission_kind` | count / ratio | hygiene | every extract |
| `kpi.harness.precision` | Are labeled extracted candidates precise? | harness matched / (matched + FP); Wilson `kpi.harness.precision.ci_*` when counts present | ratio | corpus | corpus-eval / reuse-pilot |
| `kpi.harness.precision.{high,medium,low}` | Precision by confidence band | from harness `precision_by_confidence` when present | ratio | corpus, band | corpus-eval / reuse-pilot |
| `kpi.harness.precision.tech_spec` / `code_snippet` | Precision by submission kind | from harness `by_submission_kind.*.precision` when present | ratio | corpus, kind | corpus-eval / reuse-pilot |
| `kpi.harness.recall` | Do we recover labeled statements? | matched markers / expected markers; also `kpi.harness.recall.{high,medium,low}` from harness `recall_by_confidence` | ratio | corpus, band | corpus-eval / reuse-pilot |
| `kpi.harness.recall.tech_spec` / `code_snippet` | Recall by submission kind | from harness `by_submission_kind.*.recall` when labels carry kind | ratio | corpus, kind | corpus-eval / reuse-pilot |
| `kpi.harness.false_positives` / `fp_rate` | Extract false-positive proxies | FP count; FP / (matched + FP) | count / ratio | corpus | corpus-eval / reuse-pilot |
| `kpi.review.backlog` | How many candidates await human review? | `pending_review_count` | count | workflow | validate-obligation-reviews |
| `kpi.review.pending_by_band` | Where is backlog concentrated? | pending_high / medium / low | count | band | validate-obligation-reviews |
| `kpi.review.accept_rate` | Do humans accept assist proposals? | approved / (approved + rejected) among proposal parents with a non-pending latest review | ratio | provider_mode | review registry vs proposals |
| `kpi.review.reject_rate` | Do humans reject assist proposals? | rejected / (approved + rejected) among decided proposal parents | ratio | provider_mode | review registry vs proposals |
| `kpi.assess.verdict` | Did the deterministic assess pass? | assessment `verdict` | enum | workflow, ref | assess / assess-repo |
| `kpi.assess.submission_kind` | Was this assess run tech-spec, code/IaC, or a mixed batch? | assessment `submission_kind` (`tech_spec` \| `code_snippet` \| `mixed` \| empty) | enum | workflow, ref | assess / assess-repo / batch |
| `kpi.assess.na_rate` | How often are requirements N/A? | NOT_APPLICABLE / requirement_count | ratio | workflow | assess |
| `kpi.assess.applicable_count` | How many requirements were applicable? | non-N/A count (or `applicable_requirement_count`) | count | workflow | assess |
| `kpi.assess.pass_share` / `fail_share` / `indeterminate_share` / `warn_share` | Outcome mix among applicable checks | status_count / applicable_count | ratio | workflow | assess |
| `kpi.assess.funnel.*` | Binding / corpus coverage funnel | inventory→accepted→interpreted→bound→evidence_complete counts and `/inventory` rates | count / ratio | workflow | assess |
| `kpi.assess.covered` / `unbound` / `stale` | Binding coverage health | covered / unbound / stale counts from obligation coverage | count | workflow | assessment + review |
| `kpi.invariant.llm_calls` | Did assess stay zero-LLM? | `runtime_llm_calls` (must be 0 on assess paths) | count | workflow | assess |
| `kpi.assist.proposal_count` | Assist volume | proposal_count | count | provider_mode, max_confidence | assist-obligations |
| `kpi.assist.prompt_tokens` / `completion_tokens` / `cost_usd` | LLM usage & estimated spend | tokens from provider usage; USD estimate for live (`gpt-4o-mini` list rates); offline = 0 | count / USD | provider, model | assist-obligations |
| `kpi.cost.per_accepted_proposal` | USD per useful assist outcome | `cost_usd` / accepted proposal parents (when registry present) | USD | provider | assist + review |
| `kpi.cost.per_fail` | USD per FAIL observation | `cost_usd` / `kpi.adr.fail_count` when fails > 0 | USD | workflow | assist + assess |
| `kpi.promptfoo.pass_rate` | Did assist eval assertions pass? | successes / (successes+failures); **not a compliance verdict** | ratio | offline\|live | Promptfoo |
| `kpi.evidence.gap_rate` | Share needing manual / non-executable patterns | `manual_review` candidates / total; also `.tech_spec` / `.code_snippet` | ratio | workflow, kind | extract |
| `kpi.corpus.size` | Labeled corpus scale | labeled ADR count | count | corpus | labels.json |
| `kpi.adr.touched` | How many ADRs appear in assessment requirements? | distinct `adr_id` in rolled requirement observations | count | workflow, ref | assess / workshop-emit |
| `kpi.adr.assessed` | How many ADRs have ≥1 applicable (non-N/A) result? | distinct ADR with aligned or deviate observations | count | workflow, ref | assess / workshop-emit |
| `kpi.adr.flag_rate` | How often do applicable ADR checks deviate? | deviate / assessed (± Wilson `kpi.adr.flag_rate.ci_low/ci_high`). **Aligned** = `PASS`. **Deviated** = `FAIL`/`WARN`/`INDETERMINATE`/`EVIDENCE_MISSING`/`ERROR`/`REVIEW_REQUIRED`/`UNSUPPORTED`. **Excluded** = `NOT_APPLICABLE` (not in denominator). | ratio 0–1 | workflow, submission_kind | assess / `--submissions` batch |
| `kpi.adr.top_flag_rate` / `top_flag_rate_adr` | Which ADR flags most? | max per-ADR flag rate + id | ratio / id | adr | assess / workshop-emit |
| `kpi.adr.fail_count` / `pass_count` / `indeterminate_count` | Observation totals | counts of FAIL / PASS / INDETERMINATE requirement results | count | workflow | assess / workshop-emit |
| `kpi.adr.most_fail` / `most_aligned` | Leaderboard anchors | ADR id with most FAIL / most PASS observations | id | adr | assess / workshop-emit |
| `kpi.rule.distinct_count` / `hit_total` / `flag_total` | Analyzer rule activity | unique `rule_id`s; total hits; hits on deviate requirements | count | analyzer | assess / workshop-emit |
| `kpi.rule.top_rule_id` / `top_hit_count` / `top_flag_*` | Hottest rules | argmax hit / flag counts | id / count | rule_id | assess / workshop-emit |
| `kpi.publish.age_minutes` | How old is the run timestamp at emit? | minutes since `run.timestamp`; SLO in `latest.slo.publish_*` | minutes | workflow | workshop-emit |

Static dig-in artifacts:

- `adr_flagging.json` (`adr-flagging-v1`) — per-ADR counts, Wilson CIs, `by_submission_kind`, aligned/deviated run samples, fired rule ids. Route: `#/adr-flags`.
- `rule_hits.json` (`rule-hits-v1`) — per-`rule_id` hit/flag counts. Surfaced on `#/adr-flags` and Metrics · assess.

## SLO tiles (workshop home)

| SLO | Gate |
|---|---|
| Assess runtime LLM invariant | `kpi.invariant.llm_calls == 0` on assess/CI assess paths |
| Offline Promptfoo | `kpi.promptfoo.pass_rate == 1` with `promptfoo_mode=offline` |
| Publish freshness | `latest.slo.publish_freshness_ok` / UI age from `manifest.updated_at` ≤ `publish_freshness_max_age_minutes` (default 180) |
| High-confidence share | informational trend from confidence mix |

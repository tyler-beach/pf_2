/** Local escapes to avoid circular imports with ui.ts. */
function escapeHTML(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function safeDocsHref(href: string): string {
  if (href.startsWith('#/docs/')) return href;
  return '#/docs/kpis';
}

/** Plain-English help for a workshop KPI, with a Docs deep link. */
export type KpiHelpCopy = {
  plainEnglish: string;
  why: string;
  how: string;
  docsHash: string;
};

const D = {
  kpis: '#/docs/kpis',
  confidence: '#/docs/confidence',
  adr: '#/docs/adr-flagging',
  assess: '#/docs/assess-outcomes',
  rules: '#/docs/rule-hits',
  assist: '#/docs/assist',
  corpus: '#/docs/corpus',
  workshop: '#/docs/workshop-data',
  statuses: '#/docs/statuses',
  obligations: '#/docs/obligations',
} as const;

function h(
  plainEnglish: string,
  why: string,
  how: string,
  docsHash: string,
): KpiHelpCopy {
  return { plainEnglish, why, how, docsHash };
}

/** Canonical registry keyed by metric id (and a few display aliases). */
export const KPI_HELP: Record<string, KpiHelpCopy> = {
  candidate_total: h(
    'How many obligation candidates this run extracted.',
    'Volume context for shares, escalation, and assist load.',
    'Count of candidates in the extract index.',
    D.confidence,
  ),
  confidence_high: h(
    'Candidates in the high confidence band.',
    'High band usually skips assist; watch mix vs low.',
    'Count where confidence band is high (conformal or heuristic).',
    D.confidence,
  ),
  confidence_medium: h(
    'Candidates in the medium confidence band.',
    'Middle triage bucket — neither auto-clear nor always escalated.',
    'Count where confidence band is medium.',
    D.confidence,
  ),
  confidence_low: h(
    'Candidates in the low confidence band.',
    'Primary assist / human-review queue size.',
    'Count where confidence band is low.',
    D.confidence,
  ),
  'kpi.confidence.share.high': h(
    'Share of candidates in the high band.',
    'Tracks whether extract quality is concentrating in confident hits.',
    'confidence_high / candidate_total (± Wilson CI when emitted).',
    D.confidence,
  ),
  'kpi.confidence.share.medium': h(
    'Share of candidates in the medium band.',
    'Detects middle-band pile-ups that hide ambiguity.',
    'confidence_medium / candidate_total (± Wilson CI).',
    D.confidence,
  ),
  'kpi.confidence.share.low': h(
    'Share of candidates in the low band.',
    'Same signal family as escalation rate — low-band fraction.',
    'confidence_low / candidate_total (± Wilson CI).',
    D.confidence,
  ),
  'kpi.escalation.rate': h(
    'How often we escalate to assist / review.',
    'Operational load on humans and LLM assist.',
    'confidence_low / candidate_total (± Wilson CI).',
    D.confidence,
  ),
  'kpi.escalation.rate.tech_spec': h(
    'Escalation rate on English tech-spec / ADR prose candidates.',
    'Compare prose extract difficulty vs code/IaC.',
    'low-band tech_spec candidates / tech_spec count (± CI).',
    D.confidence,
  ),
  'kpi.escalation.rate.code_snippet': h(
    'Escalation rate on code / IaC snippet candidates.',
    'Compare code path ambiguity vs tech-spec prose.',
    'low-band code_snippet candidates / code_snippet count (± CI).',
    D.confidence,
  ),
  'kpi.confidence.method': h(
    'Which confidence stack produced bands this run.',
    'Honest labeling: calibrated vs heuristic fallback.',
    'From candidates.calibration.method (platt_conformal or heuristic).',
    D.confidence,
  ),
  'kpi.confidence.coverage_target': h(
    'Conformal target coverage (e.g. 90%).',
    'The guarantee we aim for under exchangeability with gold.',
    'From calibration coverage / conformal_target_coverage.',
    D.confidence,
  ),
  'kpi.confidence.mean_p': h(
    'Mean calibrated P(extract_correct) across candidates.',
    'Overall extract confidence level — not a compliance score.',
    'Average of candidate confidence_p values.',
    D.confidence,
  ),
  'kpi.confidence.mean_p.tech_spec': h(
    'Mean calibrated P on tech_spec candidates.',
    'Kind-split quality for English ADR/prose extracts.',
    'Average confidence_p among submission_kind=tech_spec.',
    D.confidence,
  ),
  'kpi.confidence.mean_p.code_snippet': h(
    'Mean calibrated P on code_snippet candidates.',
    'Kind-split quality for source/IaC extracts.',
    'Average confidence_p among submission_kind=code_snippet.',
    D.confidence,
  ),
  'kpi.confidence.calibrated_count': h(
    'Candidates scored with Platt+conformal params.',
    'Shows whether calibrated path is actually active.',
    'Count with confidence_method=platt_conformal.',
    D.confidence,
  ),
  'kpi.confidence.heuristic_count': h(
    'Candidates on honest heuristic fallback.',
    'High heuristic share means gold/params are insufficient or missing.',
    'Count with non-platt (heuristic) confidence_method.',
    D.confidence,
  ),
  'kpi.confidence.params_version': h(
    'Version id of the fitted calibration params.',
    'Drift watch: params version should change when you refit.',
    'From candidates.calibration.params_version.',
    D.confidence,
  ),
  'kpi.confidence.gold_size': h(
    'Gold example count used for the fitted params.',
    'Too-small gold → heuristic fallback; trend after corpus growth.',
    'From candidates.calibration.gold_example_count.',
    D.confidence,
  ),
  'kpi.confidence.empirical_coverage': h(
    'Run-level share where assigned band ∈ prediction_set.',
    'Validity check for conformal sets; drops suggest bugs or shift.',
    'Among candidates with a prediction_set, fraction whose band is in the set.',
    D.confidence,
  ),
  'kpi.confidence.coverage_gap': h(
    'Empirical coverage minus coverage target.',
    'Positive/negative gap highlights drift vs the conformal target.',
    'empirical_coverage − coverage_target when both present.',
    D.confidence,
  ),
  'kpi.ci.level': h('Confidence level for Wilson intervals (typically 0.95).', 'Documents interval strength.', 'Fixed metadata on emit.', D.kpis),
  'kpi.ci.method': h('Interval method (wilson).', 'Documents how ±CI whiskers are computed.', 'Fixed metadata on emit.', D.kpis),
  'kpi.harness.precision': h(
    'Labeled extract precision on the harness corpus.',
    'Are extracted candidates actually expected statements?',
    'matched / (matched + false_positives); Wilson CI when counts exist.',
    D.corpus,
  ),
  'kpi.harness.recall': h(
    'Labeled extract recall on the harness corpus.',
    'Do we recover the statements the labels expect?',
    'matched markers / expected markers; band splits when present.',
    D.corpus,
  ),
  'kpi.harness.recall.high': h('Harness recall among high-band expected labels.', 'Band-conditioned recovery.', 'From harness recall_by_confidence.high.', D.corpus),
  'kpi.harness.recall.medium': h('Harness recall among medium-band expected labels.', 'Band-conditioned recovery.', 'From harness recall_by_confidence.medium.', D.corpus),
  'kpi.harness.recall.low': h('Harness recall among low-band expected labels.', 'Band-conditioned recovery.', 'From harness recall_by_confidence.low.', D.corpus),
  'kpi.harness.precision.high': h('Harness precision in the high confidence band.', 'FP pressure by band.', 'From harness precision_by_confidence.high.', D.corpus),
  'kpi.harness.precision.medium': h('Harness precision in the medium confidence band.', 'FP pressure by band.', 'From harness precision_by_confidence.medium.', D.corpus),
  'kpi.harness.precision.low': h('Harness precision in the low confidence band.', 'FP pressure by band.', 'From harness precision_by_confidence.low.', D.corpus),
  'kpi.harness.precision.tech_spec': h('Harness precision on tech_spec candidates.', 'Kind-split extract quality.', 'From harness by_submission_kind.tech_spec.precision.', D.corpus),
  'kpi.harness.precision.code_snippet': h('Harness precision on code_snippet candidates.', 'Kind-split extract quality.', 'From harness by_submission_kind.code_snippet.precision.', D.corpus),
  'kpi.harness.recall.tech_spec': h('Harness recall on tech_spec labels.', 'Kind-split recovery.', 'From harness by_submission_kind.tech_spec.recall.', D.corpus),
  'kpi.harness.recall.code_snippet': h('Harness recall on code_snippet labels.', 'Kind-split recovery.', 'From harness by_submission_kind.code_snippet.recall.', D.corpus),
  'kpi.harness.false_positives': h(
    'Count of harness false-positive candidates.',
    'Proxy for noisy extract / over-generation.',
    'Harness false_positives count (non-exception unmatched candidates).',
    D.corpus,
  ),
  'kpi.harness.fp_rate': h(
    'False-positive rate on the harness.',
    'Share of scored candidates that were unmatched noise.',
    'false_positives / (matched + false_positives).',
    D.corpus,
  ),
  'kpi.review.backlog': h('Candidates still pending human review.', 'Binding / review queue depth.', 'coverage.pending_review_count.', D.obligations),
  'kpi.review.pending_high': h('Pending reviews in the high band.', 'Where backlog concentrates.', 'coverage.pending_high_confidence.', D.obligations),
  'kpi.review.pending_medium': h('Pending reviews in the medium band.', 'Where backlog concentrates.', 'coverage.pending_medium_confidence.', D.obligations),
  'kpi.review.pending_low': h('Pending reviews in the low band.', 'Where backlog concentrates.', 'coverage.pending_low_confidence.', D.obligations),
  'kpi.review.accept_rate': h(
    'Share of assist proposals humans accepted.',
    'Assist usefulness — not assess compliance.',
    'approved / (approved + rejected) among proposal parents with a decision.',
    D.assist,
  ),
  'kpi.review.reject_rate': h(
    'Share of assist proposals humans rejected.',
    'False-positive proxy for assist drafts.',
    'rejected / (approved + rejected) among decided proposal parents.',
    D.assist,
  ),
  'kpi.assess.verdict': h(
    'Deterministic assess rollup verdict for the run.',
    'Primary compliance outcome for the snapshot.',
    'assessment.verdict (PASS / FAIL / INDETERMINATE / …).',
    D.assess,
  ),
  'kpi.assess.submission_kind': h(
    'Input shape for this assess run.',
    'tech_spec vs code_snippet vs mixed batch.',
    'assessment.submission_kind (including mixed for multi-kind batches).',
    D.assess,
  ),
  'kpi.assess.covered': h('Approved / covered obligation bindings.', 'Binding completeness — not “green estate.”', 'coverage.approved_count.', D.obligations),
  'kpi.assess.unbound': h('Obligations still without an executable binding.', 'Gaps in the binding funnel.', 'coverage.unbound_count.', D.obligations),
  'kpi.assess.stale': h('Bindings whose digests no longer match.', 'Forces re-review before enforcement.', 'coverage.stale_count.', D.obligations),
  'kpi.assess.requirement_count': h('Requirements evaluated in the assessment.', 'Denominator for N/A rate.', 'assessment.requirement_count or sum of status counts.', D.assess),
  'kpi.assess.applicable_count': h(
    'Requirements that were applicable (non-N/A).',
    'Outcome shares use this denominator.',
    'assessment.applicable_requirement_count (or total − N/A).',
    D.assess,
  ),
  'kpi.assess.na_rate': h(
    'Share of requirements marked NOT_APPLICABLE.',
    'Issue/sandbox runs often N/A analyzer rules — track instead of mistaking for PASS.',
    'N/A count / requirement_count.',
    D.assess,
  ),
  'kpi.assess.pass_share': h('PASS share among applicable requirements.', 'Clean-check rate when something was actually assessed.', 'PASS / applicable_count.', D.assess),
  'kpi.assess.fail_share': h('FAIL share among applicable requirements.', 'Confirmed deviations.', 'FAIL / applicable_count.', D.assess),
  'kpi.assess.indeterminate_share': h('INDETERMINATE share among applicable requirements.', 'Evidence gaps / unsupported checks.', 'INDETERMINATE / applicable_count.', D.assess),
  'kpi.assess.warn_share': h('WARN share among applicable requirements.', 'Soft deviations.', 'WARN / applicable_count.', D.assess),
  'kpi.assess.funnel.inventory': h('ADRs in the catalog inventory.', 'Top of the binding/coverage funnel.', 'corpus_coverage.inventory_adrs.', D.assess),
  'kpi.assess.funnel.accepted': h('Accepted ADRs in the inventory.', 'Funnel stage after status filter.', 'corpus_coverage.accepted_adrs.', D.assess),
  'kpi.assess.funnel.interpreted': h('Requirements interpreted from accepted ADRs.', 'Interpretation progress.', 'corpus_coverage.interpreted_requirements.', D.assess),
  'kpi.assess.funnel.bound': h('Requirements with a bound verifier.', 'Binding progress.', 'corpus_coverage.bound_requirements.', D.assess),
  'kpi.assess.funnel.evidence_complete': h('Requirements with complete evidence.', 'Bottom of the assess funnel.', 'corpus_coverage.evidence_complete.', D.assess),
  'kpi.assess.funnel.accepted_rate': h('Accepted / inventory.', 'Funnel conversion.', 'accepted_adrs / inventory_adrs.', D.assess),
  'kpi.assess.funnel.interpreted_rate': h('Interpreted / inventory.', 'Funnel conversion.', 'interpreted_requirements / inventory_adrs.', D.assess),
  'kpi.assess.funnel.bound_rate': h('Bound / inventory.', 'Funnel conversion.', 'bound_requirements / inventory_adrs.', D.assess),
  'kpi.assess.funnel.evidence_complete_rate': h('Evidence-complete / inventory.', 'Funnel conversion.', 'evidence_complete / inventory_adrs.', D.assess),
  'kpi.submission.tech_spec_count': h('Candidate volume tagged tech_spec.', 'Kind mix of the extract.', 'Count of candidates with submission_kind=tech_spec.', D.kpis),
  'kpi.submission.code_snippet_count': h('Candidate volume tagged code_snippet.', 'Kind mix of the extract.', 'Count of candidates with submission_kind=code_snippet.', D.kpis),
  'kpi.submission.unspecified_count': h('Candidates missing submission_kind.', 'Hygiene — older or unclassified inputs.', 'Count with empty/unknown submission_kind.', D.kpis),
  'kpi.submission.unspecified_rate': h('Share of candidates missing submission_kind.', 'Hygiene rate for kind tagging.', 'unspecified / candidate_total.', D.kpis),
  'kpi.invariant.llm_calls': h(
    'Runtime LLM calls on the assess path.',
    'Must stay 0 — assess is deterministic.',
    'assessment.runtime_llm_calls.',
    D.assess,
  ),
  'kpi.assist.proposal_count': h('Assist proposals generated this batch.', 'Assist volume (not compliance).', 'proposals.proposal_count.', D.assist),
  'kpi.assist.prompt_tokens': h('Prompt tokens for batch assist.', 'Usage / cost driver.', 'From provider usage on assist-obligations.', D.assist),
  'kpi.assist.completion_tokens': h('Completion tokens for batch assist.', 'Usage / cost driver.', 'From provider usage on assist-obligations.', D.assist),
  'kpi.assist.cost_usd': h(
    'Estimated USD spend for batch assist.',
    'Cost control — not a quality score.',
    'estimated_cost_usd from proposals (0 offline).',
    D.assist,
  ),
  'kpi.assist.provider': h('Assist provider id.', 'Provenance for cost/quality compares.', 'proposals.provider.', D.assist),
  'kpi.assist.model': h('Assist model id.', 'Provenance for cost/quality compares.', 'proposals.model.', D.assist),
  'kpi.cost.per_accepted_proposal': h(
    'USD per human-accepted assist proposal.',
    'Cost per useful assist outcome.',
    'assist cost_usd / accepted proposal parents (when registry present).',
    D.assist,
  ),
  'kpi.cost.per_fail': h(
    'USD per FAIL observation in the ADR rollup.',
    'Rough cost attribution vs confirmed deviations when both exist.',
    'assist cost_usd / kpi.adr.fail_count when fails > 0.',
    D.assist,
  ),
  'kpi.promptfoo.pass_rate': h(
    'Assist Promptfoo assertion pass rate.',
    'Assist regression only — not Pathfinder compliance.',
    'successes / (successes + failures) from promptfoo summary.',
    D.assist,
  ),
  promptfoo_mode: h('Promptfoo mode (offline vs live).', 'Offline should stay green in CI.', 'From promptfoo payload mode.', D.assist),
  'kpi.evidence.gap_rate': h(
    'Share of candidates needing manual_review patterns.',
    'Non-executable / gap pressure on the extract.',
    'manual_review pattern_hint count / candidate_total.',
    D.kpis,
  ),
  'kpi.evidence.gap_rate.tech_spec': h('manual_review share among tech_spec candidates.', 'Kind-split evidence gaps.', 'manual_review tech_spec / tech_spec count.', D.kpis),
  'kpi.evidence.gap_rate.code_snippet': h('manual_review share among code_snippet candidates.', 'Kind-split evidence gaps.', 'manual_review code_snippet / code_snippet count.', D.kpis),
  'kpi.corpus.size': h('Labeled corpus scale.', 'Gold/harness denominator context.', 'labels corpus_size or expected_candidates length.', D.corpus),
  'kpi.adr.touched': h('Distinct ADRs appearing in requirement observations.', 'Breadth of ADR surface in the rollup.', 'From adr_flagging summary.', D.adr),
  'kpi.adr.assessed': h('ADRs with ≥1 applicable (non-N/A) observation.', 'ADRs that entered the flag-rate denominator.', 'From adr_flagging summary.', D.adr),
  'kpi.adr.flag_rate': h(
    'Deviate share among assessed requirement observations.',
    'How often applicable ADR checks deviate.',
    'deviated / assessed (± Wilson CI). N/A excluded.',
    D.adr,
  ),
  'kpi.adr.top_flag_rate': h('Highest per-ADR flag rate.', 'Hotspot ADR.', 'max flag_rate across ADRs with assessed > 0.', D.adr),
  'kpi.adr.top_flag_rate_adr': h('ADR id with the highest flag rate.', 'Hotspot identity.', 'Argmax of per-ADR flag_rate.', D.adr),
  'kpi.adr.fail_count': h('FAIL observation count.', 'Hard deviations.', 'Count of FAIL statuses in the rollup.', D.adr),
  'kpi.adr.pass_count': h('PASS (aligned) observation count.', 'Clean checks.', 'Count of PASS statuses.', D.adr),
  'kpi.adr.indeterminate_count': h('INDETERMINATE observation count.', 'Evidence / support gaps.', 'Count of INDETERMINATE statuses.', D.adr),
  'kpi.adr.most_fail': h('ADR with the most FAIL observations.', 'Leaderboard anchor.', 'Argmax FAIL count.', D.adr),
  'kpi.adr.most_aligned': h('ADR with the most PASS observations.', 'Leaderboard anchor.', 'Argmax PASS count.', D.adr),
  'kpi.rule.distinct_count': h('Distinct analyzer rule_ids that fired.', 'Rule surface breadth.', 'Unique rule_id in findings → rule_hits.json.', D.rules),
  'kpi.rule.hit_total': h('Total rule finding hits.', 'Analyzer activity volume.', 'Sum of hit_count across rules.', D.rules),
  'kpi.rule.flag_total': h('Hits on requirements that also deviated.', 'Rule hits that coincided with deviate statuses.', 'Sum of flag_count across rules.', D.rules),
  'kpi.rule.top_rule_id': h('rule_id with the most hits.', 'Hottest analyzer rule.', 'Argmax hit_count.', D.rules),
  'kpi.rule.top_hit_count': h('Hit count for the top rule.', 'Magnitude of the hottest rule.', 'max hit_count.', D.rules),
  'kpi.rule.top_flag_rule_id': h('rule_id with the most deviate-linked hits.', 'Rule most associated with flags.', 'Argmax flag_count.', D.rules),
  'kpi.rule.top_flag_count': h('Flag count for the top flagging rule.', 'Magnitude of deviate-linked hits.', 'max flag_count.', D.rules),
  'kpi.publish.age_minutes': h(
    'Age of the run timestamp at emit (minutes).',
    'Publish freshness SLO input; UI also recomputes from manifest.updated_at.',
    'now − run.timestamp; ok when ≤ publish_freshness_max_age_minutes (180).',
    D.workshop,
  ),
};

/** Resolve help for a kpi id, stripping .ci_low/.ci_high suffixes when needed. */
export function lookupKpiHelp(kpiId?: string | null): KpiHelpCopy | undefined {
  if (!kpiId) return undefined;
  const id = kpiId.trim();
  if (KPI_HELP[id]) return KPI_HELP[id];
  const stripped = id.replace(/\.ci_(low|high)$/, '');
  if (stripped !== id && KPI_HELP[stripped]) return KPI_HELP[stripped];
  // Prefix fallbacks for related keys
  if (id.startsWith('kpi.harness.precision')) return KPI_HELP['kpi.harness.precision'];
  if (id.startsWith('kpi.harness.recall')) return KPI_HELP['kpi.harness.recall'];
  if (id.startsWith('kpi.escalation.rate')) return KPI_HELP['kpi.escalation.rate'];
  if (id.startsWith('kpi.confidence.share')) return KPI_HELP['kpi.confidence.share.high'];
  if (id.startsWith('kpi.assess.funnel')) return KPI_HELP['kpi.assess.funnel.bound_rate'];
  if (id.startsWith('kpi.adr.')) return KPI_HELP['kpi.adr.flag_rate'];
  if (id.startsWith('kpi.rule.')) return KPI_HELP['kpi.rule.hit_total'];
  return undefined;
}

let kpiHelpSeq = 0;

/** Inline ℹ control for a KPI label (keyboard + touch; not hover-only). */
export function kpiHelpControl(kpiId: string, labelForAria?: string): string {
  const help = lookupKpiHelp(kpiId);
  if (!help) return '';
  const id = `kpi-help-${++kpiHelpSeq}`;
  const title = labelForAria || kpiId;
  const docs = safeDocsHref(help.docsHash);
  return `<span class="kpi-help">
    <button type="button" class="kpi-help-btn chart-help-btn" aria-expanded="false" aria-controls="${id}" title="About ${escapeHTML(title)}" aria-label="About ${escapeHTML(title)}">ℹ</button>
    <div class="kpi-help-pop chart-help-pop" id="${id}" role="region" aria-label="About ${escapeHTML(title)}" hidden data-kpi="${escapeHTML(kpiId)}">
      <p><span class="chart-help-k">What</span> ${escapeHTML(help.plainEnglish)}</p>
      <p><span class="chart-help-k">Why</span> ${escapeHTML(help.why)}</p>
      <p><span class="chart-help-k">How</span> ${escapeHTML(help.how)}</p>
      <p class="kpi-help-docs"><a href="${escapeHTML(docs)}">Docs</a> · <code>${escapeHTML(kpiId)}</code></p>
    </div>
  </span>`;
}

/** Label HTML with optional kpi help control. */
export function kpiLabel(label: string, kpiId?: string): string {
  if (!kpiId || !lookupKpiHelp(kpiId)) return escapeHTML(label);
  const text = label ? `<span class="kpi-label-text">${escapeHTML(label)}</span>` : '';
  return `${text}${kpiHelpControl(kpiId, label || kpiId)}`;
}

/** Table header cell with kpi help. */
export function kpiHelpTh(label: string, kpiId: string, extraClass = ''): string {
  const cls = extraClass ? ` class="${escapeHTML(extraClass)}"` : '';
  return `<th${cls}><span class="kpi-th">${kpiLabel(label, kpiId)}</span></th>`;
}

/** Wire KPI ℹ controls (shares dismiss behavior with chart help). */
export function bindKpiHelp(root: ParentNode = document): void {
  root.querySelectorAll<HTMLButtonElement>('.kpi-help-btn').forEach((btn) => {
    if (btn.dataset.bound === '1') return;
    btn.dataset.bound = '1';
    const pop = btn.parentElement?.querySelector<HTMLElement>('.kpi-help-pop');
    if (!pop) return;
    const close = () => {
      pop.hidden = true;
      btn.setAttribute('aria-expanded', 'false');
    };
    const open = () => {
      root.querySelectorAll<HTMLElement>('.kpi-help-pop:not([hidden]), .chart-help-pop:not([hidden])').forEach((other) => {
        if (other === pop) return;
        other.hidden = true;
        const ob = other.parentElement?.querySelector<HTMLButtonElement>('.kpi-help-btn, .chart-help-btn');
        ob?.setAttribute('aria-expanded', 'false');
      });
      pop.hidden = false;
      btn.setAttribute('aria-expanded', 'true');
    };
    btn.addEventListener('click', (e) => {
      e.preventDefault();
      e.stopPropagation();
      if (pop.hidden) open();
      else close();
    });
    btn.addEventListener('keydown', (e) => {
      if (e.key === 'Escape' && !pop.hidden) {
        e.preventDefault();
        close();
      }
    });
  });
  const doc = root instanceof Document ? root : root.ownerDocument || document;
  if ((doc as Document & { __pfKpiHelpDoc?: boolean }).__pfKpiHelpDoc) return;
  (doc as Document & { __pfKpiHelpDoc?: boolean }).__pfKpiHelpDoc = true;
  doc.addEventListener('click', (e) => {
    const t = e.target as HTMLElement | null;
    if (t?.closest?.('.kpi-help, .chart-head')) return;
    doc.querySelectorAll<HTMLElement>('.kpi-help-pop:not([hidden])').forEach((pop) => {
      pop.hidden = true;
      pop.parentElement?.querySelector<HTMLButtonElement>('.kpi-help-btn')?.setAttribute('aria-expanded', 'false');
    });
  });
  doc.addEventListener('keydown', (e) => {
    if (e.key !== 'Escape') return;
    doc.querySelectorAll<HTMLElement>('.kpi-help-pop:not([hidden])').forEach((pop) => {
      pop.hidden = true;
      pop.parentElement?.querySelector<HTMLButtonElement>('.kpi-help-btn')?.setAttribute('aria-expanded', 'false');
    });
  });
}

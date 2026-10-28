import {
  dataAge,
  loadActiveContext,
  loadADRFlagging,
  loadLinkJSON,
  loadRuleHits,
  type ADRFlaggingReport,
  type ADRFlaggingRow,
} from './data';
import { getQueryValue, readListQuery, writeListQuery } from './hash';
import {
  bindChartHelp,
  bindCopyButtons,
  bindExpandControls,
  bindPager,
  bindSlashSearch,
  bindSortHeaders,
  bindTableKeyboard,
  chartHeading,
  copyWithButtonFeedback,
  drawerBlock,
  drawerField,
  escapeHTML,
  expandControl,
  filterEmpty,
  fmt,
  metricsRow,
  openDrawer,
  renderPager,
  setCrumb,
  setPurpose,
  setTopActions,
  slicePage,
  sortBy,
  sortHeader,
  staleBanner,
  tag,
  toTSV,
  type ChartHelpCopy,
} from './ui';
import uPlot from 'uplot';

const HELP = {
  flagRate: {
    measures: 'kpi.adr.flag_rate = deviate / assessed across requirement observations (Wilson 95% CI when present).',
    use: 'Assessed excludes NOT_APPLICABLE. Pair with top flag-rate ADR and FAIL counts.',
    good: 'Downward or stable-low when evidence quality improves; all-N/A batches show no rate.',
  },
  statusMix: {
    measures: 'Per-ADR status counts rolled into PASS / FAIL / INDETERMINATE / NOT_APPLICABLE / other.',
    use: 'Top N by flag rate (or FAIL). Hover legend for series; X is rank in the active leaderboard.',
    good: 'PASS dominating assessed volume; FAIL and INDETERMINATE shrinking.',
  },
  kindSplit: {
    measures: 'Deviate observations by submission_kind (tech_spec vs code_snippet) for the selected board.',
    use: 'Compare whether prose or code/IaC drives flags for top ADRs.',
    good: 'Neither kind silently absorbs ambiguity; spikes warrant catalog/rule review.',
  },
} as const satisfies Record<string, ChartHelpCopy>;

function num(v: unknown): number | null {
  if (v == null || v === '') return null;
  const n = Number(v);
  return Number.isFinite(n) ? n : null;
}

function fmtCI(est: number | null, lo: number | null, hi: number | null): string {
  if (est == null) return '—';
  if (lo == null || hi == null) return fmt(est);
  return `${fmt(est)} [${fmt(lo)}–${fmt(hi)}]`;
}

function fmtRate(row: ADRFlaggingRow): string {
  return fmtCI(num(row.flag_rate), num(row.flag_rate_ci_low), num(row.flag_rate_ci_high));
}

function chart(el: HTMLElement, series: Array<{ label: string; stroke: string; data: number[] }>, xs: number[]) {
  el.innerHTML = '';
  if (!xs.length) {
    el.innerHTML = `<p class="muted">No chart data yet.</p>`;
    return;
  }
  const data: uPlot.AlignedData = [xs, ...series.map((s) => s.data)];
  // eslint-disable-next-line no-new
  new uPlot(
    {
      width: Math.max(320, el.clientWidth || 640),
      height: el.classList.contains('tall') ? 260 : 220,
      series: [{}, ...series.map((s) => ({ label: s.label, stroke: s.stroke, width: 2 }))],
      axes: [{ stroke: '#8b919a' }, { stroke: '#8b919a' }],
      legend: { live: true },
    },
    data,
    el,
  );
}

function byId(report: ADRFlaggingReport): Map<string, ADRFlaggingRow> {
  return new Map(report.adrs.map((a) => [a.adr_id, a]));
}

function boardRows(report: ADRFlaggingReport, board: string): ADRFlaggingRow[] {
  const map = byId(report);
  const ids =
    board === 'least'
      ? report.leaderboards.least_flagging
      : board === 'fail'
        ? report.leaderboards.most_fail
        : board === 'indeterminate'
          ? report.leaderboards.most_indeterminate
          : board === 'aligned'
            ? report.leaderboards.most_aligned
            : report.leaderboards.most_flagging;
  const rows = ids.map((id) => map.get(id)).filter((r): r is ADRFlaggingRow => !!r);
  if (rows.length) return rows;
  // Fallback when boards empty (e.g. all NOT_APPLICABLE): show ADRs by excluded volume.
  return [...report.adrs].sort((a, b) => b.excluded - a.excluded || a.adr_id.localeCompare(b.adr_id)).slice(0, 25);
}

export function adrFlaggingDigInHTML(row: ADRFlaggingRow): string {
  const defs = `
    <p class="muted" style="margin-top:0">
      <strong>Aligned</strong> = PASS ·
      <strong>Deviated</strong> = FAIL / WARN / INDETERMINATE / EVIDENCE_MISSING / ERROR / REVIEW_REQUIRED / UNSUPPORTED ·
      <strong>Excluded</strong> = NOT_APPLICABLE (not in flag rate) ·
      <strong>Flag rate</strong> = deviate / assessed.
    </p>`;
  const aligned = row.aligned_runs || [];
  const deviate = row.deviated_runs || [];
  const rules = row.rules || [];
  const obsRow = (o: NonNullable<ADRFlaggingRow['aligned_runs']>[number]) => {
    const run = o.submission_id || o.run_id || '—';
    const kind = o.submission_kind || '';
    return `<tr>
      <td><code class="id">${escapeHTML(run)}</code></td>
      <td>${tag(o.status, o.alignment === 'aligned' ? 'ok' : 'warn')}</td>
      <td><code class="id">${escapeHTML(o.requirement_id)}</code></td>
      <td>${escapeHTML(kind || '—')}</td>
      <td class="truncate" title="${escapeHTML(o.reason || '')}">${escapeHTML(o.reason || '—')}</td>
      <td class="entity-links">
        <a href="#/assess?q=${encodeURIComponent(o.requirement_id)}">Assess</a>
        <a href="#/submissions?q=${encodeURIComponent(o.submission_id || '')}">Submission</a>
        <a href="#/runs">Runs</a>
      </td>
    </tr>`;
  };
  const table = (title: string, rows: typeof aligned, empty: string) => `
    <h3>${escapeHTML(title)} (${rows.length})</h3>
    ${
      rows.length
        ? `<div class="table-wrap sticky-head"><table class="dense"><thead><tr>
            <th>Run / submission</th><th>Status</th><th>Requirement</th><th>Kind</th><th>Reason</th><th>Links</th>
          </tr></thead><tbody>${rows.map(obsRow).join('')}</tbody></table></div>`
        : `<p class="muted">${escapeHTML(empty)}</p>`
    }`;
  const rulesHTML = rules.length
    ? `<div class="table-wrap"><table class="dense"><thead><tr><th>Analyzer</th><th>Rule</th><th>Pattern</th><th>Path</th></tr></thead><tbody>
        ${rules
          .map(
            (r) => `<tr>
              <td>${escapeHTML(r.analyzer || '—')}</td>
              <td><code class="id">${escapeHTML(r.rule_id || '—')}</code></td>
              <td><code>${escapeHTML(r.pattern || '—')}</code></td>
              <td class="truncate">${escapeHTML(r.path || '—')}</td>
            </tr>`,
          )
          .join('')}
      </tbody></table></div>`
    : `<p class="muted">No OpenGrep/Checkov findings stored for this ADR in the rollup (common when statuses are NOT_APPLICABLE).</p>`;
  const kindRows = Object.entries(row.by_submission_kind || {})
    .sort(([a], [b]) => a.localeCompare(b))
    .map(
      ([k, v]) => `<tr>
        <td>${escapeHTML(k)}</td>
        <td class="num">${v.aligned}</td>
        <td class="num">${v.deviated}</td>
        <td class="num">${v.excluded}</td>
        <td class="num">${fmtRate({ ...row, flag_rate: v.flag_rate, flag_rate_ci_low: v.flag_rate_ci_low, flag_rate_ci_high: v.flag_rate_ci_high })}</td>
      </tr>`,
    )
    .join('');
  return `
    ${defs}
    ${metricsRow([
      { label: 'Flag rate ±', value: fmtRate(row), cls: row.deviated ? 'warn' : 'ok' },
      { label: 'Aligned', value: fmt(row.aligned), cls: 'ok' },
      { label: 'Deviated', value: fmt(row.deviated), cls: row.deviated ? 'warn' : '' },
      { label: 'Excluded (N/A)', value: fmt(row.excluded) },
      { label: 'FAIL', value: fmt(row.fail_count), cls: row.fail_count ? 'bad' : '' },
      { label: 'INDETERMINATE', value: fmt(row.indeterminate_count), cls: row.indeterminate_count ? 'warn' : '' },
    ])}
    <div class="entity-links" style="margin:0.75rem 0">
      <a href="#/adrs/${encodeURIComponent(row.adr_id)}">ADR detail</a>
      <a href="#/assess?q=${encodeURIComponent(row.adr_id)}">Assess filter</a>
      <a href="#/candidates?q=${encodeURIComponent(row.adr_id)}">Candidates</a>
      <a href="#/docs/adr-flagging">Definitions</a>
    </div>
    ${
      kindRows
        ? `<h3>By submission kind</h3>
      <div class="table-wrap"><table class="dense"><thead><tr><th>Kind</th><th>Aligned</th><th>Deviated</th><th>Excluded</th><th>Flag rate</th></tr></thead>
      <tbody>${kindRows}</tbody></table></div>`
        : ''
    }
    ${table('Aligned runs', aligned, 'No PASS observations stored for this ADR.')}
    ${table('Deviated runs', deviate, 'No deviate observations stored for this ADR.')}
    <h3>Rules / analyzers</h3>
    ${rulesHTML}
  `;
}

export function openADRFlaggingDrawer(row: ADRFlaggingRow) {
  openDrawer({
    title: row.adr_id,
    meta: row.title || 'ADR flagging',
    wide: true,
    bodyHTML: adrFlaggingDigInHTML(row),
  });
  bindCopyButtons(document.getElementById('drawer-root') || document);
}

/** Client-side fallback when emit artifact is absent but assessment has requirements. */
export function aggregateFlaggingFromAssessment(
  runId: string,
  assessment: {
    submission_kind?: string;
    requirements?: Array<{
      adr_id?: string;
      title?: string;
      requirement_id?: string;
      status?: string;
      reason?: string;
      findings?: Array<{ rule_id?: string; analyzer?: string; path?: string; message?: string }>;
    }>;
  } | null,
): ADRFlaggingReport | null {
  const reqs = assessment?.requirements || [];
  if (!reqs.length) return null;
  // Lightweight client mirror of Go ClassifyRequirementStatus + counts (no Wilson).
  const alignedStatuses = new Set(['PASS']);
  const excludedStatuses = new Set(['NOT_APPLICABLE', '']);
  type Acc = ADRFlaggingRow & { _rules: Map<string, NonNullable<ADRFlaggingRow['rules']>[number]> };
  const by = new Map<string, Acc>();
  for (const r of reqs) {
    const id = (r.adr_id || '').trim();
    if (!id) continue;
    const status = (r.status || 'UNKNOWN').toUpperCase();
    let alignment: 'aligned' | 'deviated' | 'excluded' = 'deviated';
    if (alignedStatuses.has(status)) alignment = 'aligned';
    else if (excludedStatuses.has(status)) alignment = 'excluded';
    let row = by.get(id);
    if (!row) {
      row = {
        adr_id: id,
        title: r.title || '',
        counts: {},
        assessed: 0,
        aligned: 0,
        deviated: 0,
        excluded: 0,
        fail_count: 0,
        indeterminate_count: 0,
        aligned_runs: [],
        deviated_runs: [],
        rules: [],
        run_ids: runId ? [runId] : [],
        _rules: new Map(),
      };
      by.set(id, row);
    }
    row.counts[status] = (row.counts[status] || 0) + 1;
    if (alignment === 'aligned') {
      row.aligned++;
      row.assessed++;
    } else if (alignment === 'deviated') {
      row.deviated++;
      row.assessed++;
    } else row.excluded++;
    if (status === 'FAIL') row.fail_count++;
    if (status === 'INDETERMINATE') row.indeterminate_count++;
    const obs = {
      run_id: runId,
      submission_kind: assessment?.submission_kind,
      requirement_id: r.requirement_id || '',
      status,
      alignment,
      title: r.title,
      reason: r.reason,
      rules: (r.findings || []).map((f) => ({
        rule_id: f.rule_id,
        analyzer: f.analyzer,
        path: f.path,
        message: f.message,
      })),
    };
    if (alignment === 'aligned' && (row.aligned_runs?.length || 0) < 40) row.aligned_runs!.push(obs);
    if (alignment === 'deviated' && (row.deviated_runs?.length || 0) < 40) row.deviated_runs!.push(obs);
    for (const f of r.findings || []) {
      const key = `${f.analyzer || ''}|${f.rule_id || ''}`;
      if (key !== '|' && !row._rules.has(key)) {
        row._rules.set(key, { rule_id: f.rule_id, analyzer: f.analyzer, path: f.path, message: f.message });
      }
    }
  }
  const adrs = [...by.values()].map((r) => {
    if (r.assessed > 0) {
      r.flag_rate = r.deviated / r.assessed;
    }
    r.rules = [...r._rules.values()];
    delete (r as { _rules?: unknown })._rules;
    return r as ADRFlaggingRow;
  });
  adrs.sort((a, b) => a.adr_id.localeCompare(b.adr_id));
  const assessed = adrs.filter((a) => a.assessed > 0);
  const sortIds = (list: ADRFlaggingRow[], cmp: (a: ADRFlaggingRow, b: ADRFlaggingRow) => number) =>
    [...list].sort(cmp).slice(0, 25).map((a) => a.adr_id);
  const flag = (r: ADRFlaggingRow) => r.flag_rate ?? -1;
  return {
    schema_version: 'adr-flagging-v1-client',
    run_id: runId,
    definitions: {
      aligned: 'PASS',
      deviated: 'FAIL/WARN/INDETERMINATE/…',
      excluded: 'NOT_APPLICABLE',
      flag_rate: 'deviated/assessed',
    },
    summary: {
      adr_touched: adrs.length,
      adr_assessed: assessed.length,
      requirement_observations: adrs.reduce((n, a) => n + a.assessed + a.excluded, 0),
      assessed_observations: adrs.reduce((n, a) => n + a.assessed, 0),
      aligned_observations: adrs.reduce((n, a) => n + a.aligned, 0),
      deviated_observations: adrs.reduce((n, a) => n + a.deviated, 0),
      excluded_observations: adrs.reduce((n, a) => n + a.excluded, 0),
      fail_observations: adrs.reduce((n, a) => n + a.fail_count, 0),
      indeterminate_observations: adrs.reduce((n, a) => n + a.indeterminate_count, 0),
      flag_rate:
        adrs.reduce((n, a) => n + a.assessed, 0) > 0
          ? adrs.reduce((n, a) => n + a.deviated, 0) / adrs.reduce((n, a) => n + a.assessed, 0)
          : undefined,
    },
    leaderboards: {
      most_flagging: sortIds(assessed, (a, b) => flag(b) - flag(a) || a.adr_id.localeCompare(b.adr_id)),
      least_flagging: sortIds(assessed, (a, b) => flag(a) - flag(b) || a.adr_id.localeCompare(b.adr_id)),
      most_fail: sortIds(adrs, (a, b) => b.fail_count - a.fail_count || a.adr_id.localeCompare(b.adr_id)),
      most_indeterminate: sortIds(adrs, (a, b) => b.indeterminate_count - a.indeterminate_count || a.adr_id.localeCompare(b.adr_id)),
      most_aligned: sortIds(adrs, (a, b) => b.aligned - a.aligned || a.adr_id.localeCompare(b.adr_id)),
    },
    adrs,
  };
}

export async function renderADRFlags(app: HTMLElement) {
  setCrumb('ADR Flagging');
  setPurpose(
    'Which ADRs flag most/least across assessment requirements. Aligned=PASS; deviate=FAIL/WARN/INDETERMINATE/…; N/A excluded from flag rate.',
  );
  setTopActions(`
    <a class="btn" href="#/docs/adr-flagging">Definitions</a>
    <a class="btn" href="#/adrs">ADRs list</a>
    <a class="btn" href="#/assess">Assess</a>
  `);

  const state = readListQuery({ sort: 'flag_rate', dir: 'desc', band: 'most' });
  const boardParam = getQueryValue('board') || state.band || 'most';
  const [ctx, reportRaw, assessment, ruleHits] = await Promise.all([
    loadActiveContext(),
    loadADRFlagging(),
    loadLinkJSON<{
      submission_kind?: string;
      requirements?: Array<{
        adr_id?: string;
        title?: string;
        requirement_id?: string;
        status?: string;
        reason?: string;
        findings?: Array<{ rule_id?: string; analyzer?: string; path?: string; message?: string }>;
      }>;
    }>('assessment'),
    loadRuleHits(),
  ]);
  const report =
    reportRaw ||
    aggregateFlaggingFromAssessment(ctx.runId || ctx.latest?.run?.run_id || '', assessment);
  const age = dataAge(ctx.manifest?.updated_at || ctx.latest?.run?.timestamp);
  const m = (ctx.latest?.metrics || {}) as Record<string, unknown>;
  const s = report?.summary;

  if (!report) {
    app.innerHTML = `
      ${age.stale ? staleBanner(age.label, 'Publish data may lag.') : ''}
      <div class="empty">
        <h1>No ADR flagging data</h1>
        <p class="muted">Emit <code>adr_flagging.json</code> via <code>adrctl workshop-emit --submissions …</code> or an assessment with requirements.</p>
        <p><a href="#/docs/adr-flagging">Definitions</a> · <a href="#/assess">Assess</a></p>
      </div>`;
    return;
  }

  const boards = [
    ['most', 'Most flagging'],
    ['least', 'Least flagging'],
    ['fail', 'Most FAIL'],
    ['indeterminate', 'Most INDETERMINATE'],
    ['aligned', 'Most aligned'],
  ] as const;

  const allExcluded =
    Number(s?.adr_touched || 0) > 0 &&
    Number(s?.adr_assessed || 0) === 0 &&
    Number(s?.excluded_observations || 0) > 0;
  app.innerHTML = `
    ${age.stale ? staleBanner(age.label, 'ADR flagging rollup may lag until Workshop Publish refreshes.') : ''}
    ${
      allExcluded
        ? `<div class="banner warn" role="status">
            <strong>All observations excluded (N/A).</strong>
            ${fmt(s?.excluded_observations)} requirement rows touched ${fmt(s?.adr_touched)} ADRs but none entered the flag-rate denominator —
            typical for issue-surface / sandbox assessments. Dig into <a href="#/submissions">Submissions</a> or <a href="#/docs/adr-flagging">definitions</a>; do not treat empty boards as “all aligned.”
          </div>`
        : ''
    }
    <p class="run-meta">Active run <code>${escapeHTML(report.run_id || ctx.runId || '—')}</code>
      · schema <code>${escapeHTML(report.schema_version)}</code>
      · <a href="#/docs/adr-flagging">Aligned vs deviate</a>
      · <a href="#/docs/kpis">KPI glossary</a>
    </p>
    <h2>Overview</h2>
    <p class="muted">Requirement observations rolled per ADR. Flag rate uses assessed only (excludes NOT_APPLICABLE).</p>
    ${metricsRow([
      { label: 'ADRs touched', value: fmt(s?.adr_touched ?? m['kpi.adr.touched']), href: '#/adrs', kpi: 'kpi.adr.touched' },
      {
        label: 'Flag rate ±',
        value: fmtCI(
          num(s?.flag_rate ?? m['kpi.adr.flag_rate']),
          num(s?.flag_rate_ci_low ?? m['kpi.adr.flag_rate.ci_low']),
          num(s?.flag_rate_ci_high ?? m['kpi.adr.flag_rate.ci_high']),
        ),
        cls: (s?.flag_rate || 0) > 0 ? 'warn' : 'ok',
        kpi: 'kpi.adr.flag_rate',
      },
      {
        label: 'Top flag rate',
        value: s?.top_flag_rate_adr ? `${s.top_flag_rate_adr} ${fmt(s.top_flag_rate)}` : '—',
        href: s?.top_flag_rate_adr ? `#/adr-flags?q=${encodeURIComponent(s.top_flag_rate_adr)}` : undefined,
        kpi: 'kpi.adr.top_flag_rate',
      },
      {
        label: 'Most FAIL',
        value: s?.most_fail_adr ? `${s.most_fail_adr} (${fmt(s.most_fail_count)})` : '—',
        href: s?.most_fail_adr ? `#/adrs/${encodeURIComponent(s.most_fail_adr)}` : undefined,
        cls: (s?.most_fail_count || 0) > 0 ? 'bad' : '',
        kpi: 'kpi.adr.most_fail',
      },
      {
        label: 'Most aligned',
        value: s?.most_aligned_adr ? `${s.most_aligned_adr} (${fmt(s.most_aligned_count)})` : '—',
        href: s?.most_aligned_adr ? `#/adrs/${encodeURIComponent(s.most_aligned_adr)}` : undefined,
        cls: 'ok',
        kpi: 'kpi.adr.most_aligned',
      },
      { label: 'Assessed ADRs', value: fmt(s?.adr_assessed ?? m['kpi.adr.assessed']), kpi: 'kpi.adr.assessed' },
      { label: 'Rule hits', value: fmt(ruleHits?.summary.hit_total ?? m['kpi.rule.hit_total']), kpi: 'kpi.rule.hit_total', href: '#/docs/rule-hits' },
      { label: 'FAIL obs', value: fmt(s?.fail_observations ?? m['kpi.adr.fail_count']), cls: (s?.fail_observations || 0) ? 'bad' : '', kpi: 'kpi.adr.fail_count' },
    ])}

    <div class="toolbar">
      <div class="seg" id="adrf-board">
        ${boards
          .map(([id, label]) => {
            const active = boardParam === id || (id === 'most' && !boardParam);
            return `<button type="button" data-board="${id}" class="${active ? 'active' : ''}">${escapeHTML(label)}</button>`;
          })
          .join('')}
      </div>
    </div>

    <div class="spark-grid conf-grid">
      <div>${chartHeading('Status mix (leaderboard)', HELP.statusMix)}<div class="chart tall" id="adrf-status-chart"></div></div>
      <div>${chartHeading('Flag rate', HELP.flagRate)}<div class="chart tall" id="adrf-rate-chart"></div></div>
      <div>${chartHeading('Deviate by submission kind', HELP.kindSplit)}<div class="chart tall" id="adrf-kind-chart"></div></div>
    </div>

    <h2>Leaderboard</h2>
    <div class="toolbar">
      <input class="search" id="adrf-q" placeholder="Filter ADR id or title (/)" value="${escapeHTML(state.q)}" />
      <span class="faint" id="adrf-count"></span>
      <button type="button" class="btn" id="adrf-export-tsv">Copy TSV</button>
    </div>
    <div class="table-wrap sticky-head">
      <table class="dense">
        <thead><tr id="adrf-head"></tr></thead>
        <tbody id="adrf-body"></tbody>
      </table>
    </div>
    <div id="adrf-pager"></div>
    <h2>Rule / analyzer hits</h2>
    <p class="muted">OpenGrep/Checkov <code>rule_id</code> hits from findings. Flag count = hits on deviate requirements. Artifact: <code>rule_hits.json</code>.</p>
    ${metricsRow([
      { label: 'Distinct rules', value: fmt(ruleHits?.summary.distinct_rules ?? m['kpi.rule.distinct_count']), kpi: 'kpi.rule.distinct_count' },
      { label: 'Hit total', value: fmt(ruleHits?.summary.hit_total ?? m['kpi.rule.hit_total']), kpi: 'kpi.rule.hit_total' },
      { label: 'Flag total', value: fmt(ruleHits?.summary.flag_total ?? m['kpi.rule.flag_total']), kpi: 'kpi.rule.flag_total' },
      {
        label: 'Top rule',
        value: ruleHits?.summary.top_rule_id
          ? `${ruleHits.summary.top_rule_id} (${fmt(ruleHits.summary.top_hit_count)})`
          : fmt(m['kpi.rule.top_rule_id']),
        kpi: 'kpi.rule.top_rule_id',
      },
    ])}
    <div class="table-wrap sticky-head" style="max-height:320px;overflow:auto">
      <table class="dense">
        <thead>
          <tr>
            <th>rule_id</th><th>analyzer</th><th class="num">hits</th><th class="num">flags</th><th class="num">ADRs</th>
          </tr>
        </thead>
        <tbody>
          ${
            (ruleHits?.rules || [])
              .slice(0, 40)
              .map(
                (r) => `<tr>
              <td><code>${escapeHTML(r.rule_id)}</code></td>
              <td>${escapeHTML(r.analyzer || '—')}</td>
              <td class="num">${fmt(r.hit_count)}</td>
              <td class="num">${fmt(r.flag_count)}</td>
              <td class="num">${fmt(r.adr_count)}</td>
            </tr>`,
              )
              .join('') || `<tr><td colspan="5" class="muted">No rule findings in this snapshot (common on issue-surface N/A batches).</td></tr>`
          }
        </tbody>
      </table>
    </div>

    <details style="margin-top:1rem">
      <summary>Definitions &amp; KPI keys</summary>
      <ul>
        ${Object.entries(report.definitions)
          .map(([k, v]) => `<li><code>${escapeHTML(k)}</code> — ${escapeHTML(v)}</li>`)
          .join('')}
      </ul>
      <pre class="artifact">${escapeHTML(
        [
          'kpi.adr.touched',
          'kpi.adr.assessed',
          'kpi.adr.flag_rate[+ .ci_low/.ci_high]',
          'kpi.adr.top_flag_rate / kpi.adr.top_flag_rate_adr',
          'kpi.adr.fail_count / pass_count / indeterminate_count',
          'kpi.adr.most_fail / most_aligned',
          'kpi.rule.* + artifact: rule_hits.json',
          'artifact: adr_flagging.json',
        ].join('\n'),
      )}</pre>
    </details>
  `;

  bindChartHelp(app);

  let board = (['most', 'least', 'fail', 'indeterminate', 'aligned'] as const).includes(boardParam as never)
    ? boardParam
    : 'most';
  let page = state.page;
  let pageSize = state.pageSize;
  let sort = state.sort || 'flag_rate';
  let dir: 'asc' | 'desc' = state.dir || 'desc';
  let unbindKeys: (() => void) | undefined;
  const tbody = document.getElementById('adrf-body')!;
  const thead = document.getElementById('adrf-head')!;
  const pagerEl = document.getElementById('adrf-pager')!;
  const countEl = document.getElementById('adrf-count');
  const searchEl = document.getElementById('adrf-q') as HTMLInputElement;

  const baseList = () => boardRows(report, board);

  const filtered = () => {
    const q = (searchEl?.value || '').toLowerCase().trim();
    const list = !q
      ? baseList()
      : report.adrs.filter((a) => `${a.adr_id} ${a.title || ''}`.toLowerCase().includes(q));
    return sortBy(list, sort, dir, (a, key) => {
      if (key === 'title') return a.title || '';
      if (key === 'flag_rate') return a.flag_rate ?? -1;
      if (key === 'aligned') return a.aligned;
      if (key === 'deviated') return a.deviated;
      if (key === 'excluded') return a.excluded;
      if (key === 'fail') return a.fail_count;
      if (key === 'indeterminate') return a.indeterminate_count;
      if (key === 'assessed') return a.assessed;
      return a.adr_id;
    });
  };

  const syncHash = (replace = true) =>
    writeListQuery({ q: searchEl.value.trim(), page, pageSize, sort, dir, band: board }, { replace });

  const paintCharts = (rows: ADRFlaggingRow[]) => {
    const top = rows.slice(0, 12);
    const xs = top.map((_, i) => i);
    const statusEl = document.getElementById('adrf-status-chart');
    if (statusEl) {
      chart(
        statusEl,
        [
          { label: 'PASS', stroke: '#3fae7a', data: top.map((r) => r.counts.PASS || 0) },
          { label: 'FAIL', stroke: '#d46565', data: top.map((r) => r.counts.FAIL || 0) },
          { label: 'INDETERMINATE', stroke: '#c9953a', data: top.map((r) => r.counts.INDETERMINATE || 0) },
          { label: 'NOT_APPLICABLE', stroke: '#5c6775', data: top.map((r) => r.counts.NOT_APPLICABLE || 0) },
        ],
        xs,
      );
    }
    const rateEl = document.getElementById('adrf-rate-chart');
    if (rateEl) {
      chart(
        rateEl,
        [{ label: 'flag_rate', stroke: '#c9953a', data: top.map((r) => r.flag_rate ?? 0) }],
        xs,
      );
    }
    const kindEl = document.getElementById('adrf-kind-chart');
    if (kindEl) {
      chart(
        kindEl,
        [
          {
            label: 'tech_spec deviate',
            stroke: '#3fae7a',
            data: top.map((r) => r.by_submission_kind?.tech_spec?.deviated || 0),
          },
          {
            label: 'code_snippet deviate',
            stroke: '#c9953a',
            data: top.map((r) => r.by_submission_kind?.code_snippet?.deviated || 0),
          },
        ],
        xs,
      );
    }
  };

  const paint = () => {
    unbindKeys?.();
    const list = filtered();
    const slice = slicePage(list, page, pageSize);
    page = slice.page;
    pageSize = slice.pageSize;
    if (countEl) countEl.textContent = `${list.length} ADRs`;
    thead.innerHTML = `<th class="col-expand" scope="col"><span class="sr-only">Details</span></th>${sortHeader('ADR', 'id', sort, dir)}${sortHeader('Flag rate', 'flag_rate', sort, dir)}${sortHeader('FAIL', 'fail', sort, dir)}${sortHeader('Aligned', 'aligned', sort, dir)}${sortHeader('Deviated', 'deviated', sort, dir)}${sortHeader('Excluded', 'excluded', sort, dir)}${sortHeader('Assessed', 'assessed', sort, dir)}<th>Title</th>`;
    bindSortHeaders(thead, (key, nextDir) => {
      sort = key;
      dir = nextDir;
      page = 1;
      syncHash();
      paint();
    });
    tbody.innerHTML = slice.items.length
      ? slice.items
          .map((a) => {
            const sev = a.fail_count ? 'bad' : a.deviated ? 'warn' : a.aligned ? 'ok' : '';
            return `<tr data-row data-id="${escapeHTML(a.adr_id)}">
              <td class="col-expand">${expandControl(a.adr_id)}</td>
              <td><a class="id" href="#/adrs/${encodeURIComponent(a.adr_id)}" title="${escapeHTML(a.adr_id)}">${escapeHTML(a.adr_id)}</a></td>
              <td class="num">${fmtRate(a)}</td>
              <td class="num">${fmt(a.fail_count)}</td>
              <td class="num">${fmt(a.aligned)}</td>
              <td class="num">${fmt(a.deviated)}</td>
              <td class="num">${fmt(a.excluded)}</td>
              <td class="num">${fmt(a.assessed)}</td>
              <td class="truncate expand-cell" title="${escapeHTML(a.title || '')}" data-expand="${escapeHTML(a.adr_id)}">${tag(a.assessed ? (a.deviated ? 'deviate' : 'aligned') : 'excluded', sev)} ${escapeHTML(a.title || '—')}</td>
            </tr>`;
          })
          .join('')
      : filterEmpty(report.adrs.length ? 'No ADRs match this filter.' : 'No ADR rows in rollup.');
    pagerEl.innerHTML = renderPager(slice);
    bindPager(pagerEl, () => ({ page, pageSize }), (next) => {
      page = next.page;
      pageSize = next.pageSize;
      syncHash();
      paint();
    });
    bindExpandControls(tbody, (id) => {
      const row = report.adrs.find((a) => a.adr_id === id);
      if (row) openADRFlaggingDrawer(row);
    });
    unbindKeys = bindTableKeyboard(tbody, {
      onActivate: (row) => {
        if (row.dataset.id) location.hash = `#/adrs/${row.dataset.id}`;
      },
      onExpand: (row) => {
        const r = report.adrs.find((a) => a.adr_id === row.dataset.id);
        if (r) openADRFlaggingDrawer(r);
      },
    });
    paintCharts(list);
    syncHash(true);
  };

  document.querySelectorAll('#adrf-board button').forEach((btn) => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('#adrf-board button').forEach((b) => b.classList.remove('active'));
      btn.classList.add('active');
      board = (btn as HTMLElement).dataset.board || 'most';
      page = 1;
      syncHash(false);
      paint();
    });
  });
  searchEl?.addEventListener('input', () => {
    page = 1;
    syncHash(true);
    paint();
  });
  document.getElementById('adrf-export-tsv')?.addEventListener('click', async (e) => {
    const list = filtered();
    await copyWithButtonFeedback(
      e.currentTarget as HTMLButtonElement,
      toTSV(
        ['adr_id', 'title', 'flag_rate', 'fail', 'aligned', 'deviated', 'excluded', 'assessed'],
        list.map((a) => [
          a.adr_id,
          a.title || '',
          a.flag_rate == null ? '' : String(a.flag_rate),
          String(a.fail_count),
          String(a.aligned),
          String(a.deviated),
          String(a.excluded),
          String(a.assessed),
        ]),
      ),
    );
  });
  bindSlashSearch(searchEl);
  paint();
}

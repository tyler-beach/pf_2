import uPlot from 'uplot';
import { marked } from 'marked';
import DOMPurify from 'dompurify';
import { createTwoFilesPatch } from 'diff';
import { html as diff2html } from 'diff2html';
import {
  dataAge,
  githubBlobURL,
  issueURL,
  loadActiveContext,
  loadADR,
  loadADRIndex,
  loadCorpusIndex,
  loadCosts,
  loadLatest,
  loadLinkJSON,
  loadPipeline,
  loadRuns,
  loadSampleJourney,
  previousRun,
  repoBase,
  workflowDispatchURL,
} from './data';
import { DOC_SECTIONS, docsById, docsGroups } from './docs-content';
import { getQueryValue, readListQuery, setQuery, writeListQuery } from './hash';
import { queryParam, setSelectedRunId } from './session';
import {
  bindChartHelp,
  bindCopyButtons,
  bindExpandControls,
  bindPager,
  bindSlashSearch,
  bindSortHeaders,
  bindTableKeyboard,
  buildMetricCompareRows,
  chartHeading,
  compareDeltaCell,
  compareValueCell,
  copyControl,
  copyWithButtonFeedback,
  drawerBlock,
  drawerField,
  emptyState,
  entityLinks,
  escapeHTML,
  expandControl,
  filterEmpty,
  fmt,
  fmtUSD,
  metricsRow,
  mountChartHeading,
  mountMetricsRow,
  openDrawer,
  renderCompareSummaryStrip,
  summarizeMetricCompare,
  renderPager,
  setCrumb,
  setPurpose,
  setTopActions,
  slicePage,
  sortBy,
  sortHeader,
  staleBanner,
  statusItem,
  tag,
  toMarkdownTable,
  toTSV,
  type ChartHelpCopy,
} from './ui';

const CHART_HELP = {
  sparkConfidence: {
    measures: 'Band counts of extracted candidates: confidence_high and confidence_low across recent runs.',
    use: 'X-axis is run index from metrics/runs.jsonl (oldest → newest). Legend shows the latest run; hover the plot for shape.',
    good: 'High rising or staying high, and low falling or staying low — fewer ambiguous candidates to escalate.',
  },
  sparkEscalation: {
    measures: 'kpi.escalation.rate = confidence_low / candidate_total (share flagged for assist or human review).',
    use: 'Compare consecutive runs on the same sparkline scale; drill to Metrics → Escalation for related keys.',
    good: 'Downward or stable-low is generally better — fewer low-confidence escalations.',
  },
  sparkAssistCost: {
    measures: 'kpi.assist.cost_usd — estimated batch LLM spend for assist-obligations (offline fixture is $0).',
    use: 'Read with proposal volume; this is assist usage, not a compliance verdict or assess score.',
    good: 'Context-dependent: $0 offline is expected. Live spend should track intentional assist volume — unexplained spikes warrant review, not “always down.”',
  },
  metricsTrendDefault: {
    measures: 'kpi.escalation.rate and kpi.promptfoo.pass_rate across runs.jsonl (focus tabs change the series).',
    use: 'Pick a Metrics tab to swap series. X-axis is run index oldest → newest; legend shows latest values.',
    good: 'Escalation down or stable-low; Promptfoo pass rate near 1 offline (assist regression only — not a compliance verdict).',
  },
  metricsTrendConfidence: {
    measures: 'Raw confidence band counts: confidence_high / medium / low per run.',
    use: 'Watch mix shifts run to run; pair with share KPIs in the strip above.',
    good: 'High rising or stable-high band; low falling or stable-low.',
  },
  metricsTrendHarness: {
    measures: 'kpi.harness.precision and kpi.harness.recall from corpus-eval / labeled harness.',
    use: 'Track extract quality on labeled ADRs; not the same as assess verdicts.',
    good: 'Both trending up or holding high — fewer false positives and missed labeled statements.',
  },
  metricsTrendAssist: {
    measures: 'kpi.assist.proposal_count and kpi.assist.cost_usd for batch assist.',
    use: 'Compare volume vs estimated spend; offline cost stays 0.',
    good: 'Context-dependent: cost should follow intentional assist volume. Spikes without more proposals (or live vs offline flips) need a look.',
  },
  costOverRuns: {
    measures: 'kpi.assist.cost_usd and kpi.assist.proposal_count over published runs.',
    use: 'Batch assist usage only. Pair with mode/model tiles; assess runtime LLM must stay 0.',
    good: 'Context-dependent: offline $0 is normal; live spend should match planned assist volume, not a goal to always decrease.',
  },
} as const satisfies Record<string, ChartHelpCopy>;

function chart(el: HTMLElement, series: Array<{ label: string; stroke: string; data: number[] }>, xs: number[]) {
  el.innerHTML = '';
  if (!xs.length) {
    el.innerHTML = `<p class="muted">No trend data yet.</p>`;
    return;
  }
  const width = Math.max(320, Math.min(el.clientWidth || 720, 1040));
  // live:false keeps legend on the latest point (uPlot defaults to "--" until hover).
  new uPlot(
    {
      width,
      height: el.classList.contains('tall') ? 260 : 200,
      scales: { x: { time: false } },
      series: [{}, ...series.map((s) => ({ label: s.label, stroke: s.stroke, width: 1.5 }))],
      axes: [
        {
          stroke: '#5c6775',
          grid: { stroke: '#1a2027' },
          font: '11px IBM Plex Sans',
          values: (_u, splits) => splits.map((v) => String(Math.round(v))),
        },
        { stroke: '#5c6775', grid: { stroke: '#1a2027' }, font: '11px IBM Plex Sans' },
      ],
      legend: { show: true, live: false },
      padding: [8, 12, 0, 8],
    },
    [xs, ...series.map((s) => s.data)],
    el,
  );
}

type TriageItem = { sev: 'bad' | 'warn' | 'ok'; label: string; msg: string; href: string };

function metricDelta(curr: unknown, prev: unknown): { text: string; cls: string } {
  const a = Number(curr);
  const b = Number(prev);
  if (!Number.isFinite(a) || !Number.isFinite(b)) return { text: `${fmt(prev)} → ${fmt(curr)}`, cls: '' };
  const d = a - b;
  if (Math.abs(d) < 1e-9) return { text: `${fmt(prev)} → ${fmt(curr)}`, cls: '' };
  return { text: `${fmt(prev)} → ${fmt(curr)} (${d > 0 ? '+' : ''}${fmt(d)})`, cls: d > 0 ? 'up' : 'down' };
}

export async function renderHome(app: HTMLElement) {
  setCrumb('Overview');
  setPurpose('Triage what needs attention, see what changed, then drill into evidence.');
  setTopActions(`
    <a class="btn" href="#/sample">Sample journey</a>
    <a class="btn primary" href="${workflowDispatchURL('llm-assist.yml')}">Run LLM assist</a>
  `);
  const [ctx, pipeline, candidates, proposals, assessment] = await Promise.all([
    loadActiveContext(),
    loadPipeline(),
    loadLinkJSON<{
      candidates?: Array<{ id: string; adr_id: string; confidence: string; statement: string }>;
      confidence_summary?: { low: number };
    }>('candidates'),
    loadLinkJSON<{ proposals?: Array<{ proposal_id: string; parent_candidate_id: string; approval_status: string }> }>('proposals'),
    loadLinkJSON<{
      verdict?: string;
      runtime_llm_calls?: number;
      requirements?: Array<{ requirement_id: string; status: string; title?: string }>;
    }>('assessment'),
  ]);
  const latest = ctx.latest;
  if (!latest) {
    app.innerHTML = emptyState('No published data', 'Run CI / workshop-publish to populate this view.');
    return;
  }
  const m = latest.metrics;
  const runs = ctx.runs;
  const prev = previousRun(runs, ctx.runId);
  const sloLLM = latest.slo.assess_runtime_llm_calls_zero === true || Number(m['kpi.invariant.llm_calls'] || 0) === 0;
  const sloPF = latest.slo.promptfoo_offline_pass !== false;
  const steps = pipeline?.steps || [];
  const low = (candidates?.candidates || []).filter((c) => c.confidence === 'low');
  const pendingProps = (proposals?.proposals || []).filter((p) => p.approval_status === 'pending_review');
  const failing = (assessment?.requirements || []).filter((r) => r.status === 'FAIL' || r.status === 'REVIEW_REQUIRED');

  const triage: TriageItem[] = [];
  if (!sloLLM || Number(m['kpi.invariant.llm_calls'] || 0) !== 0) {
    triage.push({ sev: 'bad', label: 'Assess', msg: 'runtime_llm_calls invariant broken', href: '#/assess' });
  }
  if (String(m['kpi.assess.verdict'] || assessment?.verdict || '') === 'FAIL') {
    triage.push({ sev: 'bad', label: 'Assess', msg: 'Assessment verdict is FAIL', href: '#/assess' });
  }
  for (const r of failing.slice(0, 5)) {
    triage.push({
      sev: 'warn',
      label: 'Requirement',
      msg: `${r.requirement_id}: ${r.title || r.status}`,
      href: '#/assess',
    });
  }
  if (low.length) {
    triage.push({
      sev: 'warn',
      label: 'Confidence',
      msg: `${low.length} low-confidence candidate(s) need review`,
      href: '#/candidates?band=low',
    });
  }
  if (pendingProps.length) {
    triage.push({
      sev: 'warn',
      label: 'Proposals',
      msg: `${pendingProps.length} assist proposal(s) pending_review`,
      href: '#/proposals?q=pending_review',
    });
  }
  if (Number(m['kpi.assess.stale'] || 0) > 0) {
    triage.push({
      sev: 'warn',
      label: 'Stale',
      msg: `${fmt(m['kpi.assess.stale'])} stale binding(s)`,
      href: '#/assess',
    });
  }
  if (Number(m['kpi.assess.unbound'] || 0) > 0) {
    triage.push({
      sev: 'warn',
      label: 'Unbound',
      msg: `${fmt(m['kpi.assess.unbound'])} unbound obligation(s)`,
      href: '#/assess',
    });
  }
  if (sloPF === false || (m['kpi.promptfoo.pass_rate'] != null && Number(m['kpi.promptfoo.pass_rate']) < 1)) {
    triage.push({ sev: 'warn', label: 'Evals', msg: 'Promptfoo offline pass rate needs attention', href: '#/evals' });
  }
  if (!triage.length) {
    triage.push({ sev: 'ok', label: 'Clear', msg: 'No open triage items for this run', href: '#/metrics' });
  }

  const watchKeys = [
    ['kpi.escalation.rate', 'Escalation'],
    ['confidence_low', 'Low band'],
    ['kpi.assist.cost_usd', 'Assist cost'],
    ['kpi.assist.proposal_count', 'Proposals'],
    ['kpi.promptfoo.pass_rate', 'Promptfoo'],
    ['kpi.assess.verdict', 'Verdict'],
  ] as const;

  const changedRows = prev
    ? watchKeys
        .map(([key, label]) => {
          const cur = m[key];
          const was = prev.metrics[key];
          if (JSON.stringify(cur) === JSON.stringify(was)) return '';
          const delta = metricDelta(cur, was);
          return `<div class="changed-row"><span>${label}</span><span class="${delta.cls}">${escapeHTML(delta.text)}</span><span class="faint">${escapeHTML(prev.run.run_id)}</span></div>`;
        })
        .filter(Boolean)
        .join('')
    : '';

  const age = dataAge(ctx.manifest?.updated_at || latest.run.timestamp);
  const attention = triage.map((t, i) => ({ ...t, rank: i + 1 }));
  app.innerHTML = `
    ${!ctx.artifactsFromSnapshot ? `<div class="banner warn">Metrics for <code>${escapeHTML(ctx.runId)}</code>; detailed artifacts fall back to the latest snapshot.</div>` : ''}
    ${age.stale ? staleBanner(age.label, 'Treat assess/assist figures as historical until Workshop Publish refreshes.') : ''}
    <div class="status-row">
      ${statusItem('Assess LLM', sloLLM ? '0 calls' : 'invariant broken', sloLLM ? 'ok' : 'bad')}
      ${statusItem('Promptfoo', sloPF ? 'offline pass' : 'check', sloPF ? 'ok' : 'warn')}
      ${statusItem('Verdict', String(m['kpi.assess.verdict'] || '—'), m['kpi.assess.verdict'] === 'PASS' ? 'ok' : 'warn')}
      ${statusItem('Assist cost', fmtUSD(m['kpi.assist.cost_usd']), '')}
    </div>
    <p class="run-meta">
      Viewing <code>${escapeHTML(latest.run.run_id)}</code>
      · ${escapeHTML(latest.run.workflow)}
      ${latest.run.timestamp ? `· ${escapeHTML(latest.run.timestamp)}` : ''}
      ${ctx.manifest?.updated_at ? `· published ${escapeHTML(ctx.manifest.updated_at)}` : ''}
    </p>
    ${metricsRow([
      { label: 'Candidates', value: fmt(m.candidate_total), href: '#/candidates' },
      { label: 'High / med / low', value: `${fmt(m.confidence_high)} / ${fmt(m.confidence_medium)} / ${fmt(m.confidence_low)}`, href: '#/candidates?band=low' },
      { label: 'Escalation', value: fmt(m['kpi.escalation.rate']), href: '#/metrics?focus=escalation' },
      { label: 'Proposals', value: fmt(m['kpi.assist.proposal_count']), href: '#/proposals' },
      { label: 'Corpus', value: fmt(m['kpi.corpus.size']), href: '#/corpus' },
    ])}
    <h2>Attention queue</h2>
    <div class="inbox">
      ${attention
        .map(
          (t) => `<a class="inbox-item" href="${escapeHTML(t.href)}">
            <span class="rank">${t.rank}</span>
            <span class="sev ${t.sev}">${escapeHTML(t.label)}</span>
            <span class="msg" title="${escapeHTML(t.msg)}">${escapeHTML(t.msg)}</span>
            <span class="go">Open →</span>
          </a>`,
        )
        .join('')}
    </div>
    <h2>What changed</h2>
    ${
      changedRows
        ? `<div class="changed">${changedRows}</div>`
        : `<p class="muted">${prev ? 'No watched KPI deltas vs previous run.' : 'Need at least two runs in runs.jsonl to diff.'}</p>`
    }
    <h2>Pipeline</h2>
    <div class="steps">
      ${steps
        .map(
          (s, i) =>
            `${i ? '<span class="step-sep">/</span>' : ''}<a class="step-link" href="#/pipeline/${escapeHTML(s.id)}" title="${escapeHTML(s.title)}">${escapeHTML(s.title)}</a>`,
        )
        .join('')}
    </div>
    <h2>Run sparklines</h2>
    <p class="muted" style="margin:0.25rem 0 0.65rem">Recent runs from <code>metrics/runs.jsonl</code> (oldest → newest).</p>
    <div class="spark-grid">
      <div>${chartHeading('Confidence', CHART_HELP.sparkConfidence)}<div id="spark-conf" class="chart"></div></div>
      <div>${chartHeading('Escalation', CHART_HELP.sparkEscalation)}<div id="spark-esc" class="chart"></div></div>
      <div>${chartHeading('Assist cost', CHART_HELP.sparkAssistCost)}<div id="spark-cost" class="chart"></div></div>
    </div>
  `;
  bindChartHelp(app);
  const sparkRuns = runs.slice(-24);
  const xs = sparkRuns.map((_, i) => i);
  const conf = document.getElementById('spark-conf');
  const esc = document.getElementById('spark-esc');
  const cost = document.getElementById('spark-cost');
  if (conf) {
    chart(
      conf,
      [
        { label: 'high', stroke: '#3fae7a', data: sparkRuns.map((r) => Number(r.metrics.confidence_high || 0)) },
        { label: 'low', stroke: '#c9953a', data: sparkRuns.map((r) => Number(r.metrics.confidence_low || 0)) },
      ],
      xs,
    );
  }
  if (esc) {
    chart(esc, [{ label: 'escalation', stroke: '#5b9fd4', data: sparkRuns.map((r) => Number(r.metrics['kpi.escalation.rate'] || 0)) }], xs);
  }
  if (cost) {
    chart(cost, [{ label: 'cost', stroke: '#d45d6a', data: sparkRuns.map((r) => Number(r.metrics['kpi.assist.cost_usd'] || 0)) }], xs);
  }
}

const METRIC_TABS = ['all', 'confidence', 'escalation', 'harness', 'review', 'assist', 'assess'] as const;
type MetricTab = (typeof METRIC_TABS)[number];

function parseMetricFocus(raw: string | null): MetricTab {
  return (METRIC_TABS as readonly string[]).includes(raw || '') ? (raw as MetricTab) : 'all';
}

export async function renderMetrics(app: HTMLElement) {
  setCrumb('Metrics');
  setPurpose('KPI trends for the selected focus. Filter the raw table with / · j/k · Enter copies the value.');
  setTopActions(`<a class="btn" href="#/runs">Compare runs</a>`);
  const listState = readListQuery({ sort: 'key', dir: 'asc' });
  const focus = parseMetricFocus(getQueryValue('focus'));
  const [runs, latest] = await Promise.all([loadRuns(), loadLatest()]);
  const m = latest?.metrics || {};
  const metricRows = Object.keys(m)
    .sort()
    .map((key) => ({ key, value: m[key] }));

  const focusMetrics: Record<MetricTab, Array<{ label: string; value: string; hint?: string; cls?: string }>> = {
    all: [
      { label: 'Escalation', value: fmt(m['kpi.escalation.rate']) },
      { label: 'Precision', value: fmt(m['kpi.harness.precision']) },
      { label: 'Recall', value: fmt(m['kpi.harness.recall']) },
      { label: 'Accept rate', value: fmt(m['kpi.review.accept_rate']) },
      { label: 'Assist cost', value: fmtUSD(m['kpi.assist.cost_usd']) },
      { label: 'Promptfoo', value: fmt(m['kpi.promptfoo.pass_rate']) },
    ],
    confidence: [
      { label: 'High share', value: fmt(m['kpi.confidence.share.high'] ?? (Number(m.candidate_total) ? Number(m.confidence_high) / Number(m.candidate_total) : null)) },
      { label: 'Medium share', value: fmt(m['kpi.confidence.share.medium']) },
      { label: 'Low share', value: fmt(m['kpi.confidence.share.low'] ?? m['kpi.escalation.rate']) },
      { label: 'High count', value: fmt(m.confidence_high) },
      { label: 'Med count', value: fmt(m.confidence_medium) },
      { label: 'Low count', value: fmt(m.confidence_low) },
    ],
    escalation: [
      { label: 'Escalation rate', value: fmt(m['kpi.escalation.rate']) },
      { label: 'Low candidates', value: fmt(m.confidence_low) },
      { label: 'Proposals', value: fmt(m['kpi.assist.proposal_count']) },
      { label: 'Evidence gap', value: fmt(m['kpi.evidence.gap_rate']) },
    ],
    harness: [
      { label: 'Precision', value: fmt(m['kpi.harness.precision']) },
      { label: 'Recall', value: fmt(m['kpi.harness.recall']) },
      { label: 'Recall high', value: fmt(m['kpi.harness.recall.high']) },
      { label: 'Recall med', value: fmt(m['kpi.harness.recall.medium']) },
      { label: 'Recall low', value: fmt(m['kpi.harness.recall.low']) },
      { label: 'Corpus', value: fmt(m['kpi.corpus.size']) },
    ],
    review: [
      { label: 'Backlog', value: fmt(m['kpi.review.backlog']) },
      { label: 'Pending high', value: fmt(m['kpi.review.pending_high']) },
      { label: 'Pending med', value: fmt(m['kpi.review.pending_medium']) },
      { label: 'Pending low', value: fmt(m['kpi.review.pending_low']) },
      { label: 'Accept rate', value: fmt(m['kpi.review.accept_rate']) },
    ],
    assist: [
      { label: 'Proposals', value: fmt(m['kpi.assist.proposal_count']) },
      { label: 'Cost', value: fmtUSD(m['kpi.assist.cost_usd']) },
      { label: 'Prompt tokens', value: fmt(m['kpi.assist.prompt_tokens']) },
      { label: 'Completion', value: fmt(m['kpi.assist.completion_tokens']) },
      { label: 'Provider', value: fmt(m['kpi.assist.provider']) },
      { label: 'Model', value: fmt(m['kpi.assist.model']) },
    ],
    assess: [
      { label: 'Verdict', value: fmt(m['kpi.assess.verdict']) },
      { label: 'LLM calls', value: fmt(m['kpi.invariant.llm_calls']), cls: Number(m['kpi.invariant.llm_calls'] || 0) === 0 ? 'ok' : 'bad' },
      { label: 'Covered', value: fmt(m['kpi.assess.covered']) },
      { label: 'Unbound', value: fmt(m['kpi.assess.unbound']) },
      { label: 'Stale', value: fmt(m['kpi.assess.stale']) },
    ],
  };

  app.replaceChildren();

  const toolbar = document.createElement('div');
  toolbar.className = 'toolbar';
  const tabHost = document.createElement('div');
  tabHost.className = 'seg';
  tabHost.id = 'metric-tabs';
  for (const t of METRIC_TABS) {
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.textContent = t;
    btn.dataset.t = t;
    if (focus === t) btn.className = 'active';
    btn.addEventListener('click', () => {
      setQuery({ focus: t === 'all' ? null : t }, { replace: false });
    });
    tabHost.appendChild(btn);
  }
  toolbar.appendChild(tabHost);
  app.appendChild(toolbar);

  mountMetricsRow(app, focusMetrics[focus]);

  const trendHelp =
    focus === 'confidence'
      ? CHART_HELP.metricsTrendConfidence
      : focus === 'harness'
        ? CHART_HELP.metricsTrendHarness
        : focus === 'assist'
          ? CHART_HELP.metricsTrendAssist
          : CHART_HELP.metricsTrendDefault;
  mountChartHeading(app, 'Trend', trendHelp, 'h2');
  bindChartHelp(app);
  const el = document.createElement('div');
  el.id = 'm-chart';
  el.className = 'chart tall';
  app.appendChild(el);

  const rawHeading = document.createElement('h2');
  rawHeading.textContent = 'Raw latest metrics';
  app.appendChild(rawHeading);

  const listToolbar = document.createElement('div');
  listToolbar.className = 'toolbar';
  listToolbar.innerHTML = `
    <input class="search" id="metric-q" placeholder="Filter metric key or value (/)" value="${escapeHTML(listState.q)}" />
    <span class="faint" id="metric-count"></span>
    <button type="button" class="btn" id="metric-export-tsv">Copy TSV</button>
  `;
  app.appendChild(listToolbar);

  const wrap = document.createElement('div');
  wrap.className = 'table-wrap sticky-head';
  wrap.innerHTML = `<table class="dense"><thead><tr id="metric-head"></tr></thead><tbody id="metric-body"></tbody></table>`;
  app.appendChild(wrap);
  const pagerHost = document.createElement('div');
  pagerHost.id = 'metric-pager';
  app.appendChild(pagerHost);

  let page = listState.page;
  let pageSize = listState.pageSize;
  let sort = listState.sort || 'key';
  let dir: 'asc' | 'desc' = listState.dir;
  let unbindKeys: (() => void) | undefined;
  const tbody = document.getElementById('metric-body')!;
  const thead = document.getElementById('metric-head')!;
  const pagerEl = document.getElementById('metric-pager')!;
  const searchEl = document.getElementById('metric-q') as HTMLInputElement;
  const countEl = document.getElementById('metric-count');

  const filtered = () => {
    const q = (searchEl?.value || '').toLowerCase().trim();
    const list = !q
      ? metricRows
      : metricRows.filter((r) => `${r.key} ${fmt(r.value)}`.toLowerCase().includes(q));
    return sortBy(list, sort, dir, (r, key) => (key === 'value' ? r.value : r.key));
  };

  const syncHash = (replace = true) => {
    writeListQuery({ q: searchEl.value.trim(), page, pageSize, sort, dir }, { replace });
    setQuery({ focus: focus === 'all' ? null : focus }, { replace: true });
  };

  const paint = () => {
    unbindKeys?.();
    const list = filtered();
    const slice = slicePage(list, page, pageSize);
    page = slice.page;
    pageSize = slice.pageSize;
    if (countEl) countEl.textContent = `${list.length} keys`;
    thead.innerHTML = `${sortHeader('Key', 'key', sort, dir)}${sortHeader('Value', 'value', sort, dir)}<th></th>`;
    bindSortHeaders(thead, (key, nextDir) => {
      sort = key;
      dir = nextDir;
      page = 1;
      syncHash();
      paint();
    });
    tbody.innerHTML = slice.items.length
      ? slice.items
          .map((r) => {
            const shown = fmt(r.value);
            return `<tr data-row data-key="${escapeHTML(r.key)}">
              <td><code class="id" title="${escapeHTML(r.key)}">${escapeHTML(r.key)}</code></td>
              <td class="num truncate" title="${escapeHTML(shown)}">${escapeHTML(shown)}</td>
              <td>${copyControl(shown, 'Copy')}</td>
            </tr>`;
          })
          .join('')
      : filterEmpty(metricRows.length ? 'No metrics match this filter.' : 'No metrics in latest.json.');
    pagerEl.innerHTML = renderPager(slice);
    bindCopyButtons(tbody);
    bindPager(pagerEl, () => ({ page, pageSize }), (next) => {
      page = next.page;
      pageSize = next.pageSize;
      syncHash();
      paint();
    });
    unbindKeys = bindTableKeyboard(tbody, {
      onActivate: (row) => {
        row.querySelector<HTMLButtonElement>('[data-copy]')?.click();
      },
    });
    syncHash(true);
  };

  searchEl?.addEventListener('input', () => {
    page = 1;
    syncHash(true);
    paint();
  });
  document.getElementById('metric-export-tsv')?.addEventListener('click', async (e) => {
    const btn = e.currentTarget as HTMLButtonElement;
    const list = filtered();
    await copyWithButtonFeedback(
      btn,
      toTSV(
        ['key', 'value'],
        list.map((r) => [r.key, fmt(r.value)]),
      ),
    );
  });
  bindSlashSearch(searchEl);
  paint();

  if (runs.length) {
    const xs = runs.map((_, i) => i);
    const series =
      focus === 'assist'
        ? [
            { label: 'proposals', stroke: '#5b9fd4', data: runs.map((r) => Number(r.metrics['kpi.assist.proposal_count'] || 0)) },
            { label: 'cost_usd', stroke: '#3fae7a', data: runs.map((r) => Number(r.metrics['kpi.assist.cost_usd'] || 0)) },
          ]
        : focus === 'harness'
          ? [
              { label: 'precision', stroke: '#3fae7a', data: runs.map((r) => Number(r.metrics['kpi.harness.precision'] || 0)) },
              { label: 'recall', stroke: '#c9953a', data: runs.map((r) => Number(r.metrics['kpi.harness.recall'] || 0)) },
            ]
          : focus === 'confidence'
            ? [
                { label: 'high', stroke: '#3fae7a', data: runs.map((r) => Number(r.metrics.confidence_high || 0)) },
                { label: 'medium', stroke: '#5b9fd4', data: runs.map((r) => Number(r.metrics.confidence_medium || 0)) },
                { label: 'low', stroke: '#c9953a', data: runs.map((r) => Number(r.metrics.confidence_low || 0)) },
              ]
            : [
                { label: 'escalation', stroke: '#c9953a', data: runs.map((r) => Number(r.metrics['kpi.escalation.rate'] || 0)) },
                { label: 'promptfoo', stroke: '#5b9fd4', data: runs.map((r) => Number(r.metrics['kpi.promptfoo.pass_rate'] || 0)) },
              ];
    chart(el, series, xs);
  }
}

export async function renderPipeline(app: HTMLElement, stepId?: string) {
  setCrumb('Pipeline');
  setPurpose('One stage at a time: contract, behavior, failure modes, and a sample artifact. ←/→ or j/k move stages.');
  setTopActions(`<a class="btn" href="#/sample">Open Sample journey</a>`);
  const data = await loadPipeline();
  const steps = data?.steps || [];
  const idx = Math.max(0, steps.findIndex((s) => s.id === stepId));
  const active = (stepId ? steps.find((s) => s.id === stepId) : steps[0]) || null;
  if (!active || !steps.length) {
    app.innerHTML = emptyState('No pipeline metadata', 'pipeline.json missing from snapshot.');
    return;
  }
  const prev = idx > 0 ? steps[idx - 1] : null;
  const next = idx < steps.length - 1 ? steps[idx + 1] : null;
  app.innerHTML = `
    <div class="steps" role="navigation" aria-label="Pipeline stages">
      ${steps
        .map(
          (s, i) =>
            `${i ? '<span class="step-sep">/</span>' : ''}<a class="step-link ${active.id === s.id ? 'active' : ''}" href="#/pipeline/${escapeHTML(s.id)}" title="${escapeHTML(s.id)}">${escapeHTML(s.title)}</a>`,
        )
        .join('')}
    </div>
    <p class="run-meta"><code class="id" title="${escapeHTML(active.id)}">${escapeHTML(active.id)}</code> · step ${idx + 1} of ${steps.length}</p>
    <div class="layout-2">
      <section class="layout-aside">
        <h1>${escapeHTML(active.title)}</h1>
        <p class="prose" style="margin-top:0.5rem">${escapeHTML(active.deep.what)}</p>
        ${
          active.command
            ? `<div class="cmd-line" title="${escapeHTML(active.command)}"><code>${escapeHTML(active.command)}</code> ${copyControl(active.command, 'Copy')}</div>`
            : ''
        }
        ${active.invariant ? `<p style="margin-top:0.5rem">${tag(active.invariant, 'ok')}</p>` : ''}
        <h2>How it works</h2>
        <ul>${active.deep.how.map((x) => `<li>${escapeHTML(x)}</li>`).join('')}</ul>
        <h2>Failure modes</h2>
        <ul>${active.deep.failure_modes.map((x) => `<li>${escapeHTML(x)}</li>`).join('')}</ul>
        <h2>Related metrics</h2>
        <p class="muted">${
          active.deep.metrics.length
            ? active.deep.metrics.map((x) => `<a href="#/metrics?q=${encodeURIComponent(x)}"><code class="id">${escapeHTML(x)}</code></a>`).join(' · ')
            : '—'
        }</p>
      </section>
      <aside class="layout-aside">
        <h2>I/O</h2>
        <dl class="kv">
          <dt>Inputs</dt><dd>${active.inputs.map((x) => `<code class="id">${escapeHTML(x)}</code>`).join('<br/>') || '—'}</dd>
          <dt>Outputs</dt><dd>${active.outputs.map((x) => `<code class="id">${escapeHTML(x)}</code>`).join('<br/>') || '—'}</dd>
        </dl>
        <h2>Sample artifact</h2>
        ${
          active.deep.sample_artifact != null
            ? `<div class="artifact"><pre>${escapeHTML(JSON.stringify(active.deep.sample_artifact, null, 2))}</pre></div>`
            : `<p class="muted">No sample artifact for this stage.</p>`
        }
      </aside>
    </div>
    <div class="journey-nav">
      ${prev ? `<a class="btn" href="#/pipeline/${escapeHTML(prev.id)}">← ${escapeHTML(prev.title)}</a>` : `<span></span>`}
      ${next ? `<a class="btn primary" href="#/pipeline/${escapeHTML(next.id)}">${escapeHTML(next.title)} →</a>` : `<span></span>`}
    </div>
  `;
  bindCopyButtons(app);
  const keyFlag = '__pfPipelineKeyHandler' as const;
  const w = window as Window & { [keyFlag]?: (e: KeyboardEvent) => void };
  if (w[keyFlag]) window.removeEventListener('keydown', w[keyFlag]!);
  w[keyFlag] = (e: KeyboardEvent) => {
    if (!location.hash.startsWith('#/pipeline')) return;
    if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement || e.target instanceof HTMLSelectElement) return;
    if ((e.key === 'ArrowLeft' || e.key === 'k') && prev) {
      e.preventDefault();
      location.hash = `#/pipeline/${prev.id}`;
    }
    if ((e.key === 'ArrowRight' || e.key === 'j') && next) {
      e.preventDefault();
      location.hash = `#/pipeline/${next.id}`;
    }
  };
  window.addEventListener('keydown', w[keyFlag]);
}

export async function renderADRs(app: HTMLElement, id?: string) {
  if (id) return renderADRDetail(app, id);
  setCrumb('ADRs');
  setPurpose('Browse published ADR sources. / search · j/k · Enter opens.');
  setTopActions('');
  const state = readListQuery({ sort: 'id', dir: 'asc' });
  const index = await loadADRIndex();
  const adrs = index?.adrs || [];
  app.innerHTML = `
    <div class="toolbar">
      <input class="search" id="adr-q" placeholder="Filter by id or title (/)" value="${escapeHTML(state.q)}" />
      <span class="faint" id="adr-count">${adrs.length} documents</span>
      <button type="button" class="btn" id="adr-export-tsv">Copy TSV</button>
    </div>
    <div class="table-wrap sticky-head">
      <table class="dense">
        <thead><tr id="adr-head"></tr></thead>
        <tbody id="adr-rows"></tbody>
      </table>
    </div>
    <div id="adr-pager"></div>
  `;
  let page = state.page;
  let pageSize = state.pageSize;
  let sort = state.sort || 'id';
  let dir: 'asc' | 'desc' = state.dir;
  let unbindKeys: (() => void) | undefined;
  const tbody = document.getElementById('adr-rows')!;
  const thead = document.getElementById('adr-head')!;
  const pagerEl = document.getElementById('adr-pager')!;
  const countEl = document.getElementById('adr-count');
  const searchEl = document.getElementById('adr-q') as HTMLInputElement;

  const filtered = () => {
    const q = (searchEl?.value || '').toLowerCase().trim();
    const list = !q ? adrs : adrs.filter((a) => `${a.id} ${a.title} ${a.status}`.toLowerCase().includes(q));
    return sortBy(list, sort, dir, (a, key) => {
      if (key === 'title') return a.title;
      if (key === 'status') return a.status;
      if (key === 'candidates') return (a.candidate_ids || []).length;
      return a.id;
    });
  };

  const syncHash = (replace = true) => writeListQuery({ q: searchEl.value.trim(), page, pageSize, sort, dir }, { replace });

  const paint = () => {
    unbindKeys?.();
    const list = filtered();
    const slice = slicePage(list, page, pageSize);
    page = slice.page;
    pageSize = slice.pageSize;
    if (countEl) countEl.textContent = `${list.length} documents`;
    thead.innerHTML = `${sortHeader('ID', 'id', sort, dir)}${sortHeader('Title', 'title', sort, dir)}${sortHeader('Status', 'status', sort, dir)}<th>Mix</th>${sortHeader('Candidates', 'candidates', sort, dir)}`;
    bindSortHeaders(thead, (key, nextDir) => {
      sort = key;
      dir = nextDir;
      page = 1;
      syncHash();
      paint();
    });
    tbody.innerHTML = slice.items.length
      ? slice.items
          .map(
            (a) => `<tr class="row-link" data-row data-id="${escapeHTML(a.id)}">
          <td><code class="id" title="${escapeHTML(a.id)}">${escapeHTML(a.id)}</code></td>
          <td class="truncate" title="${escapeHTML(a.title)}">${escapeHTML(a.title)}</td>
          <td>${tag(a.status, a.status === 'accepted' ? 'ok' : '')}</td>
          <td class="num">${a.confidence_mix ? `${a.confidence_mix.high}/${a.confidence_mix.medium}/${a.confidence_mix.low}` : '—'}</td>
          <td class="num">${(a.candidate_ids || []).length}</td>
        </tr>`,
          )
          .join('')
      : filterEmpty('No ADRs match this filter.');
    pagerEl.innerHTML = renderPager(slice);
    tbody.querySelectorAll<HTMLElement>('tr.row-link').forEach((tr) => {
      tr.addEventListener('click', () => {
        location.hash = `#/adrs/${tr.dataset.id}`;
      });
    });
    bindPager(pagerEl, () => ({ page, pageSize }), (next) => {
      page = next.page;
      pageSize = next.pageSize;
      syncHash();
      paint();
    });
    unbindKeys = bindTableKeyboard(tbody, {
      onActivate: (row) => {
        if (row.dataset.id) location.hash = `#/adrs/${row.dataset.id}`;
      },
    });
    syncHash(true);
  };

  searchEl?.addEventListener('input', () => {
    page = 1;
    syncHash(true);
    paint();
  });
  document.getElementById('adr-export-tsv')?.addEventListener('click', async (e) => {
    const list = filtered();
    await copyWithButtonFeedback(
      e.currentTarget as HTMLButtonElement,
      toTSV(
        ['id', 'title', 'status', 'candidates'],
        list.map((a) => [a.id, a.title, a.status, String((a.candidate_ids || []).length)]),
      ),
    );
  });
  bindSlashSearch(searchEl);
  paint();
}

async function renderADRDetail(app: HTMLElement, id: string) {
  setCrumb(`ADR · ${id}`);
  setPurpose('Source document beside the candidates extracted from it.');
  setTopActions(`<a class="btn" href="#/adrs">All ADRs</a>`);
  const adr = await loadADR(id);
  if (!adr) {
    app.innerHTML = emptyState('ADR not found', id);
    return;
  }
  const html = DOMPurify.sanitize(await marked.parse(adr.body || '_No body_'));
  const cands = await loadLinkJSON<{ candidates?: Array<Record<string, unknown>> }>('candidates');
  const linked = (cands?.candidates || []).filter(
    (c) => String(c.adr_id) === id || (adr.candidate_ids || []).includes(String(c.id)),
  );
  app.innerHTML = `
    <p class="run-meta">${tag(adr.status, 'ok')} · <code>${escapeHTML(adr.path)}</code></p>
    <div class="entity-links">
      <a href="#/candidates?q=${encodeURIComponent(id)}">Candidates</a>
      <a href="#/proposals">Proposals</a>
      <a href="#/assess">Assess</a>
      <a href="#/docs/lifecycle">Docs</a>
      ${copyControl(adr.id, 'Copy ADR id')}
    </div>
    ${metricsRow([
      { label: 'Linked', value: fmt(linked.length) },
      { label: 'High', value: fmt(adr.confidence_mix?.high ?? linked.filter((c) => c.confidence === 'high').length) },
      { label: 'Medium', value: fmt(adr.confidence_mix?.medium ?? linked.filter((c) => c.confidence === 'medium').length) },
      { label: 'Low', value: fmt(adr.confidence_mix?.low ?? linked.filter((c) => c.confidence === 'low').length) },
    ])}
    <div class="layout-2" style="margin-top:1.25rem">
      <article class="prose">${html}</article>
      <aside>
        <h2>Candidates</h2>
        ${
          linked.length
            ? `<div class="table-wrap"><table><thead><tr><th>ID</th><th>Band</th><th>Statement</th></tr></thead><tbody>
            ${linked
              .map(
                (c) =>
                  `<tr><td><code class="id" title="${escapeHTML(String(c.id))}">${escapeHTML(String(c.id))}</code></td><td>${escapeHTML(String(c.confidence))}</td><td class="truncate" title="${escapeHTML(String(c.statement))}">${escapeHTML(String(c.statement))}</td></tr>`,
              )
              .join('')}
          </tbody></table></div>`
            : '<p class="muted">No candidates linked in this snapshot.</p>'
        }
      </aside>
    </div>
  `;
}

export async function renderCandidates(app: HTMLElement) {
  setCrumb('Candidates');
  setPurpose('Filter extracted obligations by confidence band. / search · j/k · Enter opens ADR · o/Space/Details expands.');
  const focusId = queryParam('id');
  const state = readListQuery({ band: 'all', sort: 'confidence', dir: 'asc', q: focusId || '' });
  const [data, proposals] = await Promise.all([
    loadLinkJSON<{
      candidates?: Array<{
        id: string;
        adr_id: string;
        confidence: string;
        escalation_reasons?: string[];
        statement: string;
        pattern_hint: string;
        source_path?: string;
        source_line_start?: number;
        source_line_end?: number;
      }>;
      confidence_summary?: { high: number; medium: number; low: number };
    }>('candidates'),
    loadLinkJSON<{ proposals?: Array<{ proposal_id: string; parent_candidate_id: string }> }>('proposals'),
  ]);
  const rows = data?.candidates || [];
  const propByParent = new Map<string, string>();
  for (const p of proposals?.proposals || []) propByParent.set(p.parent_candidate_id, p.proposal_id);
  const byId = new Map(rows.map((c) => [c.id, c]));

  const checklistFor = (list: typeof rows) =>
    ['## Pathfinder candidate checklist', '', ...list.map((c) => `- [ ] ${c.id} (${c.adr_id}): ${c.statement}`)].join('\n');

  app.innerHTML = `
    ${metricsRow([
      { label: 'Total', value: fmt(rows.length) },
      { label: 'High', value: fmt(data?.confidence_summary?.high ?? rows.filter((c) => c.confidence === 'high').length), cls: 'ok' },
      { label: 'Medium', value: fmt(data?.confidence_summary?.medium ?? rows.filter((c) => c.confidence === 'medium').length) },
      { label: 'Low', value: fmt(data?.confidence_summary?.low ?? rows.filter((c) => c.confidence === 'low').length), cls: 'warn' },
    ])}
    <div class="toolbar">
      <div class="seg" id="band-seg">
        ${['all', 'high', 'medium', 'low']
          .map((b) => `<button type="button" data-band="${b}" class="${state.band === b ? 'active' : ''}">${b[0].toUpperCase()}${b.slice(1)}</button>`)
          .join('')}
      </div>
      <input class="search" id="cand-q" placeholder="Search id or statement (/)" value="${escapeHTML(state.q)}" />
      <button type="button" class="btn" id="cand-export-tsv">Copy TSV</button>
      <button type="button" class="btn" id="cand-export-md">Copy Markdown</button>
      <button type="button" class="btn" id="cand-copy-check">Copy checklist</button>
      <a class="btn primary" id="cand-issue" href="#">Open checklist issue</a>
    </div>
    <div class="table-wrap sticky-head">
      <table class="dense" id="cand-table">
        <thead><tr id="cand-head"></tr></thead>
        <tbody id="cand-body"></tbody>
      </table>
    </div>
    <div id="cand-pager"></div>
  `;

  let band = state.band;
  let page = state.page;
  let pageSize = state.pageSize;
  let sort = state.sort || 'confidence';
  let dir: 'asc' | 'desc' = state.dir;
  let unbindKeys: (() => void) | undefined;
  const tbody = document.getElementById('cand-body')!;
  const thead = document.getElementById('cand-head')!;
  const pagerEl = document.getElementById('cand-pager')!;
  const searchEl = document.getElementById('cand-q') as HTMLInputElement;
  const issueBtn = document.getElementById('cand-issue') as HTMLAnchorElement;

  const filtered = () => {
    const q = (searchEl?.value || '').toLowerCase().trim();
    const list = rows.filter((c) => {
      if (band !== 'all' && c.confidence !== band) return false;
      if (focusId && !q && c.id === focusId) return true;
      if (!q) return true;
      return `${c.id} ${c.statement} ${c.pattern_hint} ${c.adr_id}`.toLowerCase().includes(q);
    });
    return sortBy(list, sort, dir, (c, key) => {
      if (key === 'confidence') return c.confidence;
      if (key === 'adr') return c.adr_id;
      if (key === 'pattern') return c.pattern_hint;
      if (key === 'statement') return c.statement;
      if (key === 'id') return c.id;
      return '';
    });
  };

  const syncHash = (replace = true) => {
    writeListQuery({ q: searchEl.value.trim(), page, pageSize, sort, dir, band }, { replace });
  };

  const expand = (c: (typeof rows)[number]) => {
    const prop = propByParent.get(c.id);
    const sourceBits = [
      c.source_path || '—',
      c.source_line_start != null
        ? `L${c.source_line_start}${c.source_line_end != null ? `–${c.source_line_end}` : ''}`
        : '',
    ]
      .filter(Boolean)
      .join(' · ');
    openDrawer({
      title: c.id,
      meta: `${c.adr_id} · ${c.confidence} · ${c.pattern_hint}`,
      wide: true,
      bodyHTML: `
        <dl class="kv">
          ${drawerField('ID', `<span class="cell-inline"><code class="id">${escapeHTML(c.id)}</code>${copyControl(c.id, 'Copy')}</span>`, true)}
          ${drawerField('ADR', `<a class="id" href="#/adrs/${escapeHTML(c.adr_id)}">${escapeHTML(c.adr_id)}</a>`)}
          ${drawerField('Band', tag(c.confidence, c.confidence === 'low' ? 'warn' : c.confidence === 'high' ? 'ok' : ''))}
          ${drawerField('Pattern', `<code class="id">${escapeHTML(c.pattern_hint)}</code>`, true)}
          ${drawerField('Source', `<code>${escapeHTML(sourceBits)}</code>`, true)}
          ${prop ? drawerField('Proposal', `<a class="id" href="#/proposals?parent=${escapeHTML(c.id)}">${escapeHTML(prop)}</a>`) : drawerField('Proposal', '<span class="faint">None</span>')}
        </dl>
        ${drawerBlock('Statement', `<p class="drawer-prose">${escapeHTML(c.statement)}</p>`)}
        ${
          c.escalation_reasons?.length
            ? drawerBlock(
                'Escalation',
                `<ul>${c.escalation_reasons.map((r) => `<li>${escapeHTML(r)}</li>`).join('')}</ul>`,
              )
            : ''
        }
        <div class="drawer-actions entity-links">
          <a href="#/adrs/${escapeHTML(c.adr_id)}">ADR</a>
          ${prop ? `<a href="#/proposals?parent=${escapeHTML(c.id)}">Proposal</a>` : ''}
          <a href="#/assess">Assess</a>
        </div>`,
    });
    bindCopyButtons(document.getElementById('drawer-root') || document);
  };

  const paint = () => {
    unbindKeys?.();
    const list = filtered();
    const slice = slicePage(list, page, pageSize);
    page = slice.page;
    pageSize = slice.pageSize;
    thead.innerHTML = `<th class="col-expand" scope="col"><span class="sr-only">Details</span></th>${sortHeader('ID', 'id', sort, dir)}${sortHeader('ADR', 'adr', sort, dir)}${sortHeader('Band', 'confidence', sort, dir)}<th>Links</th>${sortHeader('Pattern', 'pattern', sort, dir)}${sortHeader('Statement', 'statement', sort, dir)}`;
    bindSortHeaders(thead, (key, nextDir) => {
      sort = key;
      dir = nextDir;
      page = 1;
      syncHash();
      paint();
    });
    if (!slice.items.length) {
      tbody.innerHTML = filterEmpty(rows.length ? 'No candidates match this filter.' : 'No candidates in this snapshot.');
    } else {
      tbody.innerHTML = slice.items
        .map((c) => {
          const prop = propByParent.get(c.id);
          return `<tr data-row data-id="${escapeHTML(c.id)}">
            <td class="col-expand">${expandControl(c.id)}</td>
            <td class="col-id"><span class="cell-inline"><code class="id" title="${escapeHTML(c.id)}">${escapeHTML(c.id)}</code>${copyControl(c.id, 'Copy')}</span></td>
            <td class="col-adr"><a class="id" href="#/adrs/${escapeHTML(c.adr_id)}" title="${escapeHTML(c.adr_id)}">${escapeHTML(c.adr_id)}</a></td>
            <td class="col-band">${tag(c.confidence, c.confidence === 'low' ? 'warn' : c.confidence === 'high' ? 'ok' : '')}</td>
            <td class="col-links">${entityLinks([
              { href: `#/adrs/${c.adr_id}`, label: 'ADR' },
              prop ? { href: `#/proposals?parent=${c.id}`, label: 'Proposal' } : { href: '#', label: 'No proposal', faint: true },
              { href: '#/assess', label: 'Assess' },
            ])}</td>
            <td class="col-pattern truncate" title="${escapeHTML(c.pattern_hint)}"><code class="id">${escapeHTML(c.pattern_hint)}</code></td>
            <td class="col-statement truncate expand-cell" title="${escapeHTML(c.statement)}" data-expand="${escapeHTML(c.id)}">${escapeHTML(c.statement)}</td>
          </tr>`;
        })
        .join('');
    }
    pagerEl.innerHTML = renderPager(slice);
    bindCopyButtons(tbody);
    bindPager(pagerEl, () => ({ page, pageSize }), (next) => {
      page = next.page;
      pageSize = next.pageSize;
      syncHash();
      paint();
    });
    bindExpandControls(tbody, (id) => {
      const c = byId.get(id);
      if (c) expand(c);
    });
    unbindKeys = bindTableKeyboard(tbody, {
      onActivate: (row) => {
        const c = byId.get(row.dataset.id || '');
        if (c) location.hash = `#/adrs/${c.adr_id}`;
      },
      onExpand: (row) => {
        const c = byId.get(row.dataset.id || '');
        if (c) expand(c);
      },
    });
    const check = checklistFor(list);
    issueBtn.href = issueURL('[ADR Assessment] Candidate review batch', check);
    syncHash(true);
  };

  document.querySelectorAll('#band-seg button').forEach((btn) => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('#band-seg button').forEach((b) => b.classList.remove('active'));
      btn.classList.add('active');
      band = (btn as HTMLElement).dataset.band || 'all';
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
  document.getElementById('cand-export-tsv')?.addEventListener('click', async (e) => {
    const list = filtered();
    await copyWithButtonFeedback(
      e.currentTarget as HTMLButtonElement,
      toTSV(
        ['id', 'adr_id', 'confidence', 'pattern', 'statement'],
        list.map((c) => [c.id, c.adr_id, c.confidence, c.pattern_hint, c.statement]),
      ),
    );
  });
  document.getElementById('cand-export-md')?.addEventListener('click', async (e) => {
    const list = filtered();
    await copyWithButtonFeedback(
      e.currentTarget as HTMLButtonElement,
      toMarkdownTable(
        ['ID', 'ADR', 'Band', 'Pattern', 'Statement'],
        list.map((c) => [c.id, c.adr_id, c.confidence, c.pattern_hint, c.statement]),
      ),
    );
  });
  document.getElementById('cand-copy-check')?.addEventListener('click', async (e) => {
    await copyWithButtonFeedback(e.currentTarget as HTMLButtonElement, checklistFor(filtered()));
  });
  bindSlashSearch(searchEl);
  setTopActions('');
  paint();
}

export async function renderProposals(app: HTMLElement) {
  setCrumb('Proposals');
  setPurpose('Review-only assist drafts. Never auto-approved. / search · j/k · Enter/o/Space/Details expands.');
  setTopActions(`<a class="btn" href="#/costs">Cost detail</a>`);
  const state = readListQuery({ sort: 'status', dir: 'asc' });
  const data = await loadLinkJSON<{
    provider?: string;
    model?: string;
    estimated_cost_usd?: number;
    proposal_count?: number;
    proposals?: Array<{
      proposal_id: string;
      parent_candidate_id: string;
      statement: string;
      normalized_statement: string;
      suggested_pattern_id: string;
      rationale: string;
      executable: boolean;
      approval_status: string;
      parent_confidence?: string;
      provider?: string;
      model?: string;
    }>;
  }>('proposals');
  const parentFilter = queryParam('parent');
  const all = data?.proposals || [];
  const proposals = all.filter((p) => !parentFilter || p.parent_candidate_id === parentFilter);
  const latest = await loadLatest();
  const cands = await loadLinkJSON<{ candidates?: Array<{ id: string; adr_id: string }> }>('candidates');
  const adrByCand = new Map((cands?.candidates || []).map((c) => [c.id, c.adr_id]));
  const byId = new Map(proposals.map((p) => [p.proposal_id, p]));
  const batchProvider = data?.provider || String(latest?.metrics['kpi.assist.provider'] || '');
  const batchModel = data?.model || '';

  app.innerHTML = `
    ${parentFilter ? `<div class="banner">Filtered to parent <code>${escapeHTML(parentFilter)}</code> · <a href="#/proposals">Clear</a></div>` : ''}
    ${metricsRow([
      { label: 'Proposals', value: fmt(data?.proposal_count ?? proposals.length) },
      { label: 'Provider', value: fmt(data?.provider || latest?.metrics['kpi.assist.provider']) },
      { label: 'Est. cost', value: fmtUSD(data?.estimated_cost_usd ?? latest?.metrics['kpi.assist.cost_usd']) },
      { label: 'Accept rate', value: fmt(latest?.metrics['kpi.review.accept_rate']) },
    ])}
    <div class="toolbar">
      <input class="search" id="prop-q" placeholder="Search id, candidate, rationale, or pattern (/)" value="${escapeHTML(state.q)}" />
      <button type="button" class="btn" id="prop-export-tsv">Copy TSV</button>
      <button type="button" class="btn" id="prop-export-md">Copy Markdown</button>
    </div>
    <div class="table-wrap sticky-head">
      <table class="dense" id="prop-table">
        <thead><tr id="prop-head"></tr></thead>
        <tbody id="prop-body"></tbody>
      </table>
    </div>
    <div id="prop-pager"></div>
    <div id="prop-diff" class="proposal-diff-host" hidden></div>
  `;

  let page = state.page;
  let pageSize = state.pageSize;
  let sort = state.sort || 'status';
  let dir: 'asc' | 'desc' = state.dir;
  let unbindKeys: (() => void) | undefined;
  const tbody = document.getElementById('prop-body')!;
  const thead = document.getElementById('prop-head')!;
  const pagerEl = document.getElementById('prop-pager')!;
  const searchEl = document.getElementById('prop-q') as HTMLInputElement;
  const diffHost = document.getElementById('prop-diff')!;

  const filtered = () => {
    const q = (searchEl?.value || '').toLowerCase().trim();
    const list = proposals.filter((p) => {
      if (!q) return true;
      const adr = adrByCand.get(p.parent_candidate_id) || '';
      return `${p.proposal_id} ${p.parent_candidate_id} ${p.rationale} ${p.suggested_pattern_id} ${p.approval_status} ${adr} ${p.statement} ${p.normalized_statement}`
        .toLowerCase()
        .includes(q);
    });
    return sortBy(list, sort, dir, (p, key) => {
      if (key === 'id') return p.proposal_id;
      if (key === 'candidate') return p.parent_candidate_id;
      if (key === 'status') return p.approval_status;
      if (key === 'pattern') return p.suggested_pattern_id;
      if (key === 'rationale') return p.rationale;
      return '';
    });
  };

  const syncHash = (replace = true) => {
    writeListQuery({ q: searchEl.value.trim(), page, pageSize, sort, dir, band: 'all' }, { replace });
    if (parentFilter) setQuery({ parent: parentFilter }, { replace: true });
  };

  const showDiffPanel = (p: (typeof proposals)[number]) => {
    const patch = createTwoFilesPatch('candidate', 'proposal', `${p.statement}\n`, `${p.normalized_statement}\n`, '', '', {
      context: 4,
    });
    diffHost.hidden = false;
    diffHost.innerHTML = `
      <div class="proposal">
        <div class="meta">
          <span class="cell-inline"><code class="id">${escapeHTML(p.proposal_id)}</code>${copyControl(p.proposal_id, 'Copy')}</span>
          <span class="proposal-summary truncate" title="${escapeHTML(p.rationale)}">${escapeHTML(p.rationale)}</span>
          <button type="button" class="btn" id="prop-diff-close">Hide diff</button>
        </div>
        <div class="diff d2h-dark-color-scheme">${diff2html(patch, {
          drawFileList: false,
          matching: 'lines',
          outputFormat: 'side-by-side',
        })}</div>
      </div>`;
    bindCopyButtons(diffHost);
    document.getElementById('prop-diff-close')?.addEventListener('click', () => {
      diffHost.hidden = true;
      diffHost.innerHTML = '';
    });
  };

  const expand = (p: (typeof proposals)[number]) => {
    const adr = adrByCand.get(p.parent_candidate_id);
    const provider = p.provider || batchProvider || '—';
    const model = p.model || batchModel || '—';
    openDrawer({
      title: p.proposal_id,
      meta: `${p.approval_status} · ${p.suggested_pattern_id}`,
      wide: true,
      bodyHTML: `
        <dl class="kv">
          ${drawerField('ID', `<span class="cell-inline"><code class="id">${escapeHTML(p.proposal_id)}</code>${copyControl(p.proposal_id, 'Copy')}</span>`, true)}
          ${drawerField('Candidate', `<a class="id" href="#/candidates?id=${escapeHTML(p.parent_candidate_id)}">${escapeHTML(p.parent_candidate_id)}</a>`)}
          ${adr ? drawerField('ADR', `<a class="id" href="#/adrs/${escapeHTML(adr)}">${escapeHTML(adr)}</a>`) : ''}
          ${drawerField('Status', `${tag(p.approval_status)} ${tag(`exec=${p.executable}`, p.executable ? 'bad' : 'ok')}`)}
          ${drawerField('Pattern', `<code class="id">${escapeHTML(p.suggested_pattern_id)}</code>`, true)}
          ${p.parent_confidence ? drawerField('Parent band', tag(p.parent_confidence, p.parent_confidence === 'low' ? 'warn' : p.parent_confidence === 'high' ? 'ok' : '')) : ''}
          ${drawerField('Provider', escapeHTML(provider), true)}
          ${drawerField('Model', escapeHTML(model), true)}
          ${drawerField('Batch cost', escapeHTML(fmtUSD(data?.estimated_cost_usd ?? latest?.metrics['kpi.assist.cost_usd'])))}
        </dl>
        ${drawerBlock('Rationale', `<p class="drawer-prose">${escapeHTML(p.rationale)}</p>`)}
        ${drawerBlock('Candidate statement', `<p class="drawer-prose">${escapeHTML(p.statement)}</p>`)}
        ${drawerBlock('Normalized statement', `<p class="drawer-prose">${escapeHTML(p.normalized_statement)}</p>`)}
        <div class="drawer-actions">
          <button type="button" class="btn" id="drawer-show-diff">Show statement diff</button>
          <span class="entity-links">
            <a href="#/candidates?id=${escapeHTML(p.parent_candidate_id)}">Candidate</a>
            ${adr ? `<a href="#/adrs/${escapeHTML(adr)}">ADR</a>` : ''}
            <a href="#/assess">Assess</a>
            <a href="#/costs">Cost</a>
          </span>
        </div>`,
    });
    bindCopyButtons(document.getElementById('drawer-root') || document);
    document.getElementById('drawer-show-diff')?.addEventListener('click', () => {
      showDiffPanel(p);
      document.getElementById('prop-diff')?.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    });
  };

  const paint = () => {
    unbindKeys?.();
    const items = filtered();
    const slice = slicePage(items, page, pageSize);
    page = slice.page;
    pageSize = slice.pageSize;
    thead.innerHTML = `<th class="col-expand" scope="col"><span class="sr-only">Details</span></th>${sortHeader('ID', 'id', sort, dir)}${sortHeader('Candidate', 'candidate', sort, dir)}${sortHeader('Status', 'status', sort, dir)}<th>Links</th>${sortHeader('Pattern', 'pattern', sort, dir)}${sortHeader('Rationale', 'rationale', sort, dir)}`;
    bindSortHeaders(thead, (key, nextDir) => {
      sort = key;
      dir = nextDir;
      page = 1;
      syncHash();
      paint();
    });
    if (!slice.items.length) {
      tbody.innerHTML = filterEmpty('No proposals match this filter.');
    } else {
      tbody.innerHTML = slice.items
        .map((p) => {
          const adr = adrByCand.get(p.parent_candidate_id);
          return `<tr data-row data-id="${escapeHTML(p.proposal_id)}">
            <td class="col-expand">${expandControl(p.proposal_id)}</td>
            <td class="col-id"><span class="cell-inline"><code class="id" title="${escapeHTML(p.proposal_id)}">${escapeHTML(p.proposal_id)}</code>${copyControl(p.proposal_id, 'Copy')}</span></td>
            <td class="col-adr"><a class="id" href="#/candidates?id=${escapeHTML(p.parent_candidate_id)}" title="${escapeHTML(p.parent_candidate_id)}">${escapeHTML(p.parent_candidate_id)}</a></td>
            <td class="col-band">${tag(p.approval_status)} ${tag(`exec=${p.executable}`, p.executable ? 'bad' : 'ok')}</td>
            <td class="col-links">${entityLinks([
              { href: `#/candidates?id=${p.parent_candidate_id}`, label: 'Candidate' },
              ...(adr ? [{ href: `#/adrs/${adr}`, label: 'ADR' }] : []),
              { href: '#/assess', label: 'Assess' },
              { href: '#/costs', label: 'Cost' },
            ])}</td>
            <td class="col-pattern truncate" title="${escapeHTML(p.suggested_pattern_id)}"><code class="id">${escapeHTML(p.suggested_pattern_id)}</code></td>
            <td class="col-statement truncate expand-cell" title="${escapeHTML(p.rationale)}" data-expand="${escapeHTML(p.proposal_id)}">${escapeHTML(p.rationale)}</td>
          </tr>`;
        })
        .join('');
    }
    pagerEl.innerHTML = renderPager(slice);
    bindCopyButtons(tbody);
    bindPager(pagerEl, () => ({ page, pageSize }), (next) => {
      page = next.page;
      pageSize = next.pageSize;
      syncHash();
      paint();
    });
    bindExpandControls(tbody, (id) => {
      const p = byId.get(id);
      if (p) expand(p);
    });
    unbindKeys = bindTableKeyboard(tbody, {
      onActivate: (row) => {
        const p = byId.get(row.dataset.id || '');
        if (p) expand(p);
      },
      onExpand: (row) => {
        const p = byId.get(row.dataset.id || '');
        if (p) expand(p);
      },
    });
    syncHash(true);
  };

  searchEl?.addEventListener('input', () => {
    page = 1;
    syncHash(true);
    paint();
  });
  document.getElementById('prop-export-tsv')?.addEventListener('click', async (e) => {
    const list = filtered();
    await copyWithButtonFeedback(
      e.currentTarget as HTMLButtonElement,
      toTSV(
        ['proposal_id', 'parent_candidate_id', 'status', 'pattern', 'rationale'],
        list.map((p) => [p.proposal_id, p.parent_candidate_id, p.approval_status, p.suggested_pattern_id, p.rationale]),
      ),
    );
  });
  document.getElementById('prop-export-md')?.addEventListener('click', async (e) => {
    const list = filtered();
    await copyWithButtonFeedback(
      e.currentTarget as HTMLButtonElement,
      toMarkdownTable(
        ['Proposal', 'Candidate', 'Status', 'Pattern', 'Rationale'],
        list.map((p) => [p.proposal_id, p.parent_candidate_id, p.approval_status, p.suggested_pattern_id, p.rationale]),
      ),
    );
  });
  bindSlashSearch(searchEl);
  paint();
}

export async function renderAssess(app: HTMLElement) {
  setCrumb('Assessment');
  setPurpose('Deterministic assess gate. This path must never call an LLM. / search · j/k · Enter/o/Details expands.');
  const state = readListQuery({ band: 'all', sort: 'status', dir: 'asc' });
  const assessment = await loadLinkJSON<{
    verdict?: string;
    runtime_llm_calls?: number;
    coverage_status?: string;
    requirements?: Array<{ requirement_id: string; status: string; title?: string }>;
  }>('assessment');
  const latest = await loadLatest();
  const reqs = assessment?.requirements || [];
  const failing = reqs.filter((r) => r.status === 'FAIL' || r.status === 'REVIEW_REQUIRED');
  const body = [
    '## Assessment follow-up',
    '',
    `Verdict: ${assessment?.verdict || latest?.metrics['kpi.assess.verdict'] || 'unknown'}`,
    `Runtime LLM calls: ${assessment?.runtime_llm_calls ?? latest?.metrics['kpi.invariant.llm_calls'] ?? 'n/a'}`,
    '',
    ...failing.map((r) => `- [ ] ${r.requirement_id}: ${r.title || r.status}`),
  ].join('\n');
  setTopActions(`
    <a class="btn" href="#/docs/statuses">Status docs</a>
    <a class="btn primary" href="${issueURL('[ADR Assessment] Follow-up from workshop', body)}">Open follow-up issue</a>
  `);
  const llm = assessment?.runtime_llm_calls ?? Number(latest?.metrics['kpi.invariant.llm_calls'] ?? 0);
  const statusBands = ['all', 'FAIL', 'PASS', 'REVIEW_REQUIRED', 'NOT_APPLICABLE', 'EVIDENCE_MISSING', 'INDETERMINATE'] as const;
  app.innerHTML = `
    <div class="status-row">
      ${statusItem('Verdict', String(assessment?.verdict || latest?.metrics['kpi.assess.verdict'] || '—'), assessment?.verdict === 'PASS' || latest?.metrics['kpi.assess.verdict'] === 'PASS' ? 'ok' : 'warn')}
      ${statusItem('Runtime LLM calls', fmt(llm), llm === 0 ? 'ok' : 'bad')}
      ${statusItem('Coverage', String(assessment?.coverage_status || '—'), '')}
    </div>
    ${metricsRow([
      { label: 'Covered', value: fmt(latest?.metrics['kpi.assess.covered']), href: '#/assess?band=PASS' },
      { label: 'Unbound', value: fmt(latest?.metrics['kpi.assess.unbound']), href: '#/docs/obligations' },
      { label: 'Stale', value: fmt(latest?.metrics['kpi.assess.stale']), href: '#/docs/obligations' },
      { label: 'Failing reqs', value: fmt(failing.length), cls: failing.length ? 'warn' : 'ok', href: '#/assess?band=FAIL' },
    ])}
    <div class="toolbar">
      <div class="seg" id="assess-band">
        ${statusBands
          .map((b) => {
            const label = b === 'all' ? 'All' : b;
            const active = state.band === b || (b === 'all' && (!state.band || state.band === 'all'));
            return `<button type="button" data-band="${b}" class="${active ? 'active' : ''}">${escapeHTML(label)}</button>`;
          })
          .join('')}
      </div>
      <input class="search" id="assess-q" placeholder="Search requirement id or title (/)" value="${escapeHTML(state.q)}" />
      <button type="button" class="btn" id="assess-export-tsv">Copy TSV</button>
      <button type="button" class="btn" id="assess-copy-check">Copy failing checklist</button>
    </div>
    <div class="table-wrap sticky-head">
      <table class="dense">
        <thead><tr id="assess-head"></tr></thead>
        <tbody id="assess-body"></tbody>
      </table>
    </div>
    <div id="assess-pager"></div>
    <h2>Assessment artifact</h2>
    <div class="artifact"><pre>${escapeHTML(JSON.stringify(assessment || latest?.metrics || {}, null, 2))}</pre></div>
  `;

  let band = state.band && state.band !== 'all' ? state.band : 'all';
  let page = state.page;
  let pageSize = state.pageSize;
  let sort = state.sort || 'status';
  let dir: 'asc' | 'desc' = state.dir;
  let unbindKeys: (() => void) | undefined;
  const tbody = document.getElementById('assess-body')!;
  const thead = document.getElementById('assess-head')!;
  const pagerEl = document.getElementById('assess-pager')!;
  const searchEl = document.getElementById('assess-q') as HTMLInputElement;

  const filtered = () => {
    const q = (searchEl?.value || '').toLowerCase().trim();
    const list = reqs.filter((r) => {
      if (band !== 'all' && r.status !== band) return false;
      if (!q) return true;
      return `${r.requirement_id} ${r.title || ''} ${r.status}`.toLowerCase().includes(q);
    });
    return sortBy(list, sort, dir, (r, key) => {
      if (key === 'title') return r.title || '';
      if (key === 'status') return r.status;
      return r.requirement_id;
    });
  };

  const syncHash = (replace = true) => writeListQuery({ q: searchEl.value.trim(), page, pageSize, sort, dir, band }, { replace });

  const paint = () => {
    unbindKeys?.();
    const list = filtered();
    const slice = slicePage(list, page, pageSize);
    page = slice.page;
    pageSize = slice.pageSize;
    thead.innerHTML = `<th class="col-expand" scope="col"><span class="sr-only">Details</span></th>${sortHeader('Requirement', 'id', sort, dir)}${sortHeader('Status', 'status', sort, dir)}${sortHeader('Title', 'title', sort, dir)}`;
    bindSortHeaders(thead, (key, nextDir) => {
      sort = key;
      dir = nextDir;
      page = 1;
      syncHash();
      paint();
    });
    const expandReq = (r: (typeof reqs)[number]) => {
      const sev =
        r.status === 'FAIL' || r.status === 'EVIDENCE_MISSING'
          ? 'bad'
          : r.status === 'PASS'
            ? 'ok'
            : r.status === 'REVIEW_REQUIRED' || r.status === 'INDETERMINATE'
              ? 'warn'
              : '';
      openDrawer({
        title: r.requirement_id,
        meta: r.status,
        wide: true,
        bodyHTML: `
          <dl class="kv">
            ${drawerField('Requirement', `<span class="cell-inline"><code class="id">${escapeHTML(r.requirement_id)}</code>${copyControl(r.requirement_id, 'Copy')}</span>`, true)}
            ${drawerField('Status', tag(r.status, sev))}
          </dl>
          ${drawerBlock('Title', `<p class="drawer-prose">${escapeHTML(r.title || 'No title')}</p>`)}
          <div class="drawer-actions entity-links">
            <a href="#/docs/statuses">Status meanings</a>
            <a href="#/candidates">Candidates</a>
            <a href="#/docs/obligations">Obligations</a>
          </div>`,
      });
      bindCopyButtons(document.getElementById('drawer-root') || document);
    };
    tbody.innerHTML = slice.items.length
      ? slice.items
          .map((r) => {
            const sev =
              r.status === 'FAIL' || r.status === 'EVIDENCE_MISSING'
                ? 'bad'
                : r.status === 'PASS'
                  ? 'ok'
                  : r.status === 'REVIEW_REQUIRED' || r.status === 'INDETERMINATE'
                    ? 'warn'
                    : '';
            return `<tr data-row data-id="${escapeHTML(r.requirement_id)}">
              <td class="col-expand">${expandControl(r.requirement_id)}</td>
              <td><code class="id" title="${escapeHTML(r.requirement_id)}">${escapeHTML(r.requirement_id)}</code></td>
              <td>${tag(r.status, sev)}</td>
              <td class="truncate expand-cell" title="${escapeHTML(r.title || '')}" data-expand="${escapeHTML(r.requirement_id)}">${escapeHTML(r.title || '—')}</td>
            </tr>`;
          })
          .join('')
      : filterEmpty(reqs.length ? 'No requirements match this filter.' : 'No requirements in assessment artifact.');
    pagerEl.innerHTML = renderPager(slice);
    bindPager(pagerEl, () => ({ page, pageSize }), (next) => {
      page = next.page;
      pageSize = next.pageSize;
      syncHash();
      paint();
    });
    bindExpandControls(tbody, (id) => {
      const r = reqs.find((x) => x.requirement_id === id);
      if (r) expandReq(r);
    });
    unbindKeys = bindTableKeyboard(tbody, {
      onActivate: (row) => {
        const r = reqs.find((x) => x.requirement_id === row.dataset.id);
        if (r) expandReq(r);
      },
      onExpand: (row) => {
        const r = reqs.find((x) => x.requirement_id === row.dataset.id);
        if (r) expandReq(r);
      },
    });
    syncHash(true);
  };

  document.querySelectorAll('#assess-band button').forEach((btn) => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('#assess-band button').forEach((b) => b.classList.remove('active'));
      btn.classList.add('active');
      band = (btn as HTMLElement).dataset.band || 'all';
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
  document.getElementById('assess-export-tsv')?.addEventListener('click', async (e) => {
    const list = filtered();
    await copyWithButtonFeedback(
      e.currentTarget as HTMLButtonElement,
      toTSV(
        ['requirement_id', 'status', 'title'],
        list.map((r) => [r.requirement_id, r.status, r.title || '']),
      ),
    );
  });
  document.getElementById('assess-copy-check')?.addEventListener('click', async (e) => {
    await copyWithButtonFeedback(e.currentTarget as HTMLButtonElement, body);
  });
  bindSlashSearch(searchEl);
  paint();
}

export async function renderCosts(app: HTMLElement) {
  setCrumb('LLM cost');
  setPurpose('Batch assist usage only. Offline is $0; live estimates use reported tokens.');
  setTopActions(`<a class="btn primary" href="${workflowDispatchURL('llm-assist.yml')}">Dispatch assist</a>`);
  const [latest, costs, proposals, runs] = await Promise.all([
    loadLatest(),
    loadCosts(),
    loadLinkJSON<Record<string, unknown>>('proposals'),
    loadRuns(),
  ]);
  const m = latest?.metrics || {};
  app.innerHTML = `
    ${metricsRow([
      { label: 'Mode', value: fmt(latest?.run.provider_mode || m['kpi.assist.provider'] || 'offline') },
      { label: 'Model', value: fmt(m['kpi.assist.model'] || proposals?.model) },
      { label: 'Est. cost', value: fmtUSD(m['kpi.assist.cost_usd'] ?? proposals?.estimated_cost_usd ?? costs?.estimated_cost_usd) },
      { label: 'Prompt tokens', value: fmt(m['kpi.assist.prompt_tokens'] ?? proposals?.prompt_tokens) },
      { label: 'Completion', value: fmt(m['kpi.assist.completion_tokens'] ?? proposals?.completion_tokens) },
      { label: 'Assess LLM', value: fmt(m['kpi.invariant.llm_calls']), cls: 'ok', hint: 'must stay 0' },
    ])}
    ${chartHeading('Cost over runs', CHART_HELP.costOverRuns, 'h2')}
    <div id="cost-chart" class="chart"></div>
    <h2>Pricing notes</h2>
    <div class="artifact"><pre>${escapeHTML(
      JSON.stringify(
        costs || {
          pricing_note:
            proposals?.cost_pricing_note ||
            'offline=0; live uses gpt-4o-mini list rates ($0.15/1M input, $0.60/1M output)',
        },
        null,
        2,
      ),
    )}</pre></div>
  `;
  bindChartHelp(app);
  const el = document.getElementById('cost-chart');
  if (el) {
    chart(
      el,
      [
        { label: 'cost_usd', stroke: '#3fae7a', data: runs.map((r) => Number(r.metrics['kpi.assist.cost_usd'] || 0)) },
        { label: 'proposals', stroke: '#5b9fd4', data: runs.map((r) => Number(r.metrics['kpi.assist.proposal_count'] || 0)) },
      ],
      runs.map((_, i) => i),
    );
  }
}

function fieldRows(obj: Record<string, unknown> | null): string {
  if (!obj) return `<p class="muted">No artifact for this step.</p>`;
  const wrapKeys = new Set(['excerpt', 'statement', 'normalized_statement', 'summary', 'rationale']);
  return `<div class="fields">${Object.entries(obj)
    .map(([k, v]) => {
      const text = Array.isArray(v) ? v.join(', ') : v == null ? '—' : String(v);
      const isId = /(_id|id)$/i.test(k) || k.endsWith('_path');
      const wrap = wrapKeys.has(k);
      return `<div class="row"><div class="k">${escapeHTML(k)}</div><div class="v ${wrap ? 'wrap' : ''}" title="${escapeHTML(text)}">${
        isId && !wrap ? `<code>${escapeHTML(text)}</code>` : escapeHTML(text)
      }</div></div>`;
    })
    .join('')}</div>`;
}

export async function renderSample(app: HTMLElement, stepId?: string) {
  setCrumb('Sample journey');
  setPurpose('Walk one Pathfinder pass end to end — each step shows the idea, the command, and a real artifact shape.');
  const journey = await loadSampleJourney();
  const steps = journey?.steps || [];
  const idx = Math.max(0, steps.findIndex((s) => s.id === stepId));
  const step = steps[idx] || steps[0];
  const artifact = step?.artifact_key ? (journey?.artifacts?.[step.artifact_key] as Record<string, unknown> | undefined) : null;
  setTopActions('');
  if (!step) {
    app.innerHTML = emptyState('Journey missing', 'sample-journey.json not published.');
    return;
  }
  const pct = ((idx + 1) / steps.length) * 100;
  const prev = idx > 0 ? steps[idx - 1] : null;
  const next = idx < steps.length - 1 ? steps[idx + 1] : null;
  app.innerHTML = `
    <div class="journey">
      <nav class="journey-rail" aria-label="Journey steps">
        ${steps
          .map((s, i) => {
            const state = s.id === step.id ? 'active' : i < idx ? 'done' : '';
            const label = s.short || s.title;
            return `<a class="${state}" href="#/sample/${escapeHTML(s.id)}" title="${escapeHTML(s.title)}">
              <span class="n">${String(i + 1).padStart(2, '0')}</span>
              <span class="t">${escapeHTML(label)}</span>
            </a>`;
          })
          .join('')}
      </nav>
      <div class="journey-body">
        <div class="journey-progress" aria-hidden="true"><span style="width:${pct}%"></span></div>
        <section class="journey-copy">
          <p class="faint">Step ${idx + 1} of ${steps.length}</p>
          <h1>${escapeHTML(step.title)}</h1>
          <p class="prose">${escapeHTML(step.narrative)}</p>
          <h2>What happens technically</h2>
          <p class="prose">${escapeHTML(step.technical)}</p>
          ${(step.commands || [])
            .map(
              (c) =>
                `<div class="cmd-line" title="${escapeHTML(c)}"><code>${escapeHTML(c)}</code> ${copyControl(c, 'Copy')}</div>`,
            )
            .join('')}
          ${
            (step.checks || []).length
              ? `<h2>Checks</h2><ul class="checklist">${(step.checks || [])
                  .map((c) => `<li><span>${escapeHTML(c)}</span></li>`)
                  .join('')}</ul>`
              : ''
          }
        </section>
        <section>
          <h2>${escapeHTML(step.artifact_label || 'Example artifact')}</h2>
          ${fieldRows(artifact || null)}
        </section>
        <div class="journey-nav">
          ${prev ? `<a class="btn" id="sample-prev" href="#/sample/${escapeHTML(prev.id)}">← ${escapeHTML(prev.short || prev.title)}</a>` : `<span></span>`}
          ${
            next
              ? `<a class="btn primary" id="sample-next" href="#/sample/${escapeHTML(next.id)}">${escapeHTML(next.short || next.title)} →</a>`
              : `<a class="btn primary" href="#/pipeline">Explore live pipeline →</a>`
          }
        </div>
        <p class="faint">Keyboard: ← previous · → next</p>
      </div>
    </div>
  `;
  bindCopyButtons(app);
  const keyFlag = '__pfSampleKeyHandler' as const;
  const w = window as Window & { [keyFlag]?: (e: KeyboardEvent) => void };
  if (w[keyFlag]) window.removeEventListener('keydown', w[keyFlag]!);
  w[keyFlag] = (e: KeyboardEvent) => {
    if (!location.hash.startsWith('#/sample')) return;
    if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement || e.target instanceof HTMLSelectElement) return;
    if (e.key === 'ArrowLeft' && prev) {
      e.preventDefault();
      location.hash = `#/sample/${prev.id}`;
    }
    if (e.key === 'ArrowRight' && next) {
      e.preventDefault();
      location.hash = `#/sample/${next.id}`;
    }
  };
  window.addEventListener('keydown', w[keyFlag]);
}

export async function renderRuns(app: HTMLElement) {
  setCrumb('Runs');
  setPurpose('History and KPI compare. Compare selections persist in the URL.');
  setTopActions('');
  const state = readListQuery({ sort: 'when', dir: 'desc' });
  const runs = await loadRuns();
  const ordered = runs.slice().reverse();
  const aSel = getQueryValue('a') || ordered[Math.min(1, ordered.length - 1)]?.run.run_id || '';
  const bSel = getQueryValue('b') || ordered[0]?.run.run_id || '';
  const options = ordered
    .map(
      (r) =>
        `<option value="${escapeHTML(r.run.run_id)}">${escapeHTML(r.run.run_id)} · ${escapeHTML(r.run.workflow)}</option>`,
    )
    .join('');
  app.innerHTML = `
    <div class="toolbar">
      <label class="muted">A <select id="run-a">${options}</select></label>
      <label class="muted">B <select id="run-b">${options}</select></label>
      <button type="button" class="btn primary" id="run-compare-btn">Compare</button>
      <input class="search" id="run-q" placeholder="Search run id or workflow (/)" value="${escapeHTML(state.q)}" />
      <button type="button" class="btn" id="run-export-tsv">Copy TSV</button>
    </div>
    <div id="run-compare-out" class="muted" style="margin-bottom:1rem">Select two runs to compare.</div>
    <div class="table-wrap sticky-head">
      <table class="dense">
        <thead><tr id="run-head"></tr></thead>
        <tbody id="run-body"></tbody>
      </table>
    </div>
    <div id="run-pager"></div>
  `;
  let page = state.page;
  let pageSize = state.pageSize;
  let sort = state.sort || 'when';
  let dir: 'asc' | 'desc' = state.dir;
  let unbindKeys: (() => void) | undefined;
  const tbody = document.getElementById('run-body')!;
  const thead = document.getElementById('run-head')!;
  const pagerEl = document.getElementById('run-pager')!;
  const searchEl = document.getElementById('run-q') as HTMLInputElement;
  const a = document.getElementById('run-a') as HTMLSelectElement | null;
  const b = document.getElementById('run-b') as HTMLSelectElement | null;
  if (a) a.value = aSel;
  if (b) b.value = bSel;

  const filtered = () => {
    const q = (searchEl?.value || '').toLowerCase().trim();
    const list = !q
      ? ordered
      : ordered.filter((r) =>
          `${r.run.run_id} ${r.run.workflow} ${r.run.timestamp || ''} ${r.metrics['kpi.assess.verdict'] || ''}`
            .toLowerCase()
            .includes(q),
        );
    return sortBy(list, sort, dir, (r, key) => {
      if (key === 'workflow') return r.run.workflow;
      if (key === 'when') return r.run.timestamp || '';
      if (key === 'candidates') return Number(r.metrics.candidate_total || 0);
      if (key === 'low') return Number(r.metrics.confidence_low || 0);
      if (key === 'cost') return Number(r.metrics['kpi.assist.cost_usd'] || 0);
      if (key === 'verdict') return String(r.metrics['kpi.assess.verdict'] || '');
      return r.run.run_id;
    });
  };

  const syncHash = (replace = true) => {
    writeListQuery({ q: searchEl.value.trim(), page, pageSize, sort, dir }, { replace });
    setQuery({ a: a?.value || null, b: b?.value || null }, { replace: true });
  };

  const paint = () => {
    unbindKeys?.();
    const list = filtered();
    const slice = slicePage(list, page, pageSize);
    page = slice.page;
    pageSize = slice.pageSize;
    thead.innerHTML = `${sortHeader('Run', 'id', sort, dir)}${sortHeader('Workflow', 'workflow', sort, dir)}${sortHeader('When', 'when', sort, dir)}${sortHeader('Candidates', 'candidates', sort, dir)}${sortHeader('Low', 'low', sort, dir)}${sortHeader('Cost', 'cost', sort, dir)}${sortHeader('Verdict', 'verdict', sort, dir)}`;
    bindSortHeaders(thead, (key, nextDir) => {
      sort = key;
      dir = nextDir;
      page = 1;
      syncHash();
      paint();
    });
    tbody.innerHTML = slice.items.length
      ? slice.items
          .map(
            (r) => `<tr class="row-link" data-row data-run="${escapeHTML(r.run.run_id)}">
          <td><code class="id" title="${escapeHTML(r.run.run_id)}">${escapeHTML(r.run.run_id)}</code></td>
          <td class="truncate" title="${escapeHTML(r.run.workflow)}">${escapeHTML(r.run.workflow)}</td>
          <td class="truncate num" title="${escapeHTML(r.run.timestamp || '')}">${escapeHTML(r.run.timestamp || '')}</td>
          <td class="num">${fmt(r.metrics.candidate_total)}</td>
          <td class="num">${fmt(r.metrics.confidence_low)}</td>
          <td class="num">${fmtUSD(r.metrics['kpi.assist.cost_usd'])}</td>
          <td>${fmt(r.metrics['kpi.assess.verdict'])}</td>
        </tr>`,
          )
          .join('')
      : filterEmpty('No runs match this filter.');
    pagerEl.innerHTML = renderPager(slice);
    tbody.querySelectorAll<HTMLElement>('tr.row-link[data-run]').forEach((tr) => {
      tr.addEventListener('click', () => {
        const id = tr.dataset.run;
        if (!id) return;
        setSelectedRunId(id);
        location.hash = '#/';
      });
    });
    bindPager(pagerEl, () => ({ page, pageSize }), (next) => {
      page = next.page;
      pageSize = next.pageSize;
      syncHash();
      paint();
    });
    unbindKeys = bindTableKeyboard(tbody, {
      onActivate: (row) => {
        const id = row.dataset.run;
        if (!id) return;
        setSelectedRunId(id);
        location.hash = '#/';
      },
    });
    syncHash(true);
  };

  searchEl?.addEventListener('input', () => {
    page = 1;
    syncHash(true);
    paint();
  });
  document.getElementById('run-export-tsv')?.addEventListener('click', async (e) => {
    const list = filtered();
    await copyWithButtonFeedback(
      e.currentTarget as HTMLButtonElement,
      toTSV(
        ['run_id', 'workflow', 'timestamp', 'candidates', 'low', 'cost', 'verdict'],
        list.map((r) => [
          r.run.run_id,
          r.run.workflow,
          r.run.timestamp || '',
          String(r.metrics.candidate_total ?? ''),
          String(r.metrics.confidence_low ?? ''),
          String(r.metrics['kpi.assist.cost_usd'] ?? ''),
          String(r.metrics['kpi.assess.verdict'] ?? ''),
        ]),
      ),
    );
  });
  bindSlashSearch(searchEl);
  paint();

  let compareChangesOnly = false;

  const doCompare = () => {
    const out = document.getElementById('run-compare-out');
    if (!out || !a || !b) return;
    syncHash(true);
    const left = runs.find((r) => r.run.run_id === a.value);
    const right = runs.find((r) => r.run.run_id === b.value);
    if (!left || !right || left.run.run_id === right.run.run_id) {
      out.textContent = 'Choose two different run ids.';
      return;
    }
    const rows = buildMetricCompareRows(left.metrics, right.metrics);
    const summary = summarizeMetricCompare(rows);
    const visible = compareChangesOnly ? rows.filter((r) => r.changed) : rows;
    const body = visible.length
      ? visible
          .map((row) => {
            const rowCls = row.changed
              ? `compare-row compare-row-${row.direction === 'unchanged' ? 'same' : row.direction}`
              : 'compare-row compare-row-same';
            return `<tr class="${rowCls}" data-changed="${row.changed ? '1' : '0'}">
          <td><code class="id" title="${escapeHTML(row.key)}">${escapeHTML(row.key)}</code></td>
          ${compareValueCell(row.a, 'a', row)}
          ${compareValueCell(row.b, 'b', row)}
          ${compareDeltaCell(row)}
        </tr>`;
          })
          .join('')
      : `<tr><td colspan="4" class="muted">${compareChangesOnly ? 'No changed metrics between these runs.' : 'No metrics.'}</td></tr>`;

    out.innerHTML = `<div class="compare-panel">
      ${renderCompareSummaryStrip(summary)}
      <div class="compare-toolbar">
        <label class="compare-filter muted"><input type="checkbox" id="run-compare-changes"${compareChangesOnly ? ' checked' : ''} /> Changes only</label>
        <span class="faint compare-filter-meta">${visible.length} of ${rows.length} metrics</span>
      </div>
      <div class="table-wrap sticky-head compare-table-wrap">
        <table class="dense compare-table">
          <thead><tr><th>Metric</th><th>A</th><th>B</th><th>Δ</th></tr></thead>
          <tbody>${body}</tbody>
        </table>
      </div>
    </div>`;

    document.getElementById('run-compare-changes')?.addEventListener('change', (e) => {
      compareChangesOnly = (e.target as HTMLInputElement).checked;
      doCompare();
    });
  };
  document.getElementById('run-compare-btn')?.addEventListener('click', doCompare);
  if (getQueryValue('a') && getQueryValue('b')) doCompare();
}

export async function renderEvals(app: HTMLElement) {
  setCrumb('Evals');
  setPurpose('Promptfoo scores assist quality only — not a Pathfinder compliance verdict.');
  const pf = await loadLinkJSON<Record<string, unknown>>('promptfoo');
  const latest = await loadLatest();
  setTopActions(`<a class="btn primary" href="${workflowDispatchURL('llm-assist.yml')}">Dispatch LLM assist</a>`);
  app.innerHTML = `
    ${metricsRow([
      { label: 'Pass rate', value: fmt(latest?.metrics['kpi.promptfoo.pass_rate'] ?? pf?.pass_rate) },
      { label: 'Mode', value: fmt(latest?.metrics.promptfoo_mode ?? pf?.mode) },
    ])}
    <h2>Latest Promptfoo summary</h2>
    <div class="artifact"><pre>${escapeHTML(JSON.stringify(pf || {}, null, 2))}</pre></div>
  `;
}

export async function renderCorpus(app: HTMLElement) {
  setCrumb('Corpus');
  setPurpose('Labeled harness entries. / search · j/k · Enter opens ADR · o/Space/Details expands marker.');
  const state = readListQuery({ band: 'all', sort: 'adr', dir: 'asc' });
  const [latest, corpus] = await Promise.all([loadLatest(), loadCorpusIndex()]);
  const { owner, repo } = repoBase();
  const entries = (corpus?.entries || []).map((e, i) => ({ ...e, _i: i }));
  const byKey = new Map(entries.map((e) => [String(e._i), e]));
  setTopActions(`
    <a class="btn" href="#/adrs">Published ADRs</a>
    <a class="btn" href="#/docs/corpus">Corpus docs</a>
    <a class="btn primary" href="${workflowDispatchURL('corpus-eval.yml')}">Run corpus eval</a>
  `);
  app.innerHTML = `
    ${metricsRow([
      { label: 'Corpus size', value: fmt(corpus?.corpus_size ?? latest?.metrics['kpi.corpus.size'] ?? entries.length), href: '#/docs/corpus' },
      { label: 'Precision', value: fmt(latest?.metrics['kpi.harness.precision']), href: '#/evals' },
      { label: 'Recall', value: fmt(latest?.metrics['kpi.harness.recall']), href: '#/evals' },
      { label: 'Recall H/M/L', value: `${fmt(latest?.metrics['kpi.harness.recall.high'])} / ${fmt(latest?.metrics['kpi.harness.recall.medium'])} / ${fmt(latest?.metrics['kpi.harness.recall.low'])}` },
      { label: 'Escalate labels', value: fmt(entries.filter((e) => e.escalate_expected).length), href: '#/corpus?band=low' },
    ])}
    <div class="toolbar">
      <div class="seg" id="corpus-band">
        ${['all', 'high', 'medium', 'low']
          .map((b) => `<button type="button" data-band="${b}" class="${state.band === b ? 'active' : ''}">${b[0].toUpperCase()}${b.slice(1)}</button>`)
          .join('')}
      </div>
      <input class="search" id="corpus-q" placeholder="Search ADR id or statement marker (/)" value="${escapeHTML(state.q)}" />
      <button type="button" class="btn" id="corpus-export-tsv">Copy TSV</button>
      <button type="button" class="btn" id="corpus-export-md">Copy Markdown</button>
      <a class="btn" href="https://github.com/${owner}/${repo}/compare">Open labeling PR</a>
    </div>
    <div class="table-wrap sticky-head">
      <table class="dense">
        <thead><tr id="corpus-head"></tr></thead>
        <tbody id="corpus-body"></tbody>
      </table>
    </div>
    <div id="corpus-pager"></div>
    <p class="run-meta" style="margin-top:1rem">Source: <code>${escapeHTML(corpus?.source || 'testdata/corpus/labels.json')}</code> · label via PR with <code>[corpus-label]</code>. Harness scores ≠ compliance verdicts.</p>
  `;
  let band = state.band;
  let page = state.page;
  let pageSize = state.pageSize;
  let sort = state.sort || 'adr';
  let dir: 'asc' | 'desc' = state.dir;
  let unbindKeys: (() => void) | undefined;
  const tbody = document.getElementById('corpus-body')!;
  const thead = document.getElementById('corpus-head')!;
  const pagerEl = document.getElementById('corpus-pager')!;
  const searchEl = document.getElementById('corpus-q') as HTMLInputElement;

  const filtered = () => {
    const q = (searchEl?.value || '').toLowerCase().trim();
    const list = entries.filter((e) => {
      if (band !== 'all' && e.confidence_band !== band) return false;
      if (!q) return true;
      return `${e.adr_id} ${e.statement_contains} ${e.pattern_id} ${e.modality}`.toLowerCase().includes(q);
    });
    return sortBy(list, sort, dir, (e, key) => {
      if (key === 'band') return e.confidence_band;
      if (key === 'modality') return e.modality;
      if (key === 'pattern') return e.pattern_id;
      if (key === 'escalate') return e.escalate_expected ? 1 : 0;
      if (key === 'marker') return e.statement_contains;
      return e.adr_id;
    });
  };

  const syncHash = (replace = true) => writeListQuery({ q: searchEl.value.trim(), page, pageSize, sort, dir, band }, { replace });

  const expand = (e: (typeof entries)[number]) => {
    openDrawer({
      title: e.adr_id,
      meta: `${e.confidence_band} · ${e.pattern_id}`,
      wide: true,
      bodyHTML: `
        <dl class="kv">
          ${drawerField('ADR', `<span class="cell-inline"><a class="id" href="${githubBlobURL(`testdata/corpus/adrs/${e.adr_id}.md`)}">${escapeHTML(e.adr_id)}</a>${copyControl(e.adr_id, 'Copy')}</span>`, true)}
          ${drawerField('Band', tag(e.confidence_band, e.confidence_band === 'low' ? 'warn' : e.confidence_band === 'high' ? 'ok' : ''))}
          ${drawerField('Modality', escapeHTML(e.modality), true)}
          ${drawerField('Pattern', `<code class="id">${escapeHTML(e.pattern_id)}</code>`, true)}
          ${drawerField('Escalate', e.escalate_expected ? tag('expected', 'warn') : tag('no'))}
        </dl>
        ${drawerBlock('Statement marker', `<p class="drawer-prose">${escapeHTML(e.statement_contains)}</p>`)}
        <div class="drawer-actions entity-links">
          <a href="${githubBlobURL(`testdata/corpus/adrs/${e.adr_id}.md`)}">GitHub ADR</a>
          <a href="#/candidates?q=${encodeURIComponent(e.adr_id)}">Candidates</a>
          <a href="#/docs/corpus">Corpus docs</a>
        </div>`,
    });
    bindCopyButtons(document.getElementById('drawer-root') || document);
  };

  const paint = () => {
    unbindKeys?.();
    const list = filtered();
    const slice = slicePage(list, page, pageSize);
    page = slice.page;
    pageSize = slice.pageSize;
    thead.innerHTML = `<th class="col-expand" scope="col"><span class="sr-only">Details</span></th>${sortHeader('ADR', 'adr', sort, dir)}${sortHeader('Band', 'band', sort, dir)}${sortHeader('Modality', 'modality', sort, dir)}${sortHeader('Pattern', 'pattern', sort, dir)}${sortHeader('Escalate', 'escalate', sort, dir)}${sortHeader('Marker', 'marker', sort, dir)}`;
    bindSortHeaders(thead, (key, nextDir) => {
      sort = key;
      dir = nextDir;
      page = 1;
      syncHash();
      paint();
    });
    if (!slice.items.length) {
      tbody.innerHTML = filterEmpty(entries.length ? 'No corpus rows match this filter.' : 'No corpus index in this snapshot.');
    } else {
      tbody.innerHTML = slice.items
        .map((e) => {
          const rowKey = String(e._i);
          return `<tr data-row data-key="${escapeHTML(rowKey)}" data-adr="${escapeHTML(e.adr_id)}">
          <td class="col-expand">${expandControl(rowKey)}</td>
          <td class="col-adr"><a class="id" href="${githubBlobURL(`testdata/corpus/adrs/${e.adr_id}.md`)}" title="${escapeHTML(e.adr_id)}">${escapeHTML(e.adr_id)}</a></td>
          <td class="col-band">${tag(e.confidence_band, e.confidence_band === 'low' ? 'warn' : e.confidence_band === 'high' ? 'ok' : '')}</td>
          <td class="truncate" title="${escapeHTML(e.modality)}">${escapeHTML(e.modality)}</td>
          <td class="col-pattern truncate" title="${escapeHTML(e.pattern_id)}"><code class="id">${escapeHTML(e.pattern_id)}</code></td>
          <td>${e.escalate_expected ? tag('yes', 'warn') : tag('no')}</td>
          <td class="truncate expand-cell" title="${escapeHTML(e.statement_contains)}" data-expand="${escapeHTML(rowKey)}">${escapeHTML(e.statement_contains)}</td>
        </tr>`;
        })
        .join('');
    }
    pagerEl.innerHTML = renderPager(slice);
    bindPager(pagerEl, () => ({ page, pageSize }), (next) => {
      page = next.page;
      pageSize = next.pageSize;
      syncHash();
      paint();
    });
    bindExpandControls(tbody, (id) => {
      const e = byKey.get(id);
      if (e) expand(e);
    });
    unbindKeys = bindTableKeyboard(tbody, {
      onActivate: (row) => {
        const adr = row.dataset.adr;
        if (adr) window.open(githubBlobURL(`testdata/corpus/adrs/${adr}.md`), '_blank', 'noopener');
      },
      onExpand: (row) => {
        const e = byKey.get(row.dataset.key || '');
        if (e) expand(e);
      },
    });
    syncHash(true);
  };

  document.querySelectorAll('#corpus-band button').forEach((btn) => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('#corpus-band button').forEach((b) => b.classList.remove('active'));
      btn.classList.add('active');
      band = (btn as HTMLElement).dataset.band || 'all';
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
  document.getElementById('corpus-export-tsv')?.addEventListener('click', async (ev) => {
    const list = filtered();
    await copyWithButtonFeedback(
      ev.currentTarget as HTMLButtonElement,
      toTSV(
        ['adr_id', 'confidence_band', 'modality', 'pattern_id', 'escalate_expected', 'statement_contains'],
        list.map((e) => [
          e.adr_id,
          e.confidence_band,
          e.modality,
          e.pattern_id,
          String(e.escalate_expected),
          e.statement_contains,
        ]),
      ),
    );
  });
  document.getElementById('corpus-export-md')?.addEventListener('click', async (ev) => {
    const list = filtered();
    await copyWithButtonFeedback(
      ev.currentTarget as HTMLButtonElement,
      toMarkdownTable(
        ['ADR', 'Band', 'Modality', 'Pattern', 'Escalate', 'Marker'],
        list.map((e) => [
          e.adr_id,
          e.confidence_band,
          e.modality,
          e.pattern_id,
          e.escalate_expected ? 'yes' : 'no',
          e.statement_contains,
        ]),
      ),
    );
  });
  bindSlashSearch(searchEl);
  paint();
}

type DocsHeading = { id: string; text: string; level: number };

function docsSlugify(text: string): string {
  const base = text
    .toLowerCase()
    .replace(/&amp;/g, 'and')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 64);
  return base || 'section';
}

/** Inject stable ids on h2/h3 in trusted docs HTML and collect a TOC. */
function prepareDocsBody(body: string): { html: string; headings: DocsHeading[] } {
  const headings: DocsHeading[] = [];
  const used = new Set<string>();
  const html = body.replace(/<h([23])(\s[^>]*)?>([\s\S]*?)<\/h\1>/gi, (_m, levelStr: string, attrs: string | undefined, inner: string) => {
    const level = Number(levelStr);
    const text = inner.replace(/<[^>]+>/g, '').replace(/\s+/g, ' ').trim();
    const existing = attrs?.match(/\sid=["']([^"']+)["']/i)?.[1];
    let id = existing || docsSlugify(text);
    if (!existing) {
      let n = 2;
      while (used.has(id)) id = `${docsSlugify(text)}-${n++}`;
    }
    used.add(id);
    headings.push({ id, text, level });
    const cleanedAttrs = (attrs || '').replace(/\sid=["'][^"']*["']/i, '');
    return `<h${level}${cleanedAttrs} id="${escapeHTML(id)}">${inner}</h${level}>`;
  });
  return { html, headings };
}

function renderDocsNav(
  groups: ReturnType<typeof docsGroups>,
  activeId: string | null,
  owner: string,
  repo: string,
  repoLinks: Array<[string, string]>,
): string {
  return `
    <nav class="docs-nav" aria-label="Documentation">
      <div class="docs-nav-sticky">
        <div class="docs-nav-head">
          <a class="docs-nav-home${activeId == null ? ' active' : ''}" href="#/docs">Documentation</a>
          <label class="docs-nav-search">
            <span class="visually-hidden">Filter docs</span>
            <input type="search" id="docs-toc-filter" class="docs-nav-filter" placeholder="Filter sections…" autocomplete="off" />
          </label>
        </div>
        <div class="docs-nav-scroll" id="docs-nav-scroll">
          ${groups
            .map(
              (g) => `<div class="docs-group" data-docs-group>
                <div class="docs-group-label">${escapeHTML(g.group)}</div>
                ${g.items
                  .map(
                    (s) =>
                      `<a href="#/docs/${escapeHTML(s.id)}" class="docs-nav-link${s.id === activeId ? ' active' : ''}" data-docs-title="${escapeHTML(s.title)}" title="${escapeHTML(s.summary)}">${escapeHTML(s.title)}</a>`,
                  )
                  .join('')}
              </div>`,
            )
            .join('')}
          <div class="docs-group docs-group-external" data-docs-group>
            <div class="docs-group-label">Repository sources</div>
            ${repoLinks
              .map(
                ([file, label]) =>
                  `<a class="docs-nav-link docs-nav-ext" href="https://github.com/${owner}/${repo}/blob/main/docs/${file}" data-docs-title="${escapeHTML(label)}" target="_blank" rel="noopener noreferrer">${escapeHTML(label)}</a>`,
              )
              .join('')}
          </div>
        </div>
      </div>
    </nav>`;
}

function renderDocsLanding(groups: ReturnType<typeof docsGroups>, owner: string, repo: string): string {
  const overview = docsById('overview');
  return `
    <article class="docs-article docs-landing">
      <p class="docs-kicker">Pathfinder docs</p>
      <h1>Documentation</h1>
      <p class="docs-lead">${escapeHTML(
        overview?.summary ||
          'Deterministic ADR compliance: extract → review → bind → evidence → assess.',
      )}</p>
      <div class="docs-landing-intro">
        <p>This in-app suite covers the assessment engine, pipelines, analyzers, governance, and operator reference. Use the sidebar to jump sections, or start from a group below. Authoritative Markdown still lives in the repository <code>docs/</code> tree.</p>
        <p class="docs-landing-actions">
          <a class="btn primary" href="#/docs/overview">Start with overview</a>
          <a class="btn" href="#/sample">Sample journey</a>
          <a class="btn" href="https://github.com/${owner}/${repo}/tree/main/docs">Repo docs/</a>
        </p>
      </div>
      <div class="docs-home-groups">
        ${groups
          .map(
            (g) => `<section class="docs-home-group">
              <h2>${escapeHTML(g.group)}</h2>
              <ul class="docs-home-list">
                ${g.items
                  .map(
                    (s) => `<li>
                      <a class="docs-home-card" href="#/docs/${escapeHTML(s.id)}">
                        <span class="docs-home-card-title">${escapeHTML(s.title)}</span>
                        <span class="docs-home-card-summary">${escapeHTML(s.summary)}</span>
                      </a>
                    </li>`,
                  )
                  .join('')}
              </ul>
            </section>`,
          )
          .join('')}
      </div>
    </article>`;
}

function renderDocsArticle(
  active: (typeof DOC_SECTIONS)[number],
  bodyHtml: string,
  prev: (typeof DOC_SECTIONS)[number] | undefined,
  next: (typeof DOC_SECTIONS)[number] | undefined,
): string {
  return `
    <article class="docs-article" id="docs-article">
      <header class="docs-article-head">
        <p class="docs-kicker">${escapeHTML(active.group)}</p>
        <h1 id="docs-top">${escapeHTML(active.title)}</h1>
        <p class="docs-lead">${escapeHTML(active.summary)}</p>
      </header>
      <div class="docs-prose">
        ${bodyHtml}
      </div>
      <nav class="docs-pager" aria-label="Section navigation">
        ${
          prev
            ? `<a class="docs-pager-link prev" href="#/docs/${escapeHTML(prev.id)}">
                <span class="docs-pager-label">Previous</span>
                <span class="docs-pager-title">${escapeHTML(prev.title)}</span>
              </a>`
            : `<span class="docs-pager-link prev empty"></span>`
        }
        ${
          next
            ? `<a class="docs-pager-link next" href="#/docs/${escapeHTML(next.id)}">
                <span class="docs-pager-label">Next</span>
                <span class="docs-pager-title">${escapeHTML(next.title)}</span>
              </a>`
            : `<span class="docs-pager-link next empty"></span>`
        }
      </nav>
    </article>`;
}

function renderDocsOnThisPage(headings: DocsHeading[]): string {
  if (!headings.length) return `<aside class="docs-otp docs-otp-empty" aria-hidden="true"></aside>`;
  // Use buttons — bare #fragment hrefs would clobber the SPA hash router (#/docs/…).
  return `
    <aside class="docs-otp" aria-label="On this page">
      <div class="docs-otp-sticky">
        <div class="docs-otp-label">On this page</div>
        <nav class="docs-otp-nav">
          <button type="button" class="docs-otp-link" data-docs-anchor="docs-top">Overview</button>
          ${headings
            .map(
              (h) =>
                `<button type="button" class="docs-otp-link level-${h.level}" data-docs-anchor="${escapeHTML(h.id)}">${escapeHTML(h.text)}</button>`,
            )
            .join('')}
        </nav>
      </div>
    </aside>`;
}

function bindDocsTocFilter(root: HTMLElement) {
  const input = root.querySelector<HTMLInputElement>('#docs-toc-filter');
  if (!input) return;
  const apply = () => {
    const q = input.value.trim().toLowerCase();
    root.querySelectorAll<HTMLElement>('[data-docs-group]').forEach((group) => {
      let visible = 0;
      group.querySelectorAll<HTMLAnchorElement>('[data-docs-title]').forEach((link) => {
        const title = (link.dataset.docsTitle || '').toLowerCase();
        const match = !q || title.includes(q);
        link.hidden = !match;
        if (match) visible += 1;
      });
      group.hidden = visible === 0;
    });
  };
  input.addEventListener('input', apply);
  bindSlashSearch(input);
}

function bindDocsOnThisPage(root: HTMLElement) {
  const links = [...root.querySelectorAll<HTMLElement>('.docs-otp-link[data-docs-anchor]')];
  if (!links.length) return;

  const setActive = (id: string) => {
    links.forEach((el) => {
      el.classList.toggle('active', el.dataset.docsAnchor === id);
    });
  };

  root.querySelectorAll<HTMLElement>('[data-docs-anchor]').forEach((el) => {
    el.addEventListener('click', () => {
      const id = el.dataset.docsAnchor;
      const target = id ? document.getElementById(id) : null;
      if (!target) return;
      target.scrollIntoView({ behavior: 'smooth', block: 'start' });
      setActive(id!);
    });
  });

  const targets = links
    .map((el) => (el.dataset.docsAnchor ? document.getElementById(el.dataset.docsAnchor) : null))
    .filter((el): el is HTMLElement => !!el);
  if (!targets.length) return;

  const observer = new IntersectionObserver(
    (entries) => {
      const visible = entries
        .filter((e) => e.isIntersecting)
        .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top);
      if (visible[0]?.target?.id) setActive(visible[0].target.id);
    },
    { rootMargin: '-20% 0px -65% 0px', threshold: [0, 1] },
  );
  targets.forEach((t) => observer.observe(t));
}

export async function renderDocs(app: HTMLElement, sectionId?: string) {
  const { owner, repo } = repoBase();
  const isLanding = !sectionId;
  const active = isLanding ? undefined : docsById(sectionId);
  const resolved = active || (isLanding ? undefined : DOC_SECTIONS[0]);
  const groups = docsGroups();
  const repoLinks: Array<[string, string]> = [
    ['specification-to-enforcement.md', 'Lifecycle contract'],
    ['architecture.md', 'Runtime architecture'],
    ['workshop.md', 'Workshop / Pages'],
    ['kpi-dictionary.md', 'KPI dictionary'],
    ['obligation-review.md', 'Review registry'],
    ['language-support.md', 'Language / rule coverage'],
    ['analyzer-plan-manifest.md', 'Analyzer plans'],
    ['tool-adoption-decisions.md', 'Tool adoption'],
    ['scaling-adr-design.md', 'Scaling design'],
    ['scalable-architecture.md', 'Scalable architecture'],
  ];

  if (isLanding || !resolved) {
    setCrumb('Docs');
    setPurpose('In-app Pathfinder documentation — pipelines, assessment engine, governance, and operator reference.');
    setTopActions(`
      <a class="btn" href="#/sample">Sample journey</a>
      <a class="btn" href="https://github.com/${owner}/${repo}/tree/main/docs">Repo docs/</a>
    `);
    app.innerHTML = `
      <div class="docs-shell">
        <div class="docs-layout docs-layout-home">
          ${renderDocsNav(groups, null, owner, repo, repoLinks)}
          <div class="docs-main">
            ${renderDocsLanding(groups, owner, repo)}
          </div>
          <aside class="docs-otp docs-otp-empty" aria-hidden="true"></aside>
        </div>
      </div>`;
    bindDocsTocFilter(app);
    return;
  }

  // Unknown section id → fall back to first article (keeps old deep links working).
  const section = resolved;
  setCrumb(`Docs · ${section.title}`);
  setPurpose(section.summary);
  setTopActions(`
    <a class="btn" href="#/docs">Docs home</a>
    <a class="btn" href="#/sample">Sample journey</a>
    <a class="btn" href="https://github.com/${owner}/${repo}/tree/main/docs">Repo docs/</a>
  `);

  const idx = DOC_SECTIONS.findIndex((s) => s.id === section.id);
  const prev = idx > 0 ? DOC_SECTIONS[idx - 1] : undefined;
  const next = idx >= 0 && idx < DOC_SECTIONS.length - 1 ? DOC_SECTIONS[idx + 1] : undefined;
  const { html: bodyHtml, headings } = prepareDocsBody(section.body);

  app.innerHTML = `
    <div class="docs-shell">
      <div class="docs-layout${headings.length ? '' : ' docs-layout-no-otp'}">
        ${renderDocsNav(groups, section.id, owner, repo, repoLinks)}
        <div class="docs-main">
          ${renderDocsArticle(section, bodyHtml, prev, next)}
        </div>
        ${renderDocsOnThisPage(headings)}
      </div>
    </div>`;
  bindDocsTocFilter(app);
  bindDocsOnThisPage(app);
}

import uPlot from 'uplot';
import { marked } from 'marked';
import DOMPurify from 'dompurify';
import { createTwoFilesPatch } from 'diff';
import { html as diff2html } from 'diff2html';
import {
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
import { queryParam, setSelectedRunId } from './session';
import {
  bindCopyButtons,
  copyControl,
  emptyState,
  escapeHTML,
  fmt,
  fmtUSD,
  metricsRow,
  mountMetricsRow,
  setCrumb,
  setPurpose,
  setTopActions,
  statusItem,
  tag,
} from './ui';

function chart(el: HTMLElement, series: Array<{ label: string; stroke: string; data: number[] }>, xs: number[]) {
  el.innerHTML = '';
  if (!xs.length) {
    el.innerHTML = `<p class="muted">No trend data yet.</p>`;
    return;
  }
  const width = Math.max(320, Math.min(el.clientWidth || 720, 1040));
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
      legend: { show: true },
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
      href: '#/candidates',
    });
  }
  if (pendingProps.length) {
    triage.push({
      sev: 'warn',
      label: 'Proposals',
      msg: `${pendingProps.length} assist proposal(s) pending_review`,
      href: '#/proposals',
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

  app.innerHTML = `
    ${!ctx.artifactsFromSnapshot ? `<div class="banner warn">Metrics for <code>${escapeHTML(ctx.runId)}</code>; detailed artifacts fall back to the latest snapshot.</div>` : ''}
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
      { label: 'High / med / low', value: `${fmt(m.confidence_high)} / ${fmt(m.confidence_medium)} / ${fmt(m.confidence_low)}`, href: '#/metrics?focus=confidence' },
      { label: 'Escalation', value: fmt(m['kpi.escalation.rate']), href: '#/metrics?focus=escalation' },
      { label: 'Proposals', value: fmt(m['kpi.assist.proposal_count']), href: '#/proposals' },
      { label: 'Corpus', value: fmt(m['kpi.corpus.size']), href: '#/corpus' },
    ])}
    <h2>Needs attention</h2>
    <div class="inbox">
      ${triage
        .map(
          (t) => `<a class="inbox-item" href="${escapeHTML(t.href)}">
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
    <h2>Confidence trend</h2>
    <div id="spark" class="chart"></div>
  `;
  const el = document.getElementById('spark');
  if (el) {
    chart(
      el,
      [
        { label: 'high', stroke: '#3fae7a', data: runs.map((r) => Number(r.metrics.confidence_high || 0)) },
        { label: 'low', stroke: '#c9953a', data: runs.map((r) => Number(r.metrics.confidence_low || 0)) },
      ],
      runs.map((_, i) => i),
    );
  }
}

const METRIC_TABS = ['all', 'confidence', 'escalation', 'harness', 'review', 'assist', 'assess'] as const;
type MetricTab = (typeof METRIC_TABS)[number];

function parseMetricFocus(raw: string | null): MetricTab {
  return (METRIC_TABS as readonly string[]).includes(raw || '') ? (raw as MetricTab) : 'all';
}

export async function renderMetrics(app: HTMLElement) {
  setCrumb('Metrics');
  setPurpose('KPI trends for the selected focus. Assist and Promptfoo values are quality signals, not compliance.');
  setTopActions(`<a class="btn" href="#/runs">Compare runs</a>`);
  const params = new URLSearchParams(location.hash.split('?')[1] || '');
  const focus = parseMetricFocus(params.get('focus'));
  const [runs, latest] = await Promise.all([loadRuns(), loadLatest()]);
  const m = latest?.metrics || {};

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
      location.hash = `#/metrics?focus=${encodeURIComponent(t)}`;
    });
    tabHost.appendChild(btn);
  }
  toolbar.appendChild(tabHost);
  app.appendChild(toolbar);

  mountMetricsRow(app, focusMetrics[focus]);

  const trendHeading = document.createElement('h2');
  trendHeading.textContent = 'Trend';
  app.appendChild(trendHeading);
  const el = document.createElement('div');
  el.id = 'm-chart';
  el.className = 'chart tall';
  app.appendChild(el);

  const rawHeading = document.createElement('h2');
  rawHeading.textContent = 'Raw latest metrics';
  app.appendChild(rawHeading);
  const artifact = document.createElement('div');
  artifact.className = 'artifact';
  const pre = document.createElement('pre');
  pre.textContent = JSON.stringify(m, null, 2);
  artifact.appendChild(pre);
  app.appendChild(artifact);

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
  setPurpose('One stage at a time: contract, behavior, failure modes, and a sample artifact.');
  setTopActions(`<a class="btn" href="#/sample">Open Sample journey</a>`);
  const data = await loadPipeline();
  const steps = data?.steps || [];
  const active = stepId ? steps.find((s) => s.id === stepId) : steps[0];
  if (!active) {
    app.innerHTML = emptyState('No pipeline metadata', 'pipeline.json missing from snapshot.');
    return;
  }
  app.innerHTML = `
    <div class="steps">
      ${steps
        .map(
          (s, i) =>
            `${i ? '<span class="step-sep">/</span>' : ''}<a class="step-link ${active.id === s.id ? 'active' : ''}" href="#/pipeline/${escapeHTML(s.id)}">${escapeHTML(s.title)}</a>`,
        )
        .join('')}
    </div>
    <div class="layout-2">
      <section class="layout-aside">
        <h1>${escapeHTML(active.title)}</h1>
        <p class="prose" style="margin-top:0.5rem">${escapeHTML(active.deep.what)}</p>
        ${active.command ? `<p style="margin-top:0.75rem"><code>${escapeHTML(active.command)}</code></p>` : ''}
        ${active.invariant ? `<p style="margin-top:0.5rem">${tag(active.invariant, 'ok')}</p>` : ''}
        <h2>How it works</h2>
        <ul>${active.deep.how.map((x) => `<li>${escapeHTML(x)}</li>`).join('')}</ul>
        <h2>Failure modes</h2>
        <ul>${active.deep.failure_modes.map((x) => `<li>${escapeHTML(x)}</li>`).join('')}</ul>
        <h2>Related metrics</h2>
        <p class="muted">${active.deep.metrics.map((x) => `<code>${escapeHTML(x)}</code>`).join(' · ')}</p>
      </section>
      <aside class="layout-aside">
        <h2>I/O</h2>
        <dl class="kv">
          <dt>Inputs</dt><dd>${active.inputs.map((x) => `<code>${escapeHTML(x)}</code>`).join('<br/>') || '—'}</dd>
          <dt>Outputs</dt><dd>${active.outputs.map((x) => `<code>${escapeHTML(x)}</code>`).join('<br/>') || '—'}</dd>
        </dl>
        <h2>Sample artifact</h2>
        <div class="artifact"><pre>${escapeHTML(JSON.stringify(active.deep.sample_artifact ?? {}, null, 2))}</pre></div>
      </aside>
    </div>
  `;
}

export async function renderADRs(app: HTMLElement, id?: string) {
  if (id) return renderADRDetail(app, id);
  setCrumb('ADRs');
  setPurpose('Browse published ADR sources and jump into linked candidates.');
  setTopActions('');
  const index = await loadADRIndex();
  const adrs = index?.adrs || [];
  app.innerHTML = `
    <div class="toolbar">
      <input class="search" id="adr-q" placeholder="Filter by id or title" />
      <span class="faint">${adrs.length} documents</span>
    </div>
    <div class="table-wrap">
      <table>
        <thead><tr><th>ID</th><th>Title</th><th>Status</th><th>Mix</th><th>Candidates</th></tr></thead>
        <tbody id="adr-rows">
          ${adrs
            .map(
              (a) => `<tr class="row-link" data-id="${escapeHTML(a.id)}">
              <td><code class="id" title="${escapeHTML(a.id)}">${escapeHTML(a.id)}</code></td>
              <td class="truncate" title="${escapeHTML(a.title)}">${escapeHTML(a.title)}</td>
              <td>${tag(a.status, a.status === 'accepted' ? 'ok' : '')}</td>
              <td>${a.confidence_mix ? `${a.confidence_mix.high}/${a.confidence_mix.medium}/${a.confidence_mix.low}` : '—'}</td>
              <td>${(a.candidate_ids || []).length}</td>
            </tr>`,
            )
            .join('')}
        </tbody>
      </table>
    </div>
  `;
  const rows = () => [...document.querySelectorAll('#adr-rows tr')] as HTMLElement[];
  rows().forEach((tr) => tr.addEventListener('click', () => { location.hash = `#/adrs/${tr.dataset.id}`; }));
  document.getElementById('adr-q')?.addEventListener('input', (e) => {
    const q = (e.target as HTMLInputElement).value.toLowerCase();
    rows().forEach((tr) => {
      tr.style.display = tr.textContent?.toLowerCase().includes(q) ? '' : 'none';
    });
  });
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
      <a href="#/candidates">Candidates</a>
      <a href="#/proposals">Proposals</a>
      <a href="#/assess">Assess</a>
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
                  `<tr><td><code>${escapeHTML(String(c.id))}</code></td><td>${escapeHTML(String(c.confidence))}</td><td class="wrap">${escapeHTML(String(c.statement))}</td></tr>`,
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
  setPurpose('Filter extracted obligations by confidence band and open a review checklist.');
  const focusId = queryParam('id');
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
  const low = rows.filter((c) => c.confidence === 'low');
  const body = [
    '## Low-confidence checklist',
    '',
    'Generated from Pathfinder.',
    '',
    ...low.map((c) => `- [ ] ${c.id} (${c.adr_id}): ${c.statement}`),
  ].join('\n');
  setTopActions(
    `${copyControl(body, 'Copy checklist')}<a class="btn primary" href="${issueURL('[ADR Assessment] Low-confidence review batch', body)}">Open checklist issue</a>`,
  );
  app.innerHTML = `
    ${metricsRow([
      { label: 'Total', value: fmt(rows.length) },
      { label: 'High', value: fmt(data?.confidence_summary?.high ?? rows.filter((c) => c.confidence === 'high').length), cls: 'ok' },
      { label: 'Medium', value: fmt(data?.confidence_summary?.medium ?? rows.filter((c) => c.confidence === 'medium').length) },
      { label: 'Low', value: fmt(data?.confidence_summary?.low ?? low.length), cls: 'warn' },
    ])}
    <div class="toolbar">
      <div class="seg" id="band-seg">
        <button type="button" class="active" data-band="all">All</button>
        <button type="button" data-band="high">High</button>
        <button type="button" data-band="medium">Medium</button>
        <button type="button" data-band="low">Low</button>
      </div>
      <input class="search" id="cand-q" placeholder="Search id or statement" value="${escapeHTML(focusId || '')}" />
    </div>
    <div class="table-wrap">
      <table>
        <thead><tr><th>ID</th><th>ADR</th><th>Band</th><th>Links</th><th>Pattern</th><th>Statement</th></tr></thead>
        <tbody id="cand-body">
          ${rows
            .map((c) => {
              const prop = propByParent.get(c.id);
              return `<tr data-band="${escapeHTML(c.confidence)}" data-q="${escapeHTML((c.id + ' ' + c.statement).toLowerCase())}">
              <td><code class="id" title="${escapeHTML(c.id)}">${escapeHTML(c.id)}</code> ${copyControl(c.id, 'Copy')}</td>
              <td><a class="id" href="#/adrs/${escapeHTML(c.adr_id)}" title="${escapeHTML(c.adr_id)}">${escapeHTML(c.adr_id)}</a></td>
              <td>${tag(c.confidence, c.confidence === 'low' ? 'warn' : c.confidence === 'high' ? 'ok' : '')}</td>
              <td class="entity-links">
                <a href="#/adrs/${escapeHTML(c.adr_id)}">ADR</a>
                ${prop ? `<a href="#/proposals?parent=${escapeHTML(c.id)}">Proposal</a>` : '<span class="faint">No proposal</span>'}
                <a href="#/assess">Assess</a>
              </td>
              <td><code class="id" title="${escapeHTML(c.pattern_hint)}">${escapeHTML(c.pattern_hint)}</code></td>
              <td class="wrap">${escapeHTML(c.statement)}</td>
            </tr>`;
            })
            .join('')}
        </tbody>
      </table>
    </div>
  `;
  let band = 'all';
  const apply = () => {
    const q = (document.getElementById('cand-q') as HTMLInputElement)?.value.toLowerCase() || '';
    document.querySelectorAll('#cand-body tr').forEach((tr) => {
      const el = tr as HTMLElement;
      const okBand = band === 'all' || el.dataset.band === band;
      const okQ = !q || (el.dataset.q || '').includes(q);
      el.style.display = okBand && okQ ? '' : 'none';
    });
  };
  document.querySelectorAll('#band-seg button').forEach((btn) => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('#band-seg button').forEach((b) => b.classList.remove('active'));
      btn.classList.add('active');
      band = (btn as HTMLElement).dataset.band || 'all';
      apply();
    });
  });
  document.getElementById('cand-q')?.addEventListener('input', apply);
  apply();
  bindCopyButtons(app);
}

export async function renderProposals(app: HTMLElement) {
  setCrumb('Proposals');
  setPurpose('Review-only assist drafts. Accept via accept-assist-proposal — never auto-approved.');
  setTopActions(`<a class="btn" href="#/costs">Cost detail</a>`);
  const data = await loadLinkJSON<{
    provider?: string;
    model?: string;
    prompt_tokens?: number;
    completion_tokens?: number;
    estimated_cost_usd?: number;
    proposal_count?: number;
    proposals?: Array<{
      proposal_id: string;
      parent_candidate_id: string;
      statement: string;
      normalized_statement: string;
      suggested_pattern_id: string;
      rationale: string;
      parent_confidence?: string;
      confidence?: string;
      executable: boolean;
      approval_status: string;
    }>;
  }>('proposals');
  const parentFilter = queryParam('parent');
  const proposals = (data?.proposals || []).filter((p) => !parentFilter || p.parent_candidate_id === parentFilter);
  const latest = await loadLatest();
  const cands = await loadLinkJSON<{ candidates?: Array<{ id: string; adr_id: string }> }>('candidates');
  const adrByCand = new Map((cands?.candidates || []).map((c) => [c.id, c.adr_id]));
  app.innerHTML = `
    ${parentFilter ? `<div class="banner">Filtered to parent <code>${escapeHTML(parentFilter)}</code> · <a href="#/proposals">Clear</a></div>` : ''}
    ${metricsRow([
      { label: 'Proposals', value: fmt(data?.proposal_count ?? proposals.length) },
      { label: 'Provider', value: fmt(data?.provider || latest?.metrics['kpi.assist.provider']) },
      { label: 'Est. cost', value: fmtUSD(data?.estimated_cost_usd ?? latest?.metrics['kpi.assist.cost_usd']) },
      { label: 'Accept rate', value: fmt(latest?.metrics['kpi.review.accept_rate']) },
    ])}
    <div id="prop-list"></div>
  `;
  const list = document.getElementById('prop-list')!;
  if (!proposals.length) {
    list.innerHTML = `<p class="muted" style="margin-top:1rem">No proposals in the selected snapshot.</p>`;
    return;
  }
  for (const p of proposals) {
    const panel = document.createElement('div');
    panel.className = 'proposal';
    const patch = createTwoFilesPatch(
      'candidate',
      'proposal',
      `${p.statement}\n`,
      `${p.normalized_statement}\n`,
      '',
      '',
      { context: 4 },
    );
    const adr = adrByCand.get(p.parent_candidate_id);
    panel.innerHTML = `
      <div class="meta">
        <code>${escapeHTML(p.proposal_id)}</code>
        ${copyControl(p.proposal_id, 'Copy')}
        <span>${tag(p.approval_status)}</span>
        <span>${tag(`executable=${p.executable}`, p.executable ? 'bad' : 'ok')}</span>
      </div>
      <div class="entity-links">
        <a href="#/candidates?id=${escapeHTML(p.parent_candidate_id)}">Candidate ${escapeHTML(p.parent_candidate_id)}</a>
        ${adr ? `<a href="#/adrs/${escapeHTML(adr)}">ADR ${escapeHTML(adr)}</a>` : ''}
        <a href="#/assess">Assess</a>
        <a href="#/costs">Cost</a>
      </div>
      <p class="prose">${escapeHTML(p.rationale)}</p>
      <p class="faint" style="margin:0.45rem 0">Suggested pattern: <code>${escapeHTML(p.suggested_pattern_id)}</code></p>
      <div class="diff">${diff2html(patch, { drawFileList: false, matching: 'lines', outputFormat: 'side-by-side' })}</div>`;
    list.appendChild(panel);
  }
  bindCopyButtons(list);
}

export async function renderAssess(app: HTMLElement) {
  setCrumb('Assessment');
  setPurpose('Deterministic assess gate. This path must never call an LLM.');
  const assessment = await loadLinkJSON<{
    verdict?: string;
    runtime_llm_calls?: number;
    coverage_status?: string;
    requirements?: Array<{ requirement_id: string; status: string; title?: string }>;
  }>('assessment');
  const latest = await loadLatest();
  const failing = (assessment?.requirements || []).filter((r) => r.status === 'FAIL' || r.status === 'REVIEW_REQUIRED');
  const body = [
    '## Assessment follow-up',
    '',
    `Verdict: ${assessment?.verdict || latest?.metrics['kpi.assess.verdict'] || 'unknown'}`,
    `Runtime LLM calls: ${assessment?.runtime_llm_calls ?? latest?.metrics['kpi.invariant.llm_calls'] ?? 'n/a'}`,
    '',
    ...failing.map((r) => `- [ ] ${r.requirement_id}: ${r.title || r.status}`),
  ].join('\n');
  setTopActions(`<a class="btn primary" href="${issueURL('[ADR Assessment] Follow-up from workshop', body)}">Open follow-up issue</a>`);
  const llm = assessment?.runtime_llm_calls ?? Number(latest?.metrics['kpi.invariant.llm_calls'] ?? 0);
  app.innerHTML = `
    <div class="status-row">
      ${statusItem('Verdict', String(assessment?.verdict || latest?.metrics['kpi.assess.verdict'] || '—'), assessment?.verdict === 'PASS' || latest?.metrics['kpi.assess.verdict'] === 'PASS' ? 'ok' : 'warn')}
      ${statusItem('Runtime LLM calls', fmt(llm), llm === 0 ? 'ok' : 'bad')}
      ${statusItem('Coverage', String(assessment?.coverage_status || '—'), '')}
    </div>
    ${metricsRow([
      { label: 'Covered', value: fmt(latest?.metrics['kpi.assess.covered']) },
      { label: 'Unbound', value: fmt(latest?.metrics['kpi.assess.unbound']) },
      { label: 'Stale', value: fmt(latest?.metrics['kpi.assess.stale']) },
      { label: 'Failing reqs', value: fmt(failing.length), cls: failing.length ? 'warn' : 'ok' },
    ])}
    <h2>Assessment artifact</h2>
    <div class="artifact"><pre>${escapeHTML(JSON.stringify(assessment || latest?.metrics || {}, null, 2))}</pre></div>
  `;
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
    <h2>Cost over runs</h2>
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
  setPurpose('History and a side-by-side KPI compare between two run ids.');
  setTopActions('');
  const runs = await loadRuns();
  const ordered = runs.slice().reverse();
  const options = ordered
    .map((r) => `<option value="${escapeHTML(r.run.run_id)}">${escapeHTML(r.run.run_id)} · ${escapeHTML(r.run.workflow)}</option>`)
    .join('');
  app.innerHTML = `
    <div class="toolbar">
      <label class="muted">A <select id="run-a">${options}</select></label>
      <label class="muted">B <select id="run-b">${options}</select></label>
      <button type="button" class="btn primary" id="run-compare-btn">Compare</button>
    </div>
    <div id="run-compare-out" class="muted" style="margin-bottom:1rem">Select two runs to compare.</div>
    <div class="table-wrap">
      <table>
        <thead><tr><th>Run</th><th>Workflow</th><th>When</th><th>Candidates</th><th>Low</th><th>Cost</th><th>Verdict</th></tr></thead>
        <tbody>${ordered
          .map(
            (r) => `<tr class="row-link" data-run="${escapeHTML(r.run.run_id)}">
          <td><code class="id" title="${escapeHTML(r.run.run_id)}">${escapeHTML(r.run.run_id)}</code></td>
          <td class="truncate">${escapeHTML(r.run.workflow)}</td>
          <td class="truncate">${escapeHTML(r.run.timestamp || '')}</td>
          <td>${fmt(r.metrics.candidate_total)}</td>
          <td>${fmt(r.metrics.confidence_low)}</td>
          <td>${fmtUSD(r.metrics['kpi.assist.cost_usd'])}</td>
          <td>${fmt(r.metrics['kpi.assess.verdict'])}</td>
        </tr>`,
          )
          .join('')}</tbody>
      </table>
    </div>
  `;
  document.querySelectorAll<HTMLElement>('#app tr.row-link[data-run]').forEach((tr) => {
    tr.addEventListener('click', () => {
      const id = tr.dataset.run;
      if (!id) return;
      setSelectedRunId(id);
      location.hash = '#/';
    });
  });
  const a = document.getElementById('run-a') as HTMLSelectElement | null;
  const b = document.getElementById('run-b') as HTMLSelectElement | null;
  if (a && b && ordered.length >= 2) {
    a.selectedIndex = Math.min(1, ordered.length - 1);
    b.selectedIndex = 0;
  }
  document.getElementById('run-compare-btn')?.addEventListener('click', () => {
    const out = document.getElementById('run-compare-out');
    if (!out || !a || !b) return;
    const left = runs.find((r) => r.run.run_id === a.value);
    const right = runs.find((r) => r.run.run_id === b.value);
    if (!left || !right || left.run.run_id === right.run.run_id) {
      out.textContent = 'Choose two different run ids.';
      return;
    }
    const keys = [...new Set([...Object.keys(left.metrics), ...Object.keys(right.metrics)])].sort();
    out.innerHTML = `<div class="table-wrap"><table><thead><tr><th>Metric</th><th>A</th><th>B</th></tr></thead><tbody>${keys
      .map((k) => {
        const av = left.metrics[k];
        const bv = right.metrics[k];
        const changed = JSON.stringify(av) !== JSON.stringify(bv);
        return `<tr><td><code>${escapeHTML(k)}</code></td><td class="${changed ? 'warn' : ''}">${escapeHTML(fmt(av))}</td><td class="${changed ? 'warn' : ''}">${escapeHTML(fmt(bv))}</td></tr>`;
      })
      .join('')}</tbody></table></div>`;
  });
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
  setPurpose('Browse labeled corpus entries, filter by confidence band, and jump to source ADRs on GitHub.');
  const [latest, corpus] = await Promise.all([loadLatest(), loadCorpusIndex()]);
  const { owner, repo } = repoBase();
  const entries = corpus?.entries || [];
  setTopActions(`
    <a class="btn" href="#/adrs">Published ADRs</a>
    <a class="btn primary" href="${workflowDispatchURL('corpus-eval.yml')}">Run corpus eval</a>
  `);
  app.innerHTML = `
    ${metricsRow([
      { label: 'Corpus size', value: fmt(corpus?.corpus_size ?? latest?.metrics['kpi.corpus.size'] ?? entries.length) },
      { label: 'Precision', value: fmt(latest?.metrics['kpi.harness.precision']) },
      { label: 'Recall', value: fmt(latest?.metrics['kpi.harness.recall']) },
      { label: 'Recall H/M/L', value: `${fmt(latest?.metrics['kpi.harness.recall.high'])} / ${fmt(latest?.metrics['kpi.harness.recall.medium'])} / ${fmt(latest?.metrics['kpi.harness.recall.low'])}` },
      { label: 'Escalate labels', value: fmt(entries.filter((e) => e.escalate_expected).length) },
    ])}
    <div class="toolbar">
      <div class="seg" id="corpus-band">
        <button type="button" class="active" data-band="all">All</button>
        <button type="button" data-band="high">High</button>
        <button type="button" data-band="medium">Medium</button>
        <button type="button" data-band="low">Low</button>
      </div>
      <input class="search" id="corpus-q" placeholder="Search ADR id or statement marker" />
      <a class="btn" href="https://github.com/${owner}/${repo}/compare">Open labeling PR</a>
    </div>
    <div class="table-wrap">
      <table>
        <thead><tr><th>ADR</th><th>Band</th><th>Modality</th><th>Pattern</th><th>Escalate</th><th>Marker</th></tr></thead>
        <tbody id="corpus-body">
          ${entries
            .map(
              (e) => `<tr data-band="${escapeHTML(e.confidence_band)}" data-q="${escapeHTML((e.adr_id + ' ' + e.statement_contains).toLowerCase())}">
              <td><a class="id" href="${githubBlobURL(`testdata/corpus/adrs/${e.adr_id}.md`)}" title="${escapeHTML(e.adr_id)}">${escapeHTML(e.adr_id)}</a></td>
              <td>${tag(e.confidence_band, e.confidence_band === 'low' ? 'warn' : e.confidence_band === 'high' ? 'ok' : '')}</td>
              <td>${escapeHTML(e.modality)}</td>
              <td><code class="id" title="${escapeHTML(e.pattern_id)}">${escapeHTML(e.pattern_id)}</code></td>
              <td>${e.escalate_expected ? tag('yes', 'warn') : tag('no')}</td>
              <td class="wrap">${escapeHTML(e.statement_contains)}</td>
            </tr>`,
            )
            .join('')}
        </tbody>
      </table>
    </div>
    <p class="run-meta" style="margin-top:1rem">Source: <code>${escapeHTML(corpus?.source || 'testdata/corpus/labels.json')}</code> · label via PR with <code>[corpus-label]</code>.</p>
  `;
  let band = 'all';
  const apply = () => {
    const q = (document.getElementById('corpus-q') as HTMLInputElement)?.value.toLowerCase() || '';
    document.querySelectorAll('#corpus-body tr').forEach((tr) => {
      const el = tr as HTMLElement;
      const okBand = band === 'all' || el.dataset.band === band;
      const okQ = !q || (el.dataset.q || '').includes(q);
      el.style.display = okBand && okQ ? '' : 'none';
    });
  };
  document.querySelectorAll('#corpus-band button').forEach((btn) => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('#corpus-band button').forEach((b) => b.classList.remove('active'));
      btn.classList.add('active');
      band = (btn as HTMLElement).dataset.band || 'all';
      apply();
    });
  });
  document.getElementById('corpus-q')?.addEventListener('input', apply);
}

export async function renderDocs(app: HTMLElement) {
  setCrumb('Docs');
  setPurpose('Source documentation in the repository.');
  setTopActions('');
  const { owner, repo } = repoBase();
  const links = [
    ['Workshop architecture', `https://github.com/${owner}/${repo}/blob/main/docs/workshop.md`],
    ['KPI dictionary', `https://github.com/${owner}/${repo}/blob/main/docs/kpi-dictionary.md`],
    ['Architecture (Joern N=10)', `https://github.com/${owner}/${repo}/blob/main/docs/architecture.md`],
    ['Promptfoo eval', `https://github.com/${owner}/${repo}/blob/main/eval/promptfoo/README.md`],
  ];
  app.innerHTML = `
    <div class="table-wrap">
      <table>
        <thead><tr><th>Document</th><th>Link</th></tr></thead>
        <tbody>
          ${links.map(([label, href]) => `<tr><td>${escapeHTML(label)}</td><td><a href="${href}">Open on GitHub</a></td></tr>`).join('')}
        </tbody>
      </table>
    </div>
    <p class="run-meta" style="margin-top:1rem">Joern remains a follow-on when ≥10 accepted ADRs need data-flow evidence OpenGrep cannot supply.</p>
  `;
}

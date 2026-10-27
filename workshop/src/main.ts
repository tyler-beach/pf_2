import uPlot from 'uplot';
import 'uplot/dist/uPlot.min.css';
import { marked } from 'marked';
import DOMPurify from 'dompurify';
import { createTwoFilesPatch } from 'diff';
import { html as diff2html } from 'diff2html';
import 'diff2html/bundles/css/diff2html.min.css';
import {
  issueURL,
  loadLatest,
  loadLinkJSON,
  loadManifest,
  loadRuns,
  repoBase,
  workflowDispatchURL,
  type MetricsRow,
} from './data';

const routes = [
  ['/', 'Home'],
  ['/runs', 'Runs'],
  ['/kpis', 'KPIs'],
  ['/corpus', 'Corpus'],
  ['/candidates', 'Candidates'],
  ['/proposals', 'Proposals'],
  ['/assess', 'Assess'],
  ['/evals', 'Evals'],
  ['/docs', 'Docs'],
] as const;

function route(): string {
  const h = location.hash.replace(/^#/, '') || '/';
  return h.startsWith('/') ? h : `/${h}`;
}

function nav() {
  const el = document.getElementById('nav')!;
  const current = route();
  el.innerHTML = routes
    .map(([path, label]) => `<a href="#${path}" class="${current === path ? 'active' : ''}">${label}</a>`)
    .join('');
}

function tile(label: string, value: string, cls = ''): string {
  return `<div class="tile"><div class="label">${label}</div><div class="value ${cls}">${value}</div></div>`;
}

function fmt(v: unknown): string {
  if (v == null) return '—';
  if (typeof v === 'number') return Number.isInteger(v) ? String(v) : v.toFixed(3);
  if (typeof v === 'boolean') return v ? 'yes' : 'no';
  return String(v);
}

function escapeHTML(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function drawConfidenceTrend(el: HTMLElement, runs: MetricsRow[]) {
  if (!runs.length) {
    el.innerHTML = `<p class="muted">No run history yet.</p>`;
    return;
  }
  const xs = runs.map((_, i) => i);
  const high = runs.map((r) => Number(r.metrics.confidence_high || 0));
  const low = runs.map((r) => Number(r.metrics.confidence_low || 0));
  new uPlot(
    {
      width: el.clientWidth || 800,
      height: 220,
      series: [{}, { label: 'high', stroke: '#3dd68c' }, { label: 'low', stroke: '#f5a524' }],
      axes: [{ stroke: '#9aa7b5' }, { stroke: '#9aa7b5' }],
    },
    [xs, high, low],
    el,
  );
}

async function renderHome(app: HTMLElement) {
  const [latest, manifest, runs] = await Promise.all([loadLatest(), loadManifest(), loadRuns()]);
  if (!latest) {
    app.innerHTML = `<h1>Pathfinder Workshop</h1><p class="muted">No published data yet. Run CI / workshop-publish.</p>`;
    return;
  }
  const m = latest.metrics;
  const sloLLM = latest.slo.assess_runtime_llm_calls_zero === true;
  const sloPF = latest.slo.promptfoo_offline_pass !== false;
  app.innerHTML = `
    <h1>Workshop home</h1>
    <p class="muted">${escapeHTML(latest.summary)}</p>
    <p class="muted">Latest run <code>${escapeHTML(latest.run.run_id)}</code> · ${escapeHTML(latest.run.workflow)} · ${escapeHTML(latest.run.timestamp)}${manifest ? ` · data ${escapeHTML(manifest.updated_at)}` : ''}</p>
    <div class="grid">
      ${tile('Assess LLM calls = 0', sloLLM ? 'OK' : 'FAIL', sloLLM ? 'ok' : 'bad')}
      ${tile('Promptfoo offline', sloPF ? 'OK' : 'CHECK', sloPF ? 'ok' : 'warn')}
      ${tile('Candidates', fmt(m.candidate_total))}
      ${tile('Low / escalate', fmt(m.confidence_low))}
      ${tile('Escalation rate', fmt(m['kpi.escalation.rate']))}
      ${tile('Assess verdict', fmt(m['kpi.assess.verdict']))}
      ${tile('Proposals', fmt(m['kpi.assist.proposal_count']))}
      ${tile('Promptfoo pass', fmt(m['kpi.promptfoo.pass_rate']))}
    </div>
    <div class="actions">
      <a class="btn" href="${workflowDispatchURL('llm-assist.yml')}">Run LLM assist</a>
      <a class="btn secondary" href="${workflowDispatchURL('corpus-eval.yml')}">Run corpus eval</a>
      ${latest.run.html_url ? `<a class="btn secondary" href="${latest.run.html_url}">Source workflow run</a>` : ''}
    </div>
    <h2>Confidence trend</h2>
    <div id="spark" class="chart panel"></div>
  `;
  drawConfidenceTrend(document.getElementById('spark')!, runs);
}

function metricKeys(rows: MetricsRow[]): string[] {
  const keys = new Set<string>();
  for (const r of rows) {
    for (const k of Object.keys(r.metrics || {})) keys.add(k);
  }
  return [...keys].sort();
}

function findRun(runs: MetricsRow[], id: string): MetricsRow | undefined {
  return runs.find((r) => r.run.run_id === id);
}

async function renderRuns(app: HTMLElement) {
  const runs = await loadRuns();
  const ordered = runs.slice().reverse();
  const options = ordered
    .map((r) => `<option value="${escapeHTML(r.run.run_id)}">${escapeHTML(r.run.run_id)} · ${escapeHTML(r.run.workflow)}</option>`)
    .join('');
  app.innerHTML = `<h1>Run history</h1>
    <div class="panel">
      <h2>Compare runs</h2>
      <p class="muted">Pick two run ids to diff KPI fields (A → B).</p>
      <div class="actions">
        <label>A <select id="run-a">${options}</select></label>
        <label>B <select id="run-b">${options}</select></label>
        <button type="button" class="btn" id="run-compare-btn">Compare</button>
      </div>
      <div id="run-compare-out" class="muted">Select two runs and compare.</div>
    </div>
    <table><thead><tr><th>Run</th><th>Workflow</th><th>When</th><th>Candidates</th><th>Low</th><th>Verdict</th></tr></thead>
    <tbody>${ordered
      .map(
        (r) => `<tr>
      <td><code>${escapeHTML(r.run.run_id)}</code></td>
      <td>${escapeHTML(r.run.workflow)}</td>
      <td>${escapeHTML(r.run.timestamp || '')}</td>
      <td>${fmt(r.metrics.candidate_total)}</td>
      <td>${fmt(r.metrics.confidence_low)}</td>
      <td>${fmt(r.metrics['kpi.assess.verdict'])}</td>
    </tr>`,
      )
      .join('')}</tbody></table>`;
  const a = document.getElementById('run-a') as HTMLSelectElement | null;
  const b = document.getElementById('run-b') as HTMLSelectElement | null;
  if (a && b && ordered.length >= 2) {
    a.selectedIndex = Math.min(1, ordered.length - 1);
    b.selectedIndex = 0;
  }
  document.getElementById('run-compare-btn')?.addEventListener('click', () => {
    const out = document.getElementById('run-compare-out');
    if (!out || !a || !b) return;
    const left = findRun(runs, a.value);
    const right = findRun(runs, b.value);
    if (!left || !right) {
      out.textContent = 'Could not load selected runs.';
      return;
    }
    if (left.run.run_id === right.run.run_id) {
      out.textContent = 'Choose two different run ids.';
      return;
    }
    const keys = metricKeys([left, right]);
    const rows = keys
      .map((k) => {
        const av = left.metrics[k];
        const bv = right.metrics[k];
        const changed = JSON.stringify(av) !== JSON.stringify(bv);
        return `<tr class="${changed ? 'warn' : ''}"><td><code>${escapeHTML(k)}</code></td><td>${escapeHTML(fmt(av))}</td><td>${escapeHTML(fmt(bv))}</td></tr>`;
      })
      .join('');
    out.innerHTML = `<p><code>${escapeHTML(left.run.run_id)}</code> → <code>${escapeHTML(right.run.run_id)}</code></p>
      <table><thead><tr><th>Metric</th><th>A</th><th>B</th></tr></thead><tbody>${rows}</tbody></table>`;
  });
}

async function renderKPIs(app: HTMLElement) {
  const runs = await loadRuns();
  const latest = await loadLatest();
  app.innerHTML = `<h1>KPI trends</h1>
    <p class="muted">See <code>docs/kpi-dictionary.md</code>. Promptfoo pass rate is assist quality only — not a compliance verdict.</p>
    <div id="kpi-chart" class="chart panel"></div>
    <div class="panel"><pre>${escapeHTML(JSON.stringify(latest?.metrics || {}, null, 2))}</pre></div>`;
  const el = document.getElementById('kpi-chart')!;
  if (!runs.length) return;
  const xs = runs.map((_, i) => i);
  const esc = runs.map((r) => Number(r.metrics['kpi.escalation.rate'] || 0));
  const pf = runs.map((r) => Number(r.metrics['kpi.promptfoo.pass_rate'] || 0));
  new uPlot(
    {
      width: el.clientWidth || 800,
      height: 220,
      series: [{}, { label: 'escalation_rate', stroke: '#f5a524' }, { label: 'promptfoo_pass', stroke: '#3d8bfd' }],
      axes: [{ stroke: '#9aa7b5' }, { stroke: '#9aa7b5' }],
    },
    [xs, esc, pf],
    el,
  );
}

async function renderCandidates(app: HTMLElement) {
  const data = await loadLinkJSON<{
    candidates?: Array<{
      id: string;
      adr_id: string;
      confidence: string;
      escalation_reasons?: string[];
      statement: string;
      pattern_hint: string;
    }>;
  }>('candidates');
  const rows = data?.candidates || [];
  const low = rows.filter((c) => c.confidence === 'low');
  const body = [
    '## Low-confidence checklist',
    '',
    'Generated from Pathfinder Workshop. Review these candidates; do not treat assist proposals as approvals.',
    '',
    ...low.map((c) => `- [ ] ${c.id} (${c.adr_id}): ${c.statement}`),
  ].join('\n');
  app.innerHTML = `<h1>Candidates</h1>
    <div class="actions">
      <a class="btn" href="${issueURL('[ADR Assessment] Low-confidence review batch', body)}">Open low-confidence checklist issue</a>
    </div>
    <table><thead><tr><th>ID</th><th>ADR</th><th>Band</th><th>Pattern</th><th>Reasons</th><th>Statement</th></tr></thead>
    <tbody>${rows
      .map(
        (c) => `<tr>
      <td><code>${escapeHTML(c.id)}</code></td><td>${escapeHTML(c.adr_id)}</td><td>${escapeHTML(c.confidence)}</td>
      <td>${escapeHTML(c.pattern_hint)}</td><td>${escapeHTML((c.escalation_reasons || []).join(', '))}</td>
      <td>${escapeHTML(c.statement)}</td></tr>`,
      )
      .join('')}</tbody></table>`;
}

async function renderProposals(app: HTMLElement) {
  const data = await loadLinkJSON<{
    proposals?: Array<{
      proposal_id: string;
      parent_candidate_id: string;
      statement: string;
      normalized_statement: string;
      suggested_pattern_id: string;
      rationale: string;
      parent_confidence: string;
      executable: boolean;
      approval_status: string;
    }>;
  }>('proposals');
  const proposals = data?.proposals || [];
  app.innerHTML = `<h1>Assist proposals</h1>
    <p class="muted">Proposals only — never auto-approved, never executable. Accept via <code>record-obligation-review</code>.</p>
    <div id="prop-list"></div>`;
  const list = document.getElementById('prop-list')!;
  if (!proposals.length) {
    list.innerHTML = `<p class="muted">No proposals in latest snapshot.</p>`;
    return;
  }
  for (const p of proposals) {
    const panel = document.createElement('div');
    panel.className = 'panel';
    const patch = createTwoFilesPatch(
      'candidate',
      'proposal',
      `${p.statement}\n`,
      `${p.normalized_statement}\n`,
      '',
      '',
      { context: 4 },
    );
    const diffHTML = diff2html(patch, {
      drawFileList: false,
      matching: 'lines',
      outputFormat: 'side-by-side',
    });
    panel.innerHTML = `<h2>${escapeHTML(p.proposal_id)}</h2>
      <p class="muted">${escapeHTML(p.parent_candidate_id)} · ${escapeHTML(p.parent_confidence)} · ${escapeHTML(p.suggested_pattern_id)} · ${escapeHTML(p.approval_status)} · executable=${p.executable}</p>
      <p>${escapeHTML(p.rationale)}</p>
      <div class="diff">${diffHTML}</div>`;
    list.appendChild(panel);
  }
}

async function renderAssess(app: HTMLElement) {
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
  const llm = assessment?.runtime_llm_calls ?? Number(latest?.metrics['kpi.invariant.llm_calls'] ?? 0);
  app.innerHTML = `<h1>Assessment</h1>
    <div class="grid">
      ${tile('Verdict', fmt(assessment?.verdict || latest?.metrics['kpi.assess.verdict']))}
      ${tile('LLM calls', fmt(llm), llm === 0 ? 'ok' : 'bad')}
      ${tile('Coverage', fmt(assessment?.coverage_status))}
    </div>
    <div class="actions">
      <a class="btn" href="${issueURL('[ADR Assessment] Follow-up from workshop', body)}">Open follow-up issue</a>
    </div>
    <div class="panel"><pre>${escapeHTML(JSON.stringify(assessment || latest?.metrics || {}, null, 2))}</pre></div>`;
}

async function renderEvals(app: HTMLElement) {
  const pf = await loadLinkJSON<Record<string, unknown>>('promptfoo');
  const latest = await loadLatest();
  app.innerHTML = `<h1>Assist evals (Promptfoo)</h1>
    <p class="muted"><strong>Not a compliance verdict.</strong> Offline CI gates schema/citation regressions; live compare is credential-gated.</p>
    <div class="grid">
      ${tile('Pass rate', fmt(latest?.metrics['kpi.promptfoo.pass_rate'] ?? pf?.pass_rate))}
      ${tile('Mode', fmt(latest?.metrics.promptfoo_mode ?? pf?.mode))}
    </div>
    <div class="actions">
      <a class="btn" href="${workflowDispatchURL('llm-assist.yml')}">Dispatch LLM assist + live eval</a>
    </div>
    <div class="panel"><pre>${escapeHTML(JSON.stringify(pf || {}, null, 2))}</pre></div>`;
}

async function renderCorpus(app: HTMLElement) {
  const latest = await loadLatest();
  const { owner, repo } = repoBase();
  const html = DOMPurify.sanitize(
    await marked.parse(
      'Label schema lives in `testdata/corpus/labels.json`. Use the **corpus-label** PR template (`[corpus-label]` in title/body).',
    ),
  );
  app.innerHTML = `<h1>Corpus</h1>
    <p class="muted">Architects submit ADR Markdown + expected labels via PR.</p>
    <div class="grid">
      ${tile('Corpus size', fmt(latest?.metrics['kpi.corpus.size']))}
      ${tile('Harness precision', fmt(latest?.metrics['kpi.harness.precision']))}
      ${tile('Harness recall', fmt(latest?.metrics['kpi.harness.recall']))}
      ${tile('High / med / low', `${fmt(latest?.metrics.confidence_high)} / ${fmt(latest?.metrics.confidence_medium)} / ${fmt(latest?.metrics.confidence_low)}`)}
    </div>
    <div class="actions">
      <a class="btn" href="${workflowDispatchURL('corpus-eval.yml')}">Run corpus eval</a>
      <a class="btn secondary" href="https://github.com/${owner}/${repo}/compare">Open labeling PR</a>
    </div>
    <div class="panel adr-body">${html}</div>`;
}

async function renderDocs(app: HTMLElement) {
  const { owner, repo } = repoBase();
  app.innerHTML = `<h1>Docs</h1>
    <ul>
      <li><a href="https://github.com/${owner}/${repo}/blob/main/docs/workshop.md">Workshop architecture</a></li>
      <li><a href="https://github.com/${owner}/${repo}/blob/main/docs/kpi-dictionary.md">KPI dictionary</a></li>
      <li><a href="https://github.com/${owner}/${repo}/blob/main/docs/tool-adoption-decisions.md">Tool adoption (Promptfoo)</a></li>
      <li><a href="https://github.com/${owner}/${repo}/blob/main/eval/promptfoo/README.md">Promptfoo eval</a></li>
    </ul>
    <p class="muted">Joern remains a follow-on when ≥N accepted ADRs require data-flow evidence OpenGrep cannot supply.</p>`;
}

async function render() {
  nav();
  const app = document.getElementById('app')!;
  switch (route()) {
    case '/':
      return renderHome(app);
    case '/runs':
      return renderRuns(app);
    case '/kpis':
      return renderKPIs(app);
    case '/corpus':
      return renderCorpus(app);
    case '/candidates':
      return renderCandidates(app);
    case '/proposals':
      return renderProposals(app);
    case '/assess':
      return renderAssess(app);
    case '/evals':
      return renderEvals(app);
    case '/docs':
      return renderDocs(app);
    default:
      app.innerHTML = `<h1>Not found</h1>`;
  }
}

window.addEventListener('hashchange', () => {
  void render();
});
void render();

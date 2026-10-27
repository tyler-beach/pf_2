import uPlot from 'uplot';
import { marked } from 'marked';
import DOMPurify from 'dompurify';
import { createTwoFilesPatch } from 'diff';
import { html as diff2html } from 'diff2html';
import {
  issueURL,
  loadADR,
  loadADRIndex,
  loadCosts,
  loadLatest,
  loadLinkJSON,
  loadManifest,
  loadPipeline,
  loadRuns,
  loadSampleJourney,
  repoBase,
  workflowDispatchURL,
  type MetricsRow,
  type PipelineStep,
} from './data';
import { emptyState, escapeHTML, fmt, fmtUSD, pill, setCrumb, setTopActions, tile } from './ui';

function chart(el: HTMLElement, series: Array<{ label: string; stroke: string; data: number[] }>, xs: number[]) {
  el.innerHTML = '';
  if (!xs.length) {
    el.innerHTML = `<p class="muted">No trend data yet.</p>`;
    return;
  }
  new uPlot(
    {
      width: el.clientWidth || 720,
      height: el.classList.contains('tall') ? 280 : 220,
      series: [{}, ...series.map((s) => ({ label: s.label, stroke: s.stroke }))],
      axes: [{ stroke: '#8fa0b3', grid: { stroke: '#273445' } }, { stroke: '#8fa0b3', grid: { stroke: '#273445' } }],
      legend: { show: true },
    },
    [xs, ...series.map((s) => s.data)],
    el,
  );
}

export async function renderHome(app: HTMLElement) {
  setCrumb('Overview');
  setTopActions(`
    <a class="btn" href="${workflowDispatchURL('llm-assist.yml')}">Run LLM assist</a>
    <a class="btn secondary" href="#/sample">Sample journey</a>
  `);
  const [latest, manifest, runs, pipeline] = await Promise.all([
    loadLatest(),
    loadManifest(),
    loadRuns(),
    loadPipeline(),
  ]);
  if (!latest) {
    app.innerHTML = emptyState('No published data', 'Run CI / workshop-publish to populate the command center.');
    return;
  }
  const m = latest.metrics;
  const sloLLM = latest.slo.assess_runtime_llm_calls_zero === true;
  const sloPF = latest.slo.promptfoo_offline_pass !== false;
  const steps = pipeline?.steps || [];
  app.innerHTML = `
    <div class="fade-in">
      <div class="hero">
        <section class="hero-card">
          <div class="pill accent">Internal command center</div>
          <h1>Pathfinder visibility, end to end</h1>
          <p class="lede">${escapeHTML(latest.summary)}</p>
          <p class="muted">Run <code>${escapeHTML(latest.run.run_id)}</code> · ${escapeHTML(latest.run.workflow)} · ${escapeHTML(latest.run.timestamp || '')}${manifest?.updated_at ? ` · data ${escapeHTML(manifest.updated_at)}` : ''}</p>
          <div class="actions">
            <a class="btn" href="#/pipeline">Inspect pipeline</a>
            <a class="btn secondary" href="#/metrics">KPI drilldowns</a>
            <a class="btn secondary" href="#/adrs">Browse ADRs</a>
            <a class="btn secondary" href="#/costs">LLM cost</a>
          </div>
        </section>
        <section class="stat-stack">
          ${tile('Assess LLM invariant', sloLLM ? 'OK' : 'FAIL', { cls: sloLLM ? 'ok' : 'bad', sub: 'runtime_llm_calls must be 0' })}
          ${tile('Promptfoo offline', sloPF ? 'OK' : 'CHECK', { cls: sloPF ? 'ok' : 'warn', sub: 'assist quality only' })}
          ${tile('Assist cost (est.)', fmtUSD(m['kpi.assist.cost_usd']), { sub: String(m['kpi.assist.provider'] || latest.run.provider_mode || 'offline') })}
        </section>
      </div>
      <div class="grid">
        ${tile('Candidates', fmt(m.candidate_total), { href: '#/candidates' })}
        ${tile('High / med / low', `${fmt(m.confidence_high)} / ${fmt(m.confidence_medium)} / ${fmt(m.confidence_low)}`, { href: '#/metrics?focus=confidence' })}
        ${tile('Escalation rate', fmt(m['kpi.escalation.rate']), { href: '#/metrics?focus=escalation' })}
        ${tile('Assess verdict', fmt(m['kpi.assess.verdict']), { href: '#/assess', cls: m['kpi.assess.verdict'] === 'PASS' ? 'ok' : 'warn' })}
        ${tile('Proposals', fmt(m['kpi.assist.proposal_count']), { href: '#/proposals' })}
        ${tile('Accept rate', fmt(m['kpi.review.accept_rate']), { href: '#/proposals' })}
        ${tile('Corpus size', fmt(m['kpi.corpus.size']), { href: '#/corpus' })}
        ${tile('Promptfoo pass', fmt(m['kpi.promptfoo.pass_rate']), { href: '#/evals' })}
      </div>
      <h2>Pipeline at a glance</h2>
      <div class="pipeline">
        ${steps
          .map(
            (s) => `<a class="step" href="#/pipeline/${escapeHTML(s.id)}">
            <div class="n">0${s.order}</div>
            <div class="t">${escapeHTML(s.title)}</div>
            <div class="d">${escapeHTML(s.subtitle)}</div>
          </a>`,
          )
          .join('')}
      </div>
      <h2>Confidence trend</h2>
      <div id="spark" class="chart panel sparkline-wrap"></div>
    </div>`;
  const el = document.getElementById('spark');
  if (el) {
    const xs = runs.map((_, i) => i);
    chart(el, [
      { label: 'high', stroke: '#3dd68c', data: runs.map((r) => Number(r.metrics.confidence_high || 0)) },
      { label: 'low', stroke: '#e8a838', data: runs.map((r) => Number(r.metrics.confidence_low || 0)) },
    ], xs);
  }
}

export async function renderMetrics(app: HTMLElement) {
  setCrumb('Metrics · drilldown');
  setTopActions(`<a class="btn secondary" href="#/runs">Compare runs</a>`);
  const params = new URLSearchParams(location.hash.split('?')[1] || '');
  const focus = params.get('focus') || 'all';
  const [runs, latest] = await Promise.all([loadRuns(), loadLatest()]);
  const m = latest?.metrics || {};
  app.innerHTML = `
    <div class="fade-in">
      <h1>Metrics command deck</h1>
      <p class="lede">Full KPI surface with trends and drilldowns. Promptfoo / assist numbers are quality signals — not compliance.</p>
      <div class="tabs" id="metric-tabs">
        ${['all', 'confidence', 'escalation', 'harness', 'review', 'assist', 'assess']
          .map((t) => `<button type="button" class="tab ${focus === t ? 'active' : ''}" data-t="${t}">${t}</button>`)
          .join('')}
      </div>
      <div class="grid">
        ${tile('High share', fmt(m['kpi.confidence.share.high'] ?? (Number(m.candidate_total) ? Number(m.confidence_high) / Number(m.candidate_total) : null)))}
        ${tile('Medium share', fmt(m['kpi.confidence.share.medium']))}
        ${tile('Low share', fmt(m['kpi.confidence.share.low'] ?? m['kpi.escalation.rate']))}
        ${tile('Harness precision', fmt(m['kpi.harness.precision']))}
        ${tile('Harness recall', fmt(m['kpi.harness.recall']))}
        ${tile('Recall high/med/low', `${fmt(m['kpi.harness.recall.high'])} / ${fmt(m['kpi.harness.recall.medium'])} / ${fmt(m['kpi.harness.recall.low'])}`)}
        ${tile('Review backlog', fmt(m['kpi.review.backlog']))}
        ${tile('Accept rate', fmt(m['kpi.review.accept_rate']))}
        ${tile('Assist proposals', fmt(m['kpi.assist.proposal_count']))}
        ${tile('Assist cost', fmtUSD(m['kpi.assist.cost_usd']))}
        ${tile('Prompt tokens', fmt(m['kpi.assist.prompt_tokens']))}
        ${tile('Completion tokens', fmt(m['kpi.assist.completion_tokens']))}
      </div>
      <div class="split" style="margin-top:1rem">
        <div id="m-chart" class="chart tall panel flat"></div>
        <div class="panel flat">
          <h3>Latest metrics JSON</h3>
          <pre>${escapeHTML(JSON.stringify(m, null, 2))}</pre>
        </div>
      </div>
    </div>`;
  document.querySelectorAll('#metric-tabs .tab').forEach((btn) => {
    btn.addEventListener('click', () => {
      const t = (btn as HTMLElement).dataset.t;
      location.hash = `#/metrics?focus=${t}`;
    });
  });
  const el = document.getElementById('m-chart');
  if (el && runs.length) {
    const xs = runs.map((_, i) => i);
    const series =
      focus === 'assist'
        ? [
            { label: 'proposals', stroke: '#3d9cf0', data: runs.map((r) => Number(r.metrics['kpi.assist.proposal_count'] || 0)) },
            { label: 'cost_usd', stroke: '#2eb8a0', data: runs.map((r) => Number(r.metrics['kpi.assist.cost_usd'] || 0)) },
          ]
        : focus === 'harness'
          ? [
              { label: 'precision', stroke: '#3dd68c', data: runs.map((r) => Number(r.metrics['kpi.harness.precision'] || 0)) },
              { label: 'recall', stroke: '#e8a838', data: runs.map((r) => Number(r.metrics['kpi.harness.recall'] || 0)) },
            ]
          : [
              { label: 'escalation', stroke: '#e8a838', data: runs.map((r) => Number(r.metrics['kpi.escalation.rate'] || 0)) },
              { label: 'promptfoo', stroke: '#3d9cf0', data: runs.map((r) => Number(r.metrics['kpi.promptfoo.pass_rate'] || 0)) },
              { label: 'low', stroke: '#f0456d', data: runs.map((r) => Number(r.metrics.confidence_low || 0)) },
            ];
    chart(el, series, xs);
  }
}

export async function renderPipeline(app: HTMLElement, stepId?: string) {
  setCrumb(stepId ? `Pipeline · ${stepId}` : 'Pipeline');
  setTopActions(`<a class="btn secondary" href="#/sample">Walk Sample journey</a>`);
  const data = await loadPipeline();
  const steps = data?.steps || [];
  const active = stepId ? steps.find((s) => s.id === stepId) : steps[0];
  app.innerHTML = `
    <div class="fade-in">
      <h1>Pathfinder pipeline</h1>
      <p class="lede">Deep analysis for every stage — inputs, outputs, invariants, failure modes, and sample artifacts.</p>
      <div class="pipeline">
        ${steps
          .map(
            (s) => `<a class="step ${active?.id === s.id ? 'active' : ''}" href="#/pipeline/${escapeHTML(s.id)}">
            <div class="n">0${s.order}</div><div class="t">${escapeHTML(s.title)}</div>
            <div class="d">${escapeHTML(s.subtitle)}</div></a>`,
          )
          .join('')}
      </div>
      ${active ? renderStepDetail(active) : emptyState('No pipeline metadata', 'pipeline.json missing from snapshot.')}
    </div>`;
}

function renderStepDetail(s: PipelineStep): string {
  return `
    <div class="split">
      <section class="panel flat">
        <div class="pill accent">Step 0${s.order}</div>
        <h2 style="margin-top:0.5rem">${escapeHTML(s.title)}</h2>
        <p class="muted">${escapeHTML(s.deep.what)}</p>
        ${s.command ? `<p><code>${escapeHTML(s.command)}</code></p>` : ''}
        ${s.invariant ? `<p>${pill(s.invariant, 'ok')}</p>` : ''}
        <h3>How it works</h3>
        <ul>${s.deep.how.map((x) => `<li>${escapeHTML(x)}</li>`).join('')}</ul>
        <h3>Failure modes</h3>
        <ul>${s.deep.failure_modes.map((x) => `<li>${escapeHTML(x)}</li>`).join('')}</ul>
        <h3>Related metrics</h3>
        <div class="actions">${s.deep.metrics.map((x) => pill(x, '')).join('')}</div>
      </section>
      <section class="panel flat">
        <h3>I/O contract</h3>
        <dl class="kv">
          <dt>Inputs</dt><dd>${s.inputs.map((x) => `<code>${escapeHTML(x)}</code>`).join('<br/>') || '—'}</dd>
          <dt>Outputs</dt><dd>${s.outputs.map((x) => `<code>${escapeHTML(x)}</code>`).join('<br/>') || '—'}</dd>
        </dl>
        <h3>Sample artifact</h3>
        <pre>${escapeHTML(JSON.stringify(s.deep.sample_artifact ?? { note: 'See linked snapshot files' }, null, 2))}</pre>
      </section>
    </div>`;
}

export async function renderADRs(app: HTMLElement, id?: string) {
  if (id) return renderADRDetail(app, id);
  setCrumb('ADRs');
  setTopActions('');
  const index = await loadADRIndex();
  const adrs = index?.adrs || [];
  app.innerHTML = `
    <div class="fade-in">
      <h1>Published ADRs</h1>
      <p class="lede">Dig into ADR source, linked candidates, and confidence mix from the latest snapshot.</p>
      <div class="filter-bar"><input class="search" id="adr-q" placeholder="Filter by id or title…" /></div>
      <table>
        <thead><tr><th>ID</th><th>Title</th><th>Status</th><th>Confidence mix</th><th>Candidates</th></tr></thead>
        <tbody id="adr-rows">
          ${adrs
            .map(
              (a) => `<tr class="row-link" data-id="${escapeHTML(a.id)}">
              <td><code>${escapeHTML(a.id)}</code></td>
              <td>${escapeHTML(a.title)}</td>
              <td>${pill(a.status, a.status === 'accepted' ? 'ok' : '')}</td>
              <td>${a.confidence_mix ? `${a.confidence_mix.high}H / ${a.confidence_mix.medium}M / ${a.confidence_mix.low}L` : '—'}</td>
              <td>${(a.candidate_ids || []).length}</td>
            </tr>`,
            )
            .join('')}
        </tbody>
      </table>
    </div>`;
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
  setCrumb(`ADRs · ${id}`);
  setTopActions(`<a class="btn secondary" href="#/adrs">← All ADRs</a>`);
  const adr = await loadADR(id);
  if (!adr) {
    app.innerHTML = emptyState('ADR not found', id);
    return;
  }
  const html = DOMPurify.sanitize(await marked.parse(adr.body || '_No body_'));
  const cands = await loadLinkJSON<{ candidates?: Array<Record<string, unknown>> }>('candidates');
  const linked = (cands?.candidates || []).filter((c) => String(c.adr_id) === id || (adr.candidate_ids || []).includes(String(c.id)));
  app.innerHTML = `
    <div class="fade-in">
      <h1>${escapeHTML(adr.id)} · ${escapeHTML(adr.title)}</h1>
      <p class="muted">${pill(adr.status, 'accent')} · ${escapeHTML(adr.path)}</p>
      <div class="grid">
        ${tile('Linked candidates', fmt(linked.length))}
        ${tile('High', fmt(adr.confidence_mix?.high ?? linked.filter((c) => c.confidence === 'high').length))}
        ${tile('Medium', fmt(adr.confidence_mix?.medium ?? linked.filter((c) => c.confidence === 'medium').length))}
        ${tile('Low', fmt(adr.confidence_mix?.low ?? linked.filter((c) => c.confidence === 'low').length))}
      </div>
      <div class="split" style="margin-top:1rem">
        <div class="panel flat adr-body">${html}</div>
        <div class="panel flat">
          <h3>Extracted candidates</h3>
          ${linked.length ? `<table><thead><tr><th>ID</th><th>Band</th><th>Statement</th></tr></thead><tbody>
            ${linked.map((c) => `<tr><td><code>${escapeHTML(String(c.id))}</code></td><td>${escapeHTML(String(c.confidence))}</td><td>${escapeHTML(String(c.statement))}</td></tr>`).join('')}
          </tbody></table>` : '<p class="muted">No candidates linked in this snapshot.</p>'}
        </div>
      </div>
    </div>`;
}

export async function renderCandidates(app: HTMLElement) {
  setCrumb('Candidates');
  const data = await loadLinkJSON<{
    candidates?: Array<{
      id: string; adr_id: string; confidence: string; escalation_reasons?: string[];
      statement: string; pattern_hint: string; modality?: string; source_path?: string;
      source_line_start?: number; source_line_end?: number;
    }>;
    confidence_summary?: { high: number; medium: number; low: number };
  }>('candidates');
  const rows = data?.candidates || [];
  const low = rows.filter((c) => c.confidence === 'low');
  const body = [
    '## Low-confidence checklist',
    '',
    'Generated from Pathfinder Command Center.',
    '',
    ...low.map((c) => `- [ ] ${c.id} (${c.adr_id}): ${c.statement}`),
  ].join('\n');
  setTopActions(`<a class="btn" href="${issueURL('[ADR Assessment] Low-confidence review batch', body)}">Open checklist issue</a>`);
  app.innerHTML = `
    <div class="fade-in">
      <h1>Obligation candidates</h1>
      <p class="lede">Filter by confidence band and inspect citation + escalation reasons.</p>
      <div class="grid">
        ${tile('Total', fmt(rows.length))}
        ${tile('High', fmt(data?.confidence_summary?.high ?? rows.filter((c) => c.confidence === 'high').length), { cls: 'ok' })}
        ${tile('Medium', fmt(data?.confidence_summary?.medium ?? rows.filter((c) => c.confidence === 'medium').length), { cls: 'warn' })}
        ${tile('Low', fmt(data?.confidence_summary?.low ?? low.length), { cls: 'bad' })}
      </div>
      <div class="filter-bar">
        <button type="button" class="tab active" data-band="all">All</button>
        <button type="button" class="tab" data-band="high">High</button>
        <button type="button" class="tab" data-band="medium">Medium</button>
        <button type="button" class="tab" data-band="low">Low</button>
        <input class="search" id="cand-q" placeholder="Search statement / id…" />
      </div>
      <table>
        <thead><tr><th>ID</th><th>ADR</th><th>Band</th><th>Pattern</th><th>Citation</th><th>Reasons</th><th>Statement</th></tr></thead>
        <tbody id="cand-body">
          ${rows
            .map(
              (c) => `<tr data-band="${escapeHTML(c.confidence)}" data-q="${escapeHTML((c.id + ' ' + c.statement).toLowerCase())}">
              <td><code>${escapeHTML(c.id)}</code></td>
              <td><a href="#/adrs/${escapeHTML(c.adr_id)}">${escapeHTML(c.adr_id)}</a></td>
              <td>${pill(c.confidence, c.confidence === 'low' ? 'warn' : c.confidence === 'high' ? 'ok' : '')}</td>
              <td>${escapeHTML(c.pattern_hint)}</td>
              <td class="muted">${escapeHTML(c.source_path || '')}:${fmt(c.source_line_start)}-${fmt(c.source_line_end)}</td>
              <td>${escapeHTML((c.escalation_reasons || []).join(', '))}</td>
              <td>${escapeHTML(c.statement)}</td>
            </tr>`,
            )
            .join('')}
        </tbody>
      </table>
    </div>`;
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
  document.querySelectorAll('.filter-bar .tab').forEach((btn) => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('.filter-bar .tab').forEach((b) => b.classList.remove('active'));
      btn.classList.add('active');
      band = (btn as HTMLElement).dataset.band || 'all';
      apply();
    });
  });
  document.getElementById('cand-q')?.addEventListener('input', apply);
}

export async function renderProposals(app: HTMLElement) {
  setCrumb('Proposals');
  setTopActions(`<a class="btn secondary" href="#/costs">Cost detail</a>`);
  const data = await loadLinkJSON<{
    provider?: string; model?: string; prompt_tokens?: number; completion_tokens?: number;
    estimated_cost_usd?: number; proposal_count?: number;
    proposals?: Array<{
      proposal_id: string; parent_candidate_id: string; statement: string; normalized_statement: string;
      suggested_pattern_id: string; rationale: string; parent_confidence?: string; confidence?: string;
      executable: boolean; approval_status: string; provider?: string; model?: string;
    }>;
  }>('proposals');
  const proposals = data?.proposals || [];
  const latest = await loadLatest();
  app.innerHTML = `
    <div class="fade-in">
      <h1>Assist proposals</h1>
      <p class="lede">Review-only artifacts. Never auto-approved, never executable. Accept via <code>accept-assist-proposal</code> / <code>record-obligation-review</code>.</p>
      <div class="grid">
        ${tile('Proposals', fmt(data?.proposal_count ?? proposals.length))}
        ${tile('Provider', fmt(data?.provider || latest?.metrics['kpi.assist.provider']))}
        ${tile('Model', fmt(data?.model || latest?.metrics['kpi.assist.model']))}
        ${tile('Est. cost', fmtUSD(data?.estimated_cost_usd ?? latest?.metrics['kpi.assist.cost_usd']))}
        ${tile('Prompt tokens', fmt(data?.prompt_tokens ?? latest?.metrics['kpi.assist.prompt_tokens']))}
        ${tile('Completion tokens', fmt(data?.completion_tokens ?? latest?.metrics['kpi.assist.completion_tokens']))}
        ${tile('Accept rate', fmt(latest?.metrics['kpi.review.accept_rate']))}
      </div>
      <div id="prop-list"></div>
    </div>`;
  const list = document.getElementById('prop-list')!;
  if (!proposals.length) {
    list.innerHTML = `<p class="muted">No proposals in latest snapshot.</p>`;
    return;
  }
  for (const p of proposals) {
    const panel = document.createElement('div');
    panel.className = 'panel';
    const patch = createTwoFilesPatch('candidate', 'proposal', `${p.statement}\n`, `${p.normalized_statement}\n`, '', '', { context: 4 });
    panel.innerHTML = `
      <div class="actions" style="margin:0 0 0.5rem">
        ${pill(p.proposal_id, 'accent')} ${pill(p.approval_status)} ${pill(`executable=${p.executable}`, p.executable ? 'bad' : 'ok')}
      </div>
      <p class="muted">${escapeHTML(p.parent_candidate_id)} · ${escapeHTML(p.parent_confidence || p.confidence || '')} · ${escapeHTML(p.suggested_pattern_id)}</p>
      <p>${escapeHTML(p.rationale)}</p>
      <div class="diff">${diff2html(patch, { drawFileList: false, matching: 'lines', outputFormat: 'side-by-side' })}</div>`;
    list.appendChild(panel);
  }
}

export async function renderAssess(app: HTMLElement) {
  setCrumb('Assessment');
  const assessment = await loadLinkJSON<{
    verdict?: string; runtime_llm_calls?: number; coverage_status?: string;
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
  setTopActions(`<a class="btn" href="${issueURL('[ADR Assessment] Follow-up from workshop', body)}">Open follow-up issue</a>`);
  const llm = assessment?.runtime_llm_calls ?? Number(latest?.metrics['kpi.invariant.llm_calls'] ?? 0);
  app.innerHTML = `
    <div class="fade-in">
      <h1>Deterministic assessment</h1>
      <p class="lede">Assess / PR gates never call an LLM. This page proves the invariant and surfaces coverage / requirement status.</p>
      <div class="grid">
        ${tile('Verdict', fmt(assessment?.verdict || latest?.metrics['kpi.assess.verdict']))}
        ${tile('LLM calls', fmt(llm), { cls: llm === 0 ? 'ok' : 'bad', sub: 'must be 0' })}
        ${tile('Coverage', fmt(assessment?.coverage_status))}
        ${tile('Covered', fmt(latest?.metrics['kpi.assess.covered']))}
        ${tile('Unbound', fmt(latest?.metrics['kpi.assess.unbound']))}
        ${tile('Stale', fmt(latest?.metrics['kpi.assess.stale']))}
      </div>
      <div class="panel"><pre>${escapeHTML(JSON.stringify(assessment || latest?.metrics || {}, null, 2))}</pre></div>
    </div>`;
}

export async function renderCosts(app: HTMLElement) {
  setCrumb('LLM cost');
  setTopActions(`<a class="btn" href="${workflowDispatchURL('llm-assist.yml')}">Dispatch assist</a>`);
  const [latest, costs, proposals, runs] = await Promise.all([
    loadLatest(),
    loadCosts(),
    loadLinkJSON<Record<string, unknown>>('proposals'),
    loadRuns(),
  ]);
  const m = latest?.metrics || {};
  app.innerHTML = `
    <div class="fade-in">
      <h1>LLM cost & usage</h1>
      <p class="lede">Batch assist only. Offline fixtures cost $0. Live runs estimate USD from reported token usage (gpt-4o-mini list rates).</p>
      <div class="grid">
        ${tile('Provider mode', fmt(latest?.run.provider_mode || m['kpi.assist.provider'] || 'offline'))}
        ${tile('Model', fmt(m['kpi.assist.model'] || proposals?.model))}
        ${tile('Est. cost', fmtUSD(m['kpi.assist.cost_usd'] ?? proposals?.estimated_cost_usd ?? costs?.estimated_cost_usd))}
        ${tile('Prompt tokens', fmt(m['kpi.assist.prompt_tokens'] ?? proposals?.prompt_tokens))}
        ${tile('Completion tokens', fmt(m['kpi.assist.completion_tokens'] ?? proposals?.completion_tokens))}
        ${tile('LLM calls (assist)', fmt(proposals?.runtime_llm_calls ?? m['kpi.assist.proposal_count']))}
        ${tile('Assess LLM calls', fmt(m['kpi.invariant.llm_calls']), { cls: 'ok', sub: 'invariant' })}
      </div>
      <div id="cost-chart" class="chart panel"></div>
      <div class="panel"><h3>Cost notes</h3>
        <pre>${escapeHTML(JSON.stringify(costs || {
          pricing_note: proposals?.cost_pricing_note || 'offline=0; live uses gpt-4o-mini list rates ($0.15/1M input, $0.60/1M output)',
          proposals: proposals || null,
        }, null, 2))}</pre>
      </div>
    </div>`;
  const el = document.getElementById('cost-chart');
  if (el) {
    const xs = runs.map((_, i) => i);
    chart(el, [
      { label: 'cost_usd', stroke: '#2eb8a0', data: runs.map((r) => Number(r.metrics['kpi.assist.cost_usd'] || 0)) },
      { label: 'proposals', stroke: '#3d9cf0', data: runs.map((r) => Number(r.metrics['kpi.assist.proposal_count'] || 0)) },
    ], xs);
  }
}

export async function renderSample(app: HTMLElement, stepId?: string) {
  setCrumb('Sample journey');
  const journey = await loadSampleJourney();
  const steps = journey?.steps || [];
  const idx = Math.max(0, steps.findIndex((s) => s.id === stepId));
  const step = steps[idx] || steps[0];
  const artifact = step?.artifact_key ? journey?.artifacts?.[step.artifact_key] : null;
  setTopActions(`
    ${idx > 0 ? `<a class="btn secondary" href="#/sample/${steps[idx - 1].id}">← Prev</a>` : ''}
    ${idx < steps.length - 1 ? `<a class="btn" href="#/sample/${steps[idx + 1].id}">Next →</a>` : `<a class="btn" href="#/pipeline">Open live pipeline</a>`}
  `);
  app.innerHTML = `
    <div class="fade-in">
      <h1>Sample technical journey</h1>
      <p class="lede">Step through a complete Pathfinder pass with real artifact shapes — how extract, confidence, assist, review, assess, and workshop emit fit together.</p>
      <div class="journey-track">
        ${steps
          .map(
            (s, i) => `<a class="journey-dot ${s.id === step?.id ? 'active' : i < idx ? 'done' : ''}" href="#/sample/${escapeHTML(s.id)}">${i + 1}. ${escapeHTML(s.title)}</a>`,
          )
          .join('')}
      </div>
      ${step ? `
        <div class="split">
          <section class="panel flat">
            <div class="pill accent">Step ${idx + 1} / ${steps.length}</div>
            <h2 style="margin-top:0.5rem">${escapeHTML(step.title)}</h2>
            <p>${escapeHTML(step.narrative)}</p>
            <h3>Technical detail</h3>
            <p class="muted">${escapeHTML(step.technical)}</p>
            ${(step.callouts || []).map((c) => `<p>${pill(c, 'warn')}</p>`).join('')}
          </section>
          <section class="panel flat">
            <h3>Example artifact</h3>
            <pre>${escapeHTML(JSON.stringify(artifact ?? { tip: 'Continue to see artifacts for later steps' }, null, 2))}</pre>
          </section>
        </div>` : emptyState('Journey missing', 'sample-journey.json not published.')}
    </div>`;
}

export async function renderRuns(app: HTMLElement) {
  setCrumb('Runs');
  setTopActions('');
  const runs = await loadRuns();
  const ordered = runs.slice().reverse();
  const options = ordered.map((r) => `<option value="${escapeHTML(r.run.run_id)}">${escapeHTML(r.run.run_id)} · ${escapeHTML(r.run.workflow)}</option>`).join('');
  app.innerHTML = `<div class="fade-in"><h1>Run history</h1>
    <div class="panel flat">
      <h2>Compare runs</h2>
      <div class="actions">
        <label>A <select id="run-a">${options}</select></label>
        <label>B <select id="run-b">${options}</select></label>
        <button type="button" class="btn" id="run-compare-btn">Compare</button>
      </div>
      <div id="run-compare-out" class="muted">Select two runs and compare.</div>
    </div>
    <table><thead><tr><th>Run</th><th>Workflow</th><th>When</th><th>Candidates</th><th>Low</th><th>Cost</th><th>Verdict</th></tr></thead>
    <tbody>${ordered.map((r) => `<tr>
      <td><code>${escapeHTML(r.run.run_id)}</code></td>
      <td>${escapeHTML(r.run.workflow)}</td>
      <td>${escapeHTML(r.run.timestamp || '')}</td>
      <td>${fmt(r.metrics.candidate_total)}</td>
      <td>${fmt(r.metrics.confidence_low)}</td>
      <td>${fmtUSD(r.metrics['kpi.assist.cost_usd'])}</td>
      <td>${fmt(r.metrics['kpi.assess.verdict'])}</td>
    </tr>`).join('')}</tbody></table></div>`;
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
    const keys = new Set([...Object.keys(left.metrics), ...Object.keys(right.metrics)]);
    out.innerHTML = `<table><thead><tr><th>Metric</th><th>A</th><th>B</th></tr></thead><tbody>${[...keys]
      .sort()
      .map((k) => {
        const av = left.metrics[k];
        const bv = right.metrics[k];
        const changed = JSON.stringify(av) !== JSON.stringify(bv);
        return `<tr class="${changed ? 'warn' : ''}"><td><code>${escapeHTML(k)}</code></td><td>${escapeHTML(fmt(av))}</td><td>${escapeHTML(fmt(bv))}</td></tr>`;
      })
      .join('')}</tbody></table>`;
  });
}

export async function renderEvals(app: HTMLElement) {
  setCrumb('Evals');
  const pf = await loadLinkJSON<Record<string, unknown>>('promptfoo');
  const latest = await loadLatest();
  setTopActions(`<a class="btn" href="${workflowDispatchURL('llm-assist.yml')}">Dispatch LLM assist</a>`);
  app.innerHTML = `<div class="fade-in"><h1>Assist evals (Promptfoo)</h1>
    <p class="lede"><strong>Not a compliance verdict.</strong> Offline CI gates schema/citation regressions; live compare is credential-gated.</p>
    <div class="grid">
      ${tile('Pass rate', fmt(latest?.metrics['kpi.promptfoo.pass_rate'] ?? pf?.pass_rate))}
      ${tile('Mode', fmt(latest?.metrics.promptfoo_mode ?? pf?.mode))}
    </div>
    <div class="panel"><pre>${escapeHTML(JSON.stringify(pf || {}, null, 2))}</pre></div></div>`;
}

export async function renderCorpus(app: HTMLElement) {
  setCrumb('Corpus');
  const latest = await loadLatest();
  const { owner, repo } = repoBase();
  setTopActions(`<a class="btn" href="${workflowDispatchURL('corpus-eval.yml')}">Run corpus eval</a>`);
  app.innerHTML = `<div class="fade-in"><h1>Labeled corpus</h1>
    <p class="lede">Architect-labeled ADRs drive harness precision/recall (including recall-by-confidence).</p>
    <div class="grid">
      ${tile('Corpus size', fmt(latest?.metrics['kpi.corpus.size']))}
      ${tile('Precision', fmt(latest?.metrics['kpi.harness.precision']))}
      ${tile('Recall', fmt(latest?.metrics['kpi.harness.recall']))}
      ${tile('Recall H/M/L', `${fmt(latest?.metrics['kpi.harness.recall.high'])} / ${fmt(latest?.metrics['kpi.harness.recall.medium'])} / ${fmt(latest?.metrics['kpi.harness.recall.low'])}`)}
    </div>
    <div class="actions"><a class="btn secondary" href="https://github.com/${owner}/${repo}/compare">Open labeling PR</a>
    <a class="btn secondary" href="#/adrs">Browse published ADRs</a></div></div>`;
}

export async function renderDocs(app: HTMLElement) {
  setCrumb('Docs');
  setTopActions('');
  const { owner, repo } = repoBase();
  app.innerHTML = `<div class="fade-in"><h1>Documentation</h1>
    <ul>
      <li><a href="https://github.com/${owner}/${repo}/blob/main/docs/workshop.md">Workshop architecture</a></li>
      <li><a href="https://github.com/${owner}/${repo}/blob/main/docs/kpi-dictionary.md">KPI dictionary</a></li>
      <li><a href="https://github.com/${owner}/${repo}/blob/main/docs/architecture.md">Architecture (Joern N=10)</a></li>
      <li><a href="https://github.com/${owner}/${repo}/blob/main/eval/promptfoo/README.md">Promptfoo eval</a></li>
    </ul>
    <p class="muted">Joern remains a follow-on when ≥10 accepted ADRs require data-flow evidence OpenGrep cannot supply.</p></div>`;
}

export type { MetricsRow };

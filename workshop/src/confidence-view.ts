import {
  dataAge,
  loadActiveContext,
  loadLinkJSON,
  loadRuns,
  type MetricsRow,
} from './data';
import {
  bindChartHelp,
  bindCopyButtons,
  chartHeading,
  escapeHTML,
  fmt,
  metricsRow,
  setCrumb,
  setPurpose,
  setTopActions,
  staleBanner,
  tag,
  toTSV,
  type ChartHelpCopy,
} from './ui';
import uPlot from 'uplot';

const HELP = {
  bandMix: {
    measures: 'confidence_high / medium / low counts per run (conformal or heuristic band).',
    use: 'X-axis is run index oldest → newest from metrics/runs.jsonl.',
    good: 'High stable or rising; low stable or falling when extract quality improves.',
  },
  escalationCI: {
    measures: 'kpi.escalation.rate with Wilson 95% CI (kpi.escalation.rate.ci_low / ci_high).',
    use: 'Point estimate is low_band / candidate_total; whiskers show sampling uncertainty.',
    good: 'Downward or stable-low escalation with narrowing CIs as n grows.',
  },
  meanP: {
    measures: 'kpi.confidence.mean_p — mean calibrated P(extract_correct) across candidates.',
    use: 'Only statistically meaningful when kpi.confidence.method=platt_conformal.',
    good: 'Rising mean_p with stable or improving harness precision — not a compliance score.',
  },
  methodMix: {
    measures: 'kpi.confidence.calibrated_count vs kpi.confidence.heuristic_count.',
    use: 'Shows whether the run used fitted Platt+conformal params or honest heuristic fallback.',
    good: 'Calibrated dominating once gold is sufficient; heuristic must be labeled as such.',
  },
  kindEscalation: {
    measures: 'kpi.escalation.rate.tech_spec and .code_snippet (± Wilson CI when present).',
    use: 'Compare English ADR/prose extract path vs code/IaC submission path.',
    good: 'Neither path silently absorbs ambiguity; spikes warrant gold/refit review.',
  },
} as const satisfies Record<string, ChartHelpCopy>;

function num(m: Record<string, unknown> | undefined, key: string): number | null {
  if (!m) return null;
  const v = m[key];
  if (v == null || v === '') return null;
  const n = Number(v);
  return Number.isFinite(n) ? n : null;
}

function fmtCI(est: number | null, lo: number | null, hi: number | null): string {
  if (est == null) return '—';
  if (lo == null || hi == null) return fmt(est);
  return `${fmt(est)} [${fmt(lo)}–${fmt(hi)}]`;
}

function chart(el: HTMLElement, series: Array<{ label: string; stroke: string; data: number[] }>, xs: number[]) {
  el.innerHTML = '';
  if (!xs.length) {
    el.innerHTML = `<p class="muted">No trend data yet.</p>`;
    return;
  }
  const data: uPlot.AlignedData = [xs, ...series.map((s) => s.data)];
  // eslint-disable-next-line no-new
  new uPlot(
    {
      width: Math.max(320, el.clientWidth || 640),
      height: 220,
      series: [{}, ...series.map((s) => ({ label: s.label, stroke: s.stroke, width: 2 }))],
      axes: [{ stroke: '#8b919a' }, { stroke: '#8b919a' }],
      legend: { live: true },
    },
    data,
    el,
  );
}

function runOptions(runs: MetricsRow[], selected: string): string {
  return runs
    .slice()
    .reverse()
    .map((r) => {
      const id = r.run.run_id;
      return `<option value="${escapeHTML(id)}" ${id === selected ? 'selected' : ''}>${escapeHTML(id)} · ${escapeHTML(r.run.workflow || '')}</option>`;
    })
    .join('');
}

type Cand = {
  id: string;
  adr_id: string;
  confidence: string;
  confidence_p?: number;
  confidence_method?: string;
  confidence_coverage?: number;
  prediction_set?: string[];
  escalation_reasons?: string[];
  statement: string;
  pattern_hint: string;
  submission_kind?: string;
};

export async function renderConfidence(app: HTMLElement) {
  setCrumb('Confidence');
  setPurpose(
    'Statistical confidence: calibrated P(correct), conformal bands, Wilson CIs. Heuristic score is an input feature — not a probability.',
  );
  setTopActions(`
    <a class="btn" href="#/docs/confidence">Confidence docs</a>
    <a class="btn" href="#/metrics?focus=confidence">Metrics → confidence</a>
    <a class="btn" href="#/candidates?band=low">Low-band candidates</a>
  `);

  const [ctx, allRuns, candidates] = await Promise.all([
    loadActiveContext(),
    loadRuns(),
    loadLinkJSON<{
      candidates?: Cand[];
      confidence_summary?: { high: number; medium: number; low: number };
      calibration?: { method?: string; coverage?: number; gold_example_count?: number; params_version?: string; exchangeability_note?: string };
    }>('candidates'),
  ]);
  const { latest, runs: ctxRuns } = ctx;
  const runs = (allRuns.length ? allRuns : ctxRuns) as MetricsRow[];
  const m = (latest?.metrics || {}) as Record<string, unknown>;
  const selectedRun = latest?.run?.run_id || runs[runs.length - 1]?.run.run_id || '';
  const rows = candidates?.candidates || [];
  const cal = candidates?.calibration;
  const age = dataAge(ctx.manifest?.updated_at || latest?.run?.timestamp);

  const xs = runs.map((_, i) => i);
  const highSeries = runs.map((r) => Number(r.metrics.confidence_high || 0));
  const medSeries = runs.map((r) => Number(r.metrics.confidence_medium || 0));
  const lowSeries = runs.map((r) => Number(r.metrics.confidence_low || 0));
  const escSeries = runs.map((r) => Number(r.metrics['kpi.escalation.rate'] ?? 0));
  const meanPSeries = runs.map((r) => Number(r.metrics['kpi.confidence.mean_p'] ?? NaN));
  const calCount = runs.map((r) => Number(r.metrics['kpi.confidence.calibrated_count'] ?? 0));
  const heurCount = runs.map((r) => Number(r.metrics['kpi.confidence.heuristic_count'] ?? 0));

  const method = String(m['kpi.confidence.method'] || cal?.method || '—');
  const coverage = num(m, 'kpi.confidence.coverage_target') ?? cal?.coverage ?? null;

  app.innerHTML = `
    ${age.stale ? staleBanner(age.label, 'Confidence KPIs may lag until Workshop Publish refreshes.') : ''}
    <p class="run-meta">Active run <code>${escapeHTML(selectedRun || '—')}</code>
      · method <code>${escapeHTML(method)}</code>
      · <a href="#/docs/confidence">Confidence docs</a>
      · <a href="#/sample/extract">Sample extract</a>
    </p>
    <h2>Overview</h2>
    <p class="muted">Calibrated P(correct) vs heuristic triage for the active run.</p>
    ${metricsRow([
      { label: 'Method', value: method, href: '#/docs/confidence' },
      { label: 'Coverage target', value: coverage != null ? `${Math.round(coverage * 100)}%` : '—' },
      { label: 'Mean P(correct)', value: fmt(m['kpi.confidence.mean_p']), cls: method.includes('platt') ? 'ok' : '' },
      {
        label: 'Escalation ±CI',
        value: fmtCI(num(m, 'kpi.escalation.rate'), num(m, 'kpi.escalation.rate.ci_low'), num(m, 'kpi.escalation.rate.ci_high')),
        href: '#/metrics?focus=escalation',
      },
      { label: 'High / med / low', value: `${fmt(m.confidence_high)} / ${fmt(m.confidence_medium)} / ${fmt(m.confidence_low)}` },
      {
        label: 'Calibrated / heuristic',
        value: `${fmt(m['kpi.confidence.calibrated_count'])} / ${fmt(m['kpi.confidence.heuristic_count'])}`,
      },
    ])}
    <div class="spark-grid conf-grid">
      <div>${chartHeading('Band mix across runs', HELP.bandMix)}<div class="chart" id="conf-band-chart"></div></div>
      <div>${chartHeading('Escalation rate', HELP.escalationCI)}<div class="chart" id="conf-esc-chart"></div></div>
      <div>${chartHeading('Mean calibrated P', HELP.meanP)}<div class="chart" id="conf-p-chart"></div></div>
      <div>${chartHeading('Calibrated vs heuristic counts', HELP.methodMix)}<div class="chart" id="conf-method-chart"></div></div>
    </div>

    <h2>At scale</h2>
    <p class="muted">Submission-kind splits with Wilson 95% intervals where emitted.</p>
    ${metricsRow([
      {
        label: 'tech_spec escalation ±',
        value: fmtCI(
          num(m, 'kpi.escalation.rate.tech_spec'),
          num(m, 'kpi.escalation.rate.tech_spec.ci_low'),
          num(m, 'kpi.escalation.rate.tech_spec.ci_high'),
        ),
      },
      {
        label: 'code_snippet escalation ±',
        value: fmtCI(
          num(m, 'kpi.escalation.rate.code_snippet'),
          num(m, 'kpi.escalation.rate.code_snippet.ci_low'),
          num(m, 'kpi.escalation.rate.code_snippet.ci_high'),
        ),
      },
      {
        label: 'High share ±',
        value: fmtCI(
          num(m, 'kpi.confidence.share.high'),
          num(m, 'kpi.confidence.share.high.ci_low'),
          num(m, 'kpi.confidence.share.high.ci_high'),
        ),
      },
      {
        label: 'Harness precision ±',
        value: fmtCI(
          num(m, 'kpi.harness.precision'),
          num(m, 'kpi.harness.precision.ci_low'),
          num(m, 'kpi.harness.precision.ci_high'),
        ),
      },
      { label: 'CI method', value: String(m['kpi.ci.method'] || 'wilson') },
      { label: 'CI level', value: m['kpi.ci.level'] != null ? `${Math.round(Number(m['kpi.ci.level']) * 100)}%` : '95%' },
    ])}
    ${chartHeading('Escalation by submission kind', HELP.kindEscalation)}
    <div class="chart tall" id="conf-kind-chart"></div>

    <h2>Per run</h2>
    <div class="toolbar">
      <label class="muted">Run select
        <select id="conf-run-select">${runOptions(runs, selectedRun)}</select>
      </label>
      <a class="btn" id="conf-open-run" href="#/runs">Open in Runs</a>
      <button type="button" class="btn" id="conf-copy-meta">Copy calibration JSON</button>
    </div>
    <div class="spark-grid conf-grid">
      <div class="artifact">
        <h3>Calibration metadata</h3>
        <pre id="conf-meta-pre">${escapeHTML(
          JSON.stringify(
            {
              method,
              coverage_target: coverage,
              gold_example_count: cal?.gold_example_count ?? null,
              params_version: cal?.params_version ?? null,
              exchangeability_note: cal?.exchangeability_note ?? null,
              mean_p: m['kpi.confidence.mean_p'] ?? null,
              ci: { method: m['kpi.ci.method'] || 'wilson', level: m['kpi.ci.level'] ?? 0.95 },
            },
            null,
            2,
          ),
        )}</pre>
      </div>
      <div class="artifact">
        <h3>Honesty limits</h3>
        <ul>
          <li><code>confidence_p</code> is calibrated P(extract_correct) when <code>confidence_method=platt_conformal</code>.</li>
          <li>Heuristic triage is internal-only (feature for calibration) — not emitted on candidates.</li>
          <li>Conformal bands assume exchangeability with gold; refit after domain shift (50-subs pack).</li>
          <li>Wilson CIs are sampling intervals on rates — not model calibration error bars.</li>
        </ul>
        <p class="muted"><a href="#/docs/confidence">Confidence scoring</a> · <a href="#/docs/kpis">KPI glossary</a></p>
      </div>
    </div>

    <h2>Raw data</h2>
    <div class="toolbar">
      <button type="button" class="btn" id="conf-copy-tsv">Copy candidate TSV</button>
      <button type="button" class="btn" id="conf-copy-keys">Copy metric keys</button>
      <a class="btn" href="#/candidates">Candidates list</a>
    </div>
    <details open>
      <summary>Candidates with confidence fields (${rows.length})</summary>
      <div class="table-wrap sticky-head" style="max-height:420px;overflow:auto">
        <table class="dense" id="conf-cand-table">
          <thead>
            <tr>
              <th>ID</th><th>Band</th><th>P</th><th>Method</th><th>Coverage</th><th>Kind</th><th>Pattern</th><th>Statement</th>
            </tr>
          </thead>
          <tbody>
            ${
              rows.length
                ? rows
                    .slice(0, 200)
                    .map(
                      (c) => `<tr>
                <td><a class="id" href="#/candidates?q=${encodeURIComponent(c.id)}">${escapeHTML(c.id)}</a></td>
                <td>${tag(c.confidence, c.confidence === 'low' ? 'warn' : c.confidence === 'high' ? 'ok' : '')}</td>
                <td class="num">${fmt(c.confidence_p)}</td>
                <td><code>${escapeHTML(c.confidence_method || '—')}</code></td>
                <td class="num">${c.confidence_coverage != null ? fmt(c.confidence_coverage) : '—'}</td>
                <td>${escapeHTML(c.submission_kind || '—')}</td>
                <td class="truncate"><code>${escapeHTML(c.pattern_hint || '')}</code></td>
                <td class="truncate" title="${escapeHTML(c.statement)}">${escapeHTML(c.statement)}</td>
              </tr>`,
                    )
                    .join('')
                : `<tr><td colspan="8" class="muted">No candidates in this snapshot.</td></tr>`
            }
          </tbody>
        </table>
      </div>
      ${rows.length > 200 ? `<p class="muted">Showing first 200 of ${rows.length}. Export TSV for full set.</p>` : ''}
    </details>
    <details style="margin-top:1rem">
      <summary>Metric keys (confidence / CI)</summary>
      <pre class="artifact" id="conf-keys-pre">${escapeHTML(
        [
          'confidence_high|medium|low',
          'kpi.confidence.share.{high,medium,low}',
          'kpi.confidence.share.{high,low}.ci_{low,high}',
          'kpi.escalation.rate[+ .ci_low/.ci_high]',
          'kpi.escalation.rate.{tech_spec,code_snippet}[+ CIs]',
          'kpi.confidence.method',
          'kpi.confidence.coverage_target',
          'kpi.confidence.mean_p',
          'kpi.confidence.calibrated_count',
          'kpi.confidence.heuristic_count',
          'kpi.harness.precision[+ .ci_low/.ci_high]',
          'kpi.ci.level',
          'kpi.ci.method',
        ].join('\n'),
      )}</pre>
    </details>
  `;

  bindChartHelp(app);

  const bandEl = document.getElementById('conf-band-chart');
  if (bandEl) {
    chart(
      bandEl,
      [
        { label: 'high', stroke: '#3fae7a', data: highSeries },
        { label: 'medium', stroke: '#5b9fd4', data: medSeries },
        { label: 'low', stroke: '#c9953a', data: lowSeries },
      ],
      xs,
    );
  }
  const escEl = document.getElementById('conf-esc-chart');
  if (escEl) chart(escEl, [{ label: 'escalation', stroke: '#c9953a', data: escSeries }], xs);
  const pEl = document.getElementById('conf-p-chart');
  if (pEl) {
    const cleaned = meanPSeries.map((v) => (Number.isFinite(v) ? v : 0));
    chart(pEl, [{ label: 'mean_p', stroke: '#6c8cff', data: cleaned }], xs);
  }
  const methodEl = document.getElementById('conf-method-chart');
  if (methodEl) {
    chart(
      methodEl,
      [
        { label: 'calibrated', stroke: '#3fae7a', data: calCount },
        { label: 'heuristic', stroke: '#8b919a', data: heurCount },
      ],
      xs,
    );
  }
  const kindEl = document.getElementById('conf-kind-chart');
  if (kindEl) {
    chart(
      kindEl,
      [
        {
          label: 'tech_spec',
          stroke: '#3fae7a',
          data: runs.map((r) => Number(r.metrics['kpi.escalation.rate.tech_spec'] ?? 0)),
        },
        {
          label: 'code_snippet',
          stroke: '#c9953a',
          data: runs.map((r) => Number(r.metrics['kpi.escalation.rate.code_snippet'] ?? 0)),
        },
      ],
      xs,
    );
  }

  const sel = document.getElementById('conf-run-select') as HTMLSelectElement | null;
  const openRun = document.getElementById('conf-open-run') as HTMLAnchorElement | null;
  const syncOpen = () => {
    if (openRun && sel) openRun.href = `#/runs?a=${encodeURIComponent(sel.value)}`;
  };
  syncOpen();
  sel?.addEventListener('change', syncOpen);

  document.getElementById('conf-copy-meta')?.addEventListener('click', async () => {
    const pre = document.getElementById('conf-meta-pre');
    if (pre) await navigator.clipboard.writeText(pre.textContent || '');
  });
  document.getElementById('conf-copy-keys')?.addEventListener('click', async () => {
    const pre = document.getElementById('conf-keys-pre');
    if (pre) await navigator.clipboard.writeText(pre.textContent || '');
  });
  document.getElementById('conf-copy-tsv')?.addEventListener('click', async () => {
    const tsv = toTSV(
      ['id', 'adr_id', 'confidence', 'confidence_p', 'confidence_method', 'confidence_coverage', 'submission_kind', 'pattern_hint', 'statement'],
      rows.map((c) => [
        c.id,
        c.adr_id,
        c.confidence,
        c.confidence_p == null ? '' : String(c.confidence_p),
        c.confidence_method ?? '',
        c.confidence_coverage == null ? '' : String(c.confidence_coverage),
        c.submission_kind ?? '',
        c.pattern_hint,
        c.statement,
      ]),
    );
    await navigator.clipboard.writeText(tsv);
  });
  bindCopyButtons(app);
}

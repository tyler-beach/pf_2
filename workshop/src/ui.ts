import { bindKpiHelp, kpiHelpControl, kpiLabel } from './kpi-help';

export function escapeHTML(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

/** Allow only simple CSS token classes into class="" attributes. */
export function safeClass(cls?: string): string {
  if (!cls) return '';
  return /^[a-z][a-z0-9_-]*$/i.test(cls) ? cls : '';
}

/** Allow only in-app hash routes or https github.com links. */
export function safeHref(href?: string): string | undefined {
  if (!href) return undefined;
  if (href.startsWith('#/') || href.startsWith('https://github.com/')) return href;
  return undefined;
}

export function fmt(v: unknown): string {
  if (v == null) return '—';
  if (typeof v === 'number') {
    if (Number.isInteger(v)) return String(v);
    if (Math.abs(v) < 0.0001 && v !== 0) return v.toExponential(2);
    return v.toFixed(3);
  }
  if (typeof v === 'boolean') return v ? 'yes' : 'no';
  return String(v);
}

export function fmtUSD(v: unknown): string {
  if (v == null || typeof v !== 'number') return '—';
  if (v === 0) return '$0.00';
  if (v < 0.01) return `$${v.toFixed(5)}`;
  return `$${v.toFixed(4)}`;
}

export type Metric = {
  label: string;
  value: string;
  hint?: string;
  cls?: string;
  href?: string;
  /** When set, renders an accessible KPI help control next to the label. */
  kpi?: string;
};

export function metricsRow(items: Metric[]): string {
  const html = `<div class="metrics">${items
    .map((m) => {
      const cls = safeClass(m.cls);
      const body = `<div class="label">${m.kpi ? kpiLabel(m.label, m.kpi) : escapeHTML(m.label)}</div><div class="value${cls ? ` ${cls}` : ''}" title="${escapeHTML(m.value)}">${escapeHTML(m.value)}</div>${
        m.hint ? `<div class="hint" title="${escapeHTML(m.hint)}">${escapeHTML(m.hint)}</div>` : ''
      }`;
      const href = safeHref(m.href);
      return href
        ? `<a class="metric" href="${escapeHTML(href)}">${body}</a>`
        : `<div class="metric">${body}</div>`;
    })
    .join('')}</div>`;
  return html;
}

/** DOM-safe metrics strip — prefer this over metricsRow + innerHTML. */
export function mountMetricsRow(parent: HTMLElement, items: Metric[]): HTMLElement {
  const wrap = document.createElement('div');
  wrap.innerHTML = metricsRow(items);
  const row = wrap.firstElementChild as HTMLElement;
  parent.appendChild(row);
  bindKpiHelp(row);
  return row;
}

export function statusItem(
  label: string,
  value: string,
  kind: 'ok' | 'warn' | 'bad' | '' = '',
  kpi?: string,
): string {
  const cls = safeClass(kind);
  const lab = kpi ? kpiLabel(label, kpi) : escapeHTML(label);
  return `<span class="item"><span class="dot${cls ? ` ${cls}` : ''}"></span>${lab} <strong${cls ? ` class="${cls}"` : ''}>${escapeHTML(value)}</strong></span>`;
}

export function tag(text: string, kind = ''): string {
  const cls = safeClass(kind);
  return `<span class="tag${cls ? ` ${cls}` : ''}">${escapeHTML(text)}</span>`;
}

export function setCrumb(text: string) {
  const el = document.getElementById('crumb');
  if (el) el.textContent = text;
}

export function setPurpose(text: string) {
  const el = document.getElementById('purpose');
  if (el) el.textContent = text;
}

export function setTopActions(html: string) {
  const el = document.getElementById('top-actions');
  if (el) el.innerHTML = html;
}

export async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}

/** Brief label flash after clipboard write (export buttons, copy controls). */
export function flashButtonLabel(btn: HTMLElement, label: string, ms = 1200) {
  const prev = btn.textContent;
  btn.textContent = label;
  window.setTimeout(() => {
    if (btn.textContent === label) btn.textContent = prev;
  }, ms);
}

export async function copyWithButtonFeedback(btn: HTMLElement, text: string): Promise<boolean> {
  const ok = await copyText(text);
  flashButtonLabel(btn, ok ? 'Copied' : 'Failed');
  return ok;
}

export function copyControl(text: string, label = 'Copy'): string {
  return `<button type="button" class="btn copy-btn" data-copy="${escapeHTML(encodeURIComponent(text))}">${escapeHTML(label)}</button>`;
}

export function bindCopyButtons(root: ParentNode = document) {
  root.querySelectorAll<HTMLButtonElement>('[data-copy]').forEach((btn) => {
    if (btn.dataset.bound === '1') return;
    btn.dataset.bound = '1';
    btn.addEventListener('click', async () => {
      let raw = btn.getAttribute('data-copy') || '';
      try {
        raw = decodeURIComponent(raw);
      } catch {
        /* keep raw */
      }
      await copyWithButtonFeedback(btn, raw);
    });
  });
}

/** Visible row expand affordance — pairs with `[data-expand]` click handlers / `o` · Space. */
export function expandControl(id: string, label = 'Details'): string {
  return `<button type="button" class="btn expand-btn" data-expand="${escapeHTML(id)}" title="Expand details (o)" aria-label="${escapeHTML(label)}">
    <span class="expand-chevron" aria-hidden="true">▸</span>
    <span class="expand-label">${escapeHTML(label)}</span>
  </button>`;
}

/** Copy for chart info affordance — measures / how to use / what “good” looks like. */
export type ChartHelpCopy = {
  measures: string;
  use: string;
  good: string;
};

let chartHelpSeq = 0;

/** Chart title + keyboard-accessible ℹ popover (click / Enter / Space; Esc closes). */
export function chartHeading(title: string, help: ChartHelpCopy, level: 'h2' | 'h3' = 'h3'): string {
  const id = `chart-help-${++chartHelpSeq}`;
  const tag = level === 'h2' ? 'h2' : 'h3';
  return `<div class="chart-head">
    <${tag} class="chart-title">${escapeHTML(title)}</${tag}>
    <button type="button" class="chart-help-btn" aria-expanded="false" aria-controls="${id}" title="About this chart" aria-label="About ${escapeHTML(title)} chart">ℹ</button>
    <div class="chart-help-pop" id="${id}" role="region" aria-label="About ${escapeHTML(title)}" hidden>
      <p><span class="chart-help-k">Measures</span> ${escapeHTML(help.measures)}</p>
      <p><span class="chart-help-k">How to use</span> ${escapeHTML(help.use)}</p>
      <p><span class="chart-help-k">Good trend</span> ${escapeHTML(help.good)}</p>
    </div>
  </div>`;
}

/** DOM mount of chartHeading — prefer when building with createElement. */
export function mountChartHeading(
  parent: HTMLElement,
  title: string,
  help: ChartHelpCopy,
  level: 'h2' | 'h3' = 'h2',
): HTMLElement {
  const wrap = document.createElement('div');
  wrap.innerHTML = chartHeading(title, help, level);
  const el = wrap.firstElementChild as HTMLElement;
  parent.appendChild(el);
  return el;
}

function closeChartHelpPop(btn: HTMLButtonElement, pop: HTMLElement) {
  pop.hidden = true;
  btn.setAttribute('aria-expanded', 'false');
}

function openChartHelpPop(root: ParentNode, btn: HTMLButtonElement, pop: HTMLElement) {
  root.querySelectorAll<HTMLElement>('.chart-help-pop').forEach((other) => {
    if (other === pop || other.hidden) return;
    const otherBtn = other.parentElement?.querySelector<HTMLButtonElement>('.chart-help-btn');
    if (otherBtn) closeChartHelpPop(otherBtn, other);
  });
  pop.hidden = false;
  btn.setAttribute('aria-expanded', 'true');
}

/** Wire chart ℹ controls; safe to call after each view paint. Also binds KPI help. */
export function bindChartHelp(root: ParentNode = document): void {
  root.querySelectorAll<HTMLButtonElement>('.chart-help-btn:not(.kpi-help-btn)').forEach((btn) => {
    if (btn.dataset.bound === '1') return;
    btn.dataset.bound = '1';
    const pop = btn.parentElement?.querySelector<HTMLElement>('.chart-help-pop:not(.kpi-help-pop)');
    if (!pop) return;
    btn.addEventListener('click', (e) => {
      e.stopPropagation();
      if (pop.hidden) openChartHelpPop(root, btn, pop);
      else closeChartHelpPop(btn, pop);
    });
    btn.addEventListener('keydown', (e) => {
      if (e.key === 'Escape' && !pop.hidden) {
        e.preventDefault();
        closeChartHelpPop(btn, pop);
      }
    });
  });
  bindKpiHelp(root);
  const doc = root instanceof Document ? root : root.ownerDocument || document;
  if ((doc as Document & { __pfChartHelpDoc?: boolean }).__pfChartHelpDoc) return;
  (doc as Document & { __pfChartHelpDoc?: boolean }).__pfChartHelpDoc = true;
  doc.addEventListener('click', (e) => {
    const t = e.target as HTMLElement | null;
    if (t?.closest?.('.chart-head, .kpi-help')) return;
    doc.querySelectorAll<HTMLElement>('.chart-help-pop:not([hidden])').forEach((pop) => {
      const btn = pop.parentElement?.querySelector<HTMLButtonElement>('.chart-help-btn');
      if (btn) closeChartHelpPop(btn, pop);
    });
  });
  doc.addEventListener('keydown', (e) => {
    if (e.key !== 'Escape') return;
    doc.querySelectorAll<HTMLElement>('.chart-help-pop:not([hidden])').forEach((pop) => {
      const btn = pop.parentElement?.querySelector<HTMLButtonElement>('.chart-help-btn');
      if (btn) closeChartHelpPop(btn, pop);
    });
  });
}

export { bindKpiHelp, kpiHelpControl, kpiLabel } from './kpi-help';

export function drawerField(label: string, valueHTML: string, wrap = false): string {
  return `<dt>${escapeHTML(label)}</dt><dd class="${wrap ? 'wrap' : ''}">${valueHTML}</dd>`;
}

export function drawerBlock(title: string, bodyHTML: string): string {
  return `<section class="drawer-block"><h3>${escapeHTML(title)}</h3>${bodyHTML}</section>`;
}

/** Wire `[data-expand]` controls/cells; returns true when a handler ran. */
export function bindExpandControls(
  root: ParentNode,
  onExpand: (id: string, el: HTMLElement) => void,
): void {
  root.querySelectorAll<HTMLElement>('[data-expand]').forEach((el) => {
    el.addEventListener('click', (e) => {
      e.stopPropagation();
      const id = el.dataset.expand || '';
      if (id) onExpand(id, el);
    });
  });
}

export function idLink(href: string, id: string): string {
  return `<a class="id" href="${escapeHTML(href)}" title="${escapeHTML(id)}">${escapeHTML(id)}</a>`;
}

export function emptyState(title: string, body: string): string {
  return `<div class="empty"><h1>${escapeHTML(title)}</h1><p class="muted" style="margin-top:0.5rem">${escapeHTML(body)}</p></div>`;
}

export function filterEmpty(message: string): string {
  return `<tr class="empty-row"><td colspan="99"><div class="filter-empty">${escapeHTML(message)}</div></td></tr>`;
}

export function loadingState(label = 'Loading snapshot…'): string {
  return `<div class="loading" role="status" aria-live="polite">
    <div class="loading-bar" aria-hidden="true"></div>
    <p class="muted">${escapeHTML(label)}</p>
  </div>`;
}

/** Honest callout when issue-surface / sandbox leaves nearly everything NOT_APPLICABLE. */
export function issueSurfaceBanner(opts: {
  naRate?: unknown;
  adrAssessed?: unknown;
  adrTouched?: unknown;
  workshopBatch?: { total?: number; verdicts?: Record<string, number> } | null;
}): string {
  const na = Number(opts.naRate);
  const assessed = Number(opts.adrAssessed);
  const touched = Number(opts.adrTouched);
  const batch = opts.workshopBatch;
  const allInd =
    batch &&
    Number(batch.total || 0) > 0 &&
    Number(batch.verdicts?.INDETERMINATE || 0) === Number(batch.total || 0);
  const heavyNA = Number.isFinite(na) && na >= 0.9;
  const noAssessed = Number.isFinite(touched) && touched > 0 && Number.isFinite(assessed) && assessed === 0;
  if (!heavyNA && !noAssessed && !allInd) return '';
  return `<div class="banner warn" role="status">
    <strong>Issue-surface honesty.</strong>
    Analyzer rules are largely <code>NOT_APPLICABLE</code> on GitHub-issue / sandbox evidence — N/A is excluded from flag rate and must not be read as PASS.
    ${allInd ? ` Workshop batch: all ${escapeHTML(String(batch?.total))} submissions rolled up as INDETERMINATE.` : ''}
    <a href="#/docs/statuses">Status docs</a>
    · <a href="#/submissions">Submissions</a>
    · <a href="#/sample">Sample journey</a>
  </div>`;
}

export function staleBanner(label: string, detail?: string): string {
  return `<div class="banner warn stale-banner" role="status">
    <strong>Stale data</strong> — ${escapeHTML(label)}
    ${detail ? `<span class="faint"> · ${escapeHTML(detail)}</span>` : ''}
  </div>`;
}

export function sortHeader(label: string, key: string, sort: string, dir: 'asc' | 'desc'): string {
  const active = sort === key;
  const nextDir = active && dir === 'asc' ? 'desc' : 'asc';
  const arrow = !active ? '' : dir === 'asc' ? ' ↑' : ' ↓';
  return `<th class="sortable${active ? ' sorted' : ''}" data-sort="${escapeHTML(key)}" data-dir="${nextDir}" aria-sort="${
    active ? (dir === 'asc' ? 'ascending' : 'descending') : 'none'
  }" tabindex="0" role="columnheader">${escapeHTML(label)}${arrow}</th>`;
}

export function bindSortHeaders(
  thead: ParentNode,
  onSort: (key: string, dir: 'asc' | 'desc') => void,
): void {
  thead.querySelectorAll<HTMLElement>('th.sortable').forEach((th) => {
    const fire = () => {
      const key = th.dataset.sort || '';
      const dir = th.dataset.dir === 'desc' ? 'desc' : 'asc';
      if (key) onSort(key, dir);
    };
    th.addEventListener('click', fire);
    th.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        fire();
      }
    });
  });
}

export function compareValues(a: unknown, b: unknown, dir: 'asc' | 'desc'): number {
  const mul = dir === 'desc' ? -1 : 1;
  if (a == null && b == null) return 0;
  if (a == null) return 1 * mul;
  if (b == null) return -1 * mul;
  if (typeof a === 'number' && typeof b === 'number') return (a - b) * mul;
  return String(a).localeCompare(String(b), undefined, { numeric: true, sensitivity: 'base' }) * mul;
}

export function sortBy<T>(items: T[], key: string, dir: 'asc' | 'desc', getter: (row: T, key: string) => unknown): T[] {
  if (!key) return items;
  return items.slice().sort((a, b) => compareValues(getter(a, key), getter(b, key), dir));
}

export function entityLinks(items: Array<{ href: string; label: string; faint?: boolean }>): string {
  return `<span class="entity-links">${items
    .map((i) =>
      i.faint
        ? `<span class="faint">${escapeHTML(i.label)}</span>`
        : `<a href="${escapeHTML(i.href)}">${escapeHTML(i.label)}</a>`,
    )
    .join('')}</span>`;
}

export function openDrawer(opts: { title: string; bodyHTML: string; meta?: string; wide?: boolean }) {
  closeDrawer();
  const root = document.createElement('div');
  root.id = 'drawer-root';
  root.className = `drawer-root${opts.wide ? ' wide' : ''}`;
  root.innerHTML = `
    <div class="drawer-backdrop" data-drawer-close></div>
    <aside class="drawer" role="dialog" aria-modal="true" tabindex="-1" aria-label="${escapeHTML(opts.title)}">
      <header class="drawer-head">
        <div class="drawer-head-text">
          <h2 title="${escapeHTML(opts.title)}">${escapeHTML(opts.title)}</h2>
          ${opts.meta ? `<p class="muted" title="${escapeHTML(opts.meta)}">${escapeHTML(opts.meta)}</p>` : ''}
        </div>
        <button type="button" class="btn" data-drawer-close>Close</button>
      </header>
      <div class="drawer-body">${opts.bodyHTML}</div>
    </aside>`;
  document.body.appendChild(root);
  document.body.classList.add('drawer-open');
  root.querySelectorAll('[data-drawer-close]').forEach((el) => {
    el.addEventListener('click', () => closeDrawer());
  });
  const onKey = (e: KeyboardEvent) => {
    if (e.key === 'Escape') closeDrawer();
  };
  root.addEventListener('keydown', onKey);
  (root as HTMLElement & { _onKey?: (e: KeyboardEvent) => void })._onKey = onKey;
  window.addEventListener('keydown', onKey);
  root.querySelector<HTMLElement>('.drawer')?.focus();
}

export function closeDrawer() {
  const root = document.getElementById('drawer-root') as (HTMLElement & { _onKey?: (e: KeyboardEvent) => void }) | null;
  if (!root) return;
  if (root._onKey) window.removeEventListener('keydown', root._onKey);
  root.remove();
  document.body.classList.remove('drawer-open');
}

export function bindTableKeyboard(tbody: HTMLElement, opts: {
  onActivate: (row: HTMLElement) => void;
  onExpand?: (row: HTMLElement) => void;
}): () => void {
  let idx = -1;
  const rows = () => [...tbody.querySelectorAll<HTMLElement>('tr[data-row]')];
  const paint = () => {
    rows().forEach((r, i) => r.classList.toggle('row-focus', i === idx));
    rows()[idx]?.scrollIntoView({ block: 'nearest' });
  };
  const onKey = (e: KeyboardEvent) => {
    const tag = (e.target as HTMLElement | null)?.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') {
      if (e.key === 'Escape') (e.target as HTMLElement).blur();
      return;
    }
    const list = rows();
    if (!list.length) return;
    if (e.key === 'j' || e.key === 'ArrowDown') {
      e.preventDefault();
      idx = Math.min(list.length - 1, Math.max(0, idx + 1));
      paint();
    } else if (e.key === 'k' || e.key === 'ArrowUp') {
      e.preventDefault();
      idx = Math.max(0, idx <= 0 ? 0 : idx - 1);
      paint();
    } else if (e.key === 'Enter' && idx >= 0) {
      e.preventDefault();
      opts.onActivate(list[idx]);
    } else if ((e.key === 'o' || e.key === ' ') && idx >= 0 && opts.onExpand) {
      e.preventDefault();
      opts.onExpand(list[idx]);
    }
  };
  window.addEventListener('keydown', onKey);
  return () => window.removeEventListener('keydown', onKey);
}

export function bindSlashSearch(input: HTMLInputElement | null): () => void {
  if (!input) return () => {};
  const onKey = (e: KeyboardEvent) => {
    if (e.key !== '/' || e.metaKey || e.ctrlKey || e.altKey) return;
    const tag = (e.target as HTMLElement | null)?.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
    e.preventDefault();
    input.focus();
    input.select();
  };
  window.addEventListener('keydown', onKey);
  return () => window.removeEventListener('keydown', onKey);
}

export function toTSV(headers: string[], rows: string[][]): string {
  const esc = (s: string) => {
    if (/[\t\n\r"]/.test(s)) return `"${s.replace(/"/g, '""')}"`;
    return s;
  };
  return [headers.map(esc).join('\t'), ...rows.map((r) => r.map(esc).join('\t'))].join('\n');
}

export function toMarkdownTable(headers: string[], rows: string[][]): string {
  const line = (cells: string[]) => `| ${cells.map((c) => c.replace(/\|/g, '\\|')).join(' | ')} |`;
  return [line(headers), line(headers.map(() => '---')), ...rows.map(line)].join('\n');
}

export const DEFAULT_PAGE_SIZES = [25, 50] as const;

export type PageSlice<T> = {
  items: T[];
  page: number;
  pageSize: number;
  totalPages: number;
  total: number;
  from: number;
  to: number;
};

export function slicePage<T>(items: T[], page: number, pageSize: number): PageSlice<T> {
  const size = Math.max(1, pageSize || 25);
  const total = items.length;
  const totalPages = Math.max(1, Math.ceil(total / size) || 1);
  const safePage = Math.min(Math.max(1, page || 1), totalPages);
  const start = (safePage - 1) * size;
  return {
    items: items.slice(start, start + size),
    page: safePage,
    pageSize: size,
    totalPages,
    total,
    from: total === 0 ? 0 : start + 1,
    to: Math.min(start + size, total),
  };
}

export function renderPager(opts: {
  page: number;
  totalPages: number;
  total: number;
  from: number;
  to: number;
  pageSize: number;
  pageSizes?: readonly number[];
}): string {
  const sizes = opts.pageSizes ?? DEFAULT_PAGE_SIZES;
  return `<div class="pager">
    <span class="pager-info">${opts.from}–${opts.to} of ${opts.total}</span>
    <label class="pager-size">Rows
      <select data-pager-size aria-label="Rows per page">
        ${sizes
          .map((s) => `<option value="${s}"${s === opts.pageSize ? ' selected' : ''}>${s}</option>`)
          .join('')}
      </select>
    </label>
    <div class="pager-nav">
      <button type="button" class="btn" data-pager-prev${opts.page <= 1 ? ' disabled' : ''}>Prev</button>
      <span class="pager-page">${opts.page} / ${opts.totalPages}</span>
      <button type="button" class="btn" data-pager-next${opts.page >= opts.totalPages ? ' disabled' : ''}>Next</button>
    </div>
  </div>`;
}

export function bindPager(
  root: ParentNode,
  get: () => { page: number; pageSize: number },
  set: (next: { page: number; pageSize: number }) => void,
): void {
  root.querySelector<HTMLButtonElement>('[data-pager-prev]')?.addEventListener('click', () => {
    const cur = get();
    set({ page: Math.max(1, cur.page - 1), pageSize: cur.pageSize });
  });
  root.querySelector<HTMLButtonElement>('[data-pager-next]')?.addEventListener('click', () => {
    const cur = get();
    set({ page: cur.page + 1, pageSize: cur.pageSize });
  });
  root.querySelector<HTMLSelectElement>('[data-pager-size]')?.addEventListener('change', (e) => {
    const size = Number((e.target as HTMLSelectElement).value) || 25;
    set({ page: 1, pageSize: size });
  });
}

/** @deprecated kept for gradual migration */
export function tile(label: string, value: string, opts?: { cls?: string; sub?: string; href?: string }): string {
  return metricsRow([{ label, value, cls: opts?.cls, hint: opts?.sub, href: opts?.href }]);
}

export function pill(text: string, kind = ''): string {
  return tag(text, kind);
}

/** KPI compare: whether an increase from A→B is desirable. */
export type MetricPolarity = 'higher' | 'lower' | 'verdict' | 'neutral' | 'unknown';

/** Direction of B relative to A for a single metric. */
export type CompareDirection = 'better' | 'worse' | 'neutral' | 'unknown' | 'unchanged';

export type MetricCompareRow = {
  key: string;
  a: unknown;
  b: unknown;
  changed: boolean;
  polarity: MetricPolarity;
  direction: CompareDirection;
  /** Which side is preferred when direction is better/worse. */
  winner: 'a' | 'b' | null;
  deltaLabel: string;
  shortLabel: string;
};

const HIGHER_BETTER_EXACT = new Set([
  'confidence_high',
  'kpi.confidence.share.high',
  'kpi.harness.precision',
  'kpi.harness.recall',
  'kpi.harness.recall.high',
  'kpi.harness.recall.medium',
  'kpi.harness.recall.low',
  'kpi.review.accept_rate',
  'kpi.promptfoo.pass_rate',
  'kpi.assess.covered',
  'kpi.corpus.size',
  'kpi.adr.pass_count',
  'kpi.adr.assessed',
]);

const LOWER_BETTER_EXACT = new Set([
  'confidence_low',
  'kpi.confidence.share.low',
  'kpi.escalation.rate',
  'kpi.escalation.rate.tech_spec',
  'kpi.escalation.rate.code_snippet',
  'kpi.invariant.llm_calls',
  'kpi.assess.unbound',
  'kpi.assess.stale',
  'kpi.assess.na_rate',
  'kpi.assess.fail_share',
  'kpi.assess.indeterminate_share',
  'kpi.review.backlog',
  'kpi.review.pending_high',
  'kpi.review.pending_medium',
  'kpi.review.pending_low',
  'kpi.review.reject_rate',
  'kpi.evidence.gap_rate',
  'kpi.evidence.gap_rate.tech_spec',
  'kpi.evidence.gap_rate.code_snippet',
  'kpi.harness.fp_rate',
  'kpi.harness.false_positives',
  'kpi.assist.cost_usd',
  'kpi.assist.prompt_tokens',
  'kpi.assist.completion_tokens',
  'kpi.cost.per_accepted_proposal',
  'kpi.cost.per_fail',
  'kpi.adr.flag_rate',
  'kpi.adr.fail_count',
  'kpi.adr.indeterminate_count',
  'kpi.adr.top_flag_rate',
  'kpi.submission.unspecified_rate',
  'kpi.publish.age_minutes',
]);

const NEUTRAL_EXACT = new Set([
  'candidate_total',
  'confidence_medium',
  'kpi.confidence.share.medium',
  'kpi.assist.proposal_count',
  'kpi.submission.tech_spec_count',
  'kpi.submission.code_snippet_count',
  'schema_version',
]);

const UNKNOWN_EXACT = new Set([
  'kpi.assist.provider',
  'kpi.assist.model',
  'kpi.assess.submission_kind',
  'promptfoo_mode',
]);

const COMPARE_CALLOUT_KEYS = [
  'kpi.assess.verdict',
  'kpi.escalation.rate',
  'confidence_high',
  'confidence_low',
  'kpi.confidence.share.high',
  'kpi.confidence.share.low',
  'kpi.assist.cost_usd',
  'kpi.harness.precision',
  'kpi.harness.recall',
] as const;

const COMPARE_SHORT: Record<string, string> = {
  'kpi.assess.verdict': 'Verdict',
  'kpi.escalation.rate': 'Escalation',
  confidence_high: 'Confidence high',
  confidence_low: 'Confidence low',
  'kpi.confidence.share.high': 'High share',
  'kpi.confidence.share.low': 'Low share',
  'kpi.assist.cost_usd': 'Assist cost',
  'kpi.harness.precision': 'Precision',
  'kpi.harness.recall': 'Recall',
  'kpi.assist.model': 'Model',
  'kpi.assist.provider': 'Provider',
  'kpi.invariant.llm_calls': 'Assess LLM',
  'kpi.promptfoo.pass_rate': 'Promptfoo',
};

export function metricPolarity(key: string): MetricPolarity {
  if (key === 'kpi.assess.verdict') return 'verdict';
  if (HIGHER_BETTER_EXACT.has(key)) return 'higher';
  if (LOWER_BETTER_EXACT.has(key)) return 'lower';
  if (NEUTRAL_EXACT.has(key)) return 'neutral';
  if (UNKNOWN_EXACT.has(key)) return 'unknown';
  if (key.startsWith('kpi.harness.')) return 'higher';
  if (key.startsWith('kpi.review.pending')) return 'lower';
  if (key.includes('cost') || key.endsWith('_tokens')) return 'lower';
  if (key.includes('precision') || key.includes('recall') || key.includes('pass_rate') || key.includes('accept_rate')) {
    return 'higher';
  }
  if (key.includes('escalation') || key.includes('gap_rate') || key.includes('unbound') || key.includes('stale')) {
    return 'lower';
  }
  if (key.includes('provider') || key.includes('model') || key.endsWith('_mode')) return 'unknown';
  return 'neutral';
}

export function compareShortLabel(key: string): string {
  return COMPARE_SHORT[key] || key.replace(/^kpi\./, '');
}

function isMissing(v: unknown): boolean {
  return v == null || v === '';
}

function asFiniteNumber(v: unknown): number | null {
  if (typeof v === 'number' && Number.isFinite(v)) return v;
  if (typeof v === 'string' && v.trim() !== '' && !Number.isNaN(Number(v))) {
    const n = Number(v);
    return Number.isFinite(n) ? n : null;
  }
  return null;
}

function verdictRank(v: unknown): number | null {
  if (isMissing(v)) return null;
  const s = String(v).toUpperCase();
  if (s === 'PASS') return 2;
  if (s === 'WARN' || s === 'WARNING') return 1;
  if (s === 'FAIL') return 0;
  return null;
}

function formatSignedDelta(delta: number, isUsd: boolean, isRateLike: boolean): string {
  const sign = delta > 0 ? '+' : '';
  if (isUsd) {
    const abs = Math.abs(delta);
    const body = abs < 0.01 && abs !== 0 ? abs.toFixed(5) : abs.toFixed(4);
    return `${sign}$${body}`;
  }
  if (Number.isInteger(delta)) return `${sign}${delta}`;
  if (isRateLike && Math.abs(delta) < 1) {
    const pct = delta * 100;
    const pctStr = `${pct > 0 ? '+' : ''}${pct.toFixed(Math.abs(pct) >= 10 ? 1 : 2)}%`;
    return `${sign}${delta.toFixed(3)} (${pctStr} pts)`;
  }
  return `${sign}${delta.toFixed(3)}`;
}

function formatPctChange(a: number, b: number): string | null {
  if (a === 0) return null;
  const pct = ((b - a) / Math.abs(a)) * 100;
  if (!Number.isFinite(pct)) return null;
  const sign = pct > 0 ? '+' : '';
  return `${sign}${pct.toFixed(Math.abs(pct) >= 10 ? 0 : 1)}%`;
}

/** Classify A→B for one metric key and build a human delta label. */
export function classifyMetricCompare(key: string, a: unknown, b: unknown): MetricCompareRow {
  const polarity = metricPolarity(key);
  const shortLabel = compareShortLabel(key);
  const aMiss = isMissing(a);
  const bMiss = isMissing(b);
  const same = JSON.stringify(a) === JSON.stringify(b);

  if (same || (aMiss && bMiss)) {
    return {
      key,
      a,
      b,
      changed: false,
      polarity,
      direction: 'unchanged',
      winner: null,
      deltaLabel: '—',
      shortLabel,
    };
  }

  if (aMiss !== bMiss) {
    return {
      key,
      a,
      b,
      changed: true,
      polarity,
      direction: polarity === 'neutral' ? 'neutral' : 'unknown',
      winner: null,
      deltaLabel: aMiss ? 'new' : 'removed',
      shortLabel,
    };
  }

  const aN = asFiniteNumber(a);
  const bN = asFiniteNumber(b);
  if (aN != null && bN != null) {
    const delta = bN - aN;
    if (delta === 0) {
      return {
        key,
        a,
        b,
        changed: false,
        polarity,
        direction: 'unchanged',
        winner: null,
        deltaLabel: '—',
        shortLabel,
      };
    }
    const isUsd = key.includes('cost_usd') || key.endsWith('.cost');
    const isRateLike =
      key.includes('rate') ||
      key.includes('share') ||
      key.includes('precision') ||
      key.includes('recall') ||
      (Math.abs(aN) <= 1 && Math.abs(bN) <= 1 && !Number.isInteger(aN) && !Number.isInteger(bN));
    let deltaLabel = formatSignedDelta(delta, isUsd, isRateLike);
    if (!isRateLike) {
      const pct = formatPctChange(aN, bN);
      if (pct) deltaLabel = `${deltaLabel} · ${pct}`;
    }

    let direction: CompareDirection = 'neutral';
    let winner: 'a' | 'b' | null = null;
    if (polarity === 'higher') {
      direction = delta > 0 ? 'better' : 'worse';
      winner = delta > 0 ? 'b' : 'a';
    } else if (polarity === 'lower') {
      direction = delta < 0 ? 'better' : 'worse';
      winner = delta < 0 ? 'b' : 'a';
    } else if (polarity === 'unknown') {
      direction = 'unknown';
    } else {
      direction = 'neutral';
    }
    return { key, a, b, changed: true, polarity, direction, winner, deltaLabel, shortLabel };
  }

  if (polarity === 'verdict') {
    const ra = verdictRank(a);
    const rb = verdictRank(b);
    if (ra != null && rb != null && ra !== rb) {
      const better = rb > ra;
      return {
        key,
        a,
        b,
        changed: true,
        polarity,
        direction: better ? 'better' : 'worse',
        winner: better ? 'b' : 'a',
        deltaLabel: `${fmt(a)} → ${fmt(b)}`,
        shortLabel,
      };
    }
    return {
      key,
      a,
      b,
      changed: true,
      polarity,
      direction: 'unknown',
      winner: null,
      deltaLabel: `${fmt(a)} → ${fmt(b)}`,
      shortLabel,
    };
  }

  // String / mixed changes (model, provider, modes, etc.)
  return {
    key,
    a,
    b,
    changed: true,
    polarity,
    direction: polarity === 'neutral' ? 'neutral' : 'unknown',
    winner: null,
    deltaLabel: `${fmt(a)} → ${fmt(b)}`,
    shortLabel,
  };
}

export function buildMetricCompareRows(
  aMetrics: Record<string, unknown>,
  bMetrics: Record<string, unknown>,
): MetricCompareRow[] {
  const keys = [...new Set([...Object.keys(aMetrics), ...Object.keys(bMetrics)])].sort();
  return keys.map((k) => classifyMetricCompare(k, aMetrics[k], bMetrics[k]));
}

export type CompareSummary = {
  improved: number;
  regressed: number;
  unchanged: number;
  neutral: number;
  unknown: number;
  callouts: MetricCompareRow[];
};

export function summarizeMetricCompare(rows: MetricCompareRow[]): CompareSummary {
  let improved = 0;
  let regressed = 0;
  let unchanged = 0;
  let neutral = 0;
  let unknown = 0;
  for (const r of rows) {
    if (r.direction === 'better') improved += 1;
    else if (r.direction === 'worse') regressed += 1;
    else if (r.direction === 'unchanged') unchanged += 1;
    else if (r.direction === 'neutral') neutral += 1;
    else unknown += 1;
  }
  const byKey = new Map(rows.map((r) => [r.key, r]));
  const callouts = COMPARE_CALLOUT_KEYS.map((k) => byKey.get(k))
    .filter((r): r is MetricCompareRow => !!r && r.changed)
    .slice(0, 6);
  return { improved, regressed, unchanged, neutral, unknown, callouts };
}

export function compareDirectionClass(direction: CompareDirection): string {
  if (direction === 'better') return 'better';
  if (direction === 'worse') return 'worse';
  if (direction === 'unchanged') return 'same';
  if (direction === 'neutral') return 'neutral';
  return 'unknown';
}

/** Render value cell with optional winner/loser pill. */
export function compareValueCell(value: unknown, side: 'a' | 'b', row: MetricCompareRow): string {
  const text = escapeHTML(fmt(value));
  if (!row.changed) {
    return `<td class="num compare-cell compare-same"><span class="compare-val">${text}</span></td>`;
  }
  const isWinner = row.winner === side;
  const isLoser = row.winner != null && row.winner !== side;
  let tone = 'compare-changed';
  if (isWinner) tone = 'compare-better';
  else if (isLoser) tone = 'compare-worse';
  else if (row.direction === 'neutral') tone = 'compare-neutral';
  else tone = 'compare-unknown';
  return `<td class="num compare-cell ${tone}"><span class="compare-pill">${text}</span></td>`;
}

export function compareDeltaCell(row: MetricCompareRow): string {
  const cls = compareDirectionClass(row.direction);
  const tip =
    row.polarity === 'higher'
      ? 'Higher is better'
      : row.polarity === 'lower'
        ? 'Lower is better'
        : row.polarity === 'verdict'
          ? 'PASS better than FAIL'
          : row.polarity === 'unknown'
            ? 'Informational change'
            : 'No preferred direction';
  return `<td class="num compare-delta compare-delta-${cls}" title="${escapeHTML(tip)}"><span class="compare-delta-chip compare-delta-chip-${cls}">${escapeHTML(row.deltaLabel)}</span></td>`;
}

export function renderCompareSummaryStrip(summary: CompareSummary): string {
  const chips = [
    `<span class="compare-stat compare-stat-better"><strong>${summary.improved}</strong> improved</span>`,
    `<span class="compare-stat compare-stat-worse"><strong>${summary.regressed}</strong> regressed</span>`,
    `<span class="compare-stat compare-stat-same"><strong>${summary.unchanged}</strong> unchanged</span>`,
  ];
  if (summary.neutral) chips.push(`<span class="compare-stat compare-stat-neutral"><strong>${summary.neutral}</strong> neutral</span>`);
  if (summary.unknown) chips.push(`<span class="compare-stat compare-stat-unknown"><strong>${summary.unknown}</strong> info</span>`);

  const callouts = summary.callouts.length
    ? `<div class="compare-callouts">${summary.callouts
        .map((c) => {
          const cls = compareDirectionClass(c.direction);
          return `<span class="compare-callout compare-callout-${cls}" title="${escapeHTML(c.key)}"><span class="compare-callout-k">${escapeHTML(c.shortLabel)}${kpiHelpControl(c.key, c.shortLabel)}</span> <span class="compare-callout-d">${escapeHTML(c.deltaLabel)}</span></span>`;
        })
        .join('')}</div>`
    : `<p class="muted compare-callouts-empty">No headline KPI shifts between these runs.</p>`;

  return `<div class="compare-summary" role="status">
    <div class="compare-stats">${chips.join('')}</div>
    ${callouts}
  </div>`;
}

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
};

export function metricsRow(items: Metric[]): string {
  return `<div class="metrics">${items
    .map((m) => {
      const cls = safeClass(m.cls);
      const body = `<div class="label">${escapeHTML(m.label)}</div><div class="value${cls ? ` ${cls}` : ''}" title="${escapeHTML(m.value)}">${escapeHTML(m.value)}</div>${
        m.hint ? `<div class="hint" title="${escapeHTML(m.hint)}">${escapeHTML(m.hint)}</div>` : ''
      }`;
      const href = safeHref(m.href);
      return href
        ? `<a class="metric" href="${escapeHTML(href)}">${body}</a>`
        : `<div class="metric">${body}</div>`;
    })
    .join('')}</div>`;
}

/** DOM-safe metrics strip — prefer this over metricsRow + innerHTML. */
export function mountMetricsRow(parent: HTMLElement, items: Metric[]): HTMLElement {
  const row = document.createElement('div');
  row.className = 'metrics';
  for (const m of items) {
    const href = safeHref(m.href);
    const cell = document.createElement(href ? 'a' : 'div');
    cell.className = 'metric';
    if (href && cell instanceof HTMLAnchorElement) cell.href = href;

    const label = document.createElement('div');
    label.className = 'label';
    label.textContent = m.label;
    cell.appendChild(label);

    const value = document.createElement('div');
    value.className = 'value';
    const cls = safeClass(m.cls);
    if (cls) value.classList.add(cls);
    value.textContent = m.value;
    value.title = m.value;
    cell.appendChild(value);

    if (m.hint) {
      const hint = document.createElement('div');
      hint.className = 'hint';
      hint.textContent = m.hint;
      hint.title = m.hint;
      cell.appendChild(hint);
    }
    row.appendChild(cell);
  }
  parent.appendChild(row);
  return row;
}

export function statusItem(label: string, value: string, kind: 'ok' | 'warn' | 'bad' | '' = ''): string {
  const cls = safeClass(kind);
  return `<span class="item"><span class="dot${cls ? ` ${cls}` : ''}"></span>${escapeHTML(label)} <strong${cls ? ` class="${cls}"` : ''}>${escapeHTML(value)}</strong></span>`;
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
      const ok = await copyText(raw);
      const prev = btn.textContent;
      btn.textContent = ok ? 'Copied' : 'Failed';
      setTimeout(() => {
        btn.textContent = prev;
      }, 1200);
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

export function openDrawer(opts: { title: string; bodyHTML: string; meta?: string }) {
  closeDrawer();
  const root = document.createElement('div');
  root.id = 'drawer-root';
  root.className = 'drawer-root';
  root.innerHTML = `
    <div class="drawer-backdrop" data-drawer-close></div>
    <aside class="drawer" role="dialog" aria-modal="true" aria-label="${escapeHTML(opts.title)}">
      <header class="drawer-head">
        <div>
          <h2>${escapeHTML(opts.title)}</h2>
          ${opts.meta ? `<p class="muted">${escapeHTML(opts.meta)}</p>` : ''}
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

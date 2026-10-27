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
      const body = `<div class="label">${escapeHTML(m.label)}</div><div class="value${cls ? ` ${cls}` : ''}">${escapeHTML(m.value)}</div>${
        m.hint ? `<div class="hint">${escapeHTML(m.hint)}</div>` : ''
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
    cell.appendChild(value);

    if (m.hint) {
      const hint = document.createElement('div');
      hint.className = 'hint';
      hint.textContent = m.hint;
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

export function emptyState(title: string, body: string): string {
  return `<div class="empty"><h1>${escapeHTML(title)}</h1><p class="muted" style="margin-top:0.5rem">${escapeHTML(body)}</p></div>`;
}

/** @deprecated kept for gradual migration */
export function tile(label: string, value: string, opts?: { cls?: string; sub?: string; href?: string }): string {
  return metricsRow([{ label, value, cls: opts?.cls, hint: opts?.sub, href: opts?.href }]);
}

export function pill(text: string, kind = ''): string {
  return tag(text, kind);
}

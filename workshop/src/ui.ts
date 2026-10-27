export function escapeHTML(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
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
      const body = `<div class="label">${escapeHTML(m.label)}</div><div class="value ${m.cls || ''}">${m.value}</div>${
        m.hint ? `<div class="hint">${escapeHTML(m.hint)}</div>` : ''
      }`;
      return m.href
        ? `<a class="metric" href="${m.href}">${body}</a>`
        : `<div class="metric">${body}</div>`;
    })
    .join('')}</div>`;
}

export function statusItem(label: string, value: string, kind: 'ok' | 'warn' | 'bad' | '' = ''): string {
  return `<span class="item"><span class="dot ${kind}"></span>${escapeHTML(label)} <strong class="${kind}">${escapeHTML(value)}</strong></span>`;
}

export function tag(text: string, kind = ''): string {
  return `<span class="tag ${kind}">${escapeHTML(text)}</span>`;
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

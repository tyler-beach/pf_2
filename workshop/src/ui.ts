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

export function tile(label: string, value: string, opts?: { cls?: string; sub?: string; href?: string }): string {
  const cls = opts?.cls || '';
  const sub = opts?.sub ? `<div class="sub">${escapeHTML(opts.sub)}</div>` : '';
  const inner = `<div class="label">${escapeHTML(label)}</div><div class="value ${cls}">${value}</div>${sub}`;
  if (opts?.href) {
    return `<a class="tile clickable" href="${opts.href}">${inner}</a>`;
  }
  return `<div class="tile">${inner}</div>`;
}

export function pill(text: string, kind = ''): string {
  return `<span class="pill ${kind}">${escapeHTML(text)}</span>`;
}

export function setCrumb(text: string) {
  const el = document.getElementById('crumb');
  if (el) el.textContent = text;
}

export function setTopActions(html: string) {
  const el = document.getElementById('top-actions');
  if (el) el.innerHTML = html;
}

export function emptyState(title: string, body: string): string {
  return `<div class="panel flat"><h2>${escapeHTML(title)}</h2><p class="muted">${escapeHTML(body)}</p></div>`;
}

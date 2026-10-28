/** Hash-route query helpers: `#/path?k=v` */

export function pathAndQuery(): { path: string; query: URLSearchParams } {
  const raw = location.hash.replace(/^#/, '') || '/';
  const [pathPart, qs = ''] = raw.split('?');
  const path = pathPart.startsWith('/') ? pathPart : `/${pathPart}`;
  return { path: path || '/', query: new URLSearchParams(qs) };
}

export function getQuery(): URLSearchParams {
  return pathAndQuery().query;
}

export function getQueryValue(name: string): string | null {
  const v = getQuery().get(name);
  return v == null || v === '' ? null : v;
}

/** Merge updates into the current hash query. Null/empty removes the key. */
export function setQuery(updates: Record<string, string | number | null | undefined>, opts?: { replace?: boolean }) {
  const { path, query } = pathAndQuery();
  for (const [k, v] of Object.entries(updates)) {
    if (v == null || v === '') query.delete(k);
    else query.set(k, String(v));
  }
  const qs = query.toString();
  const next = `#${path}${qs ? `?${qs}` : ''}`;
  if (next === location.hash) return;
  if (opts?.replace) history.replaceState(null, '', next);
  else location.hash = next;
}

export function readInt(name: string, fallback: number, min = 1, max = 1_000_000): number {
  const raw = getQueryValue(name);
  const n = raw == null ? fallback : Number(raw);
  if (!Number.isFinite(n)) return fallback;
  return Math.min(max, Math.max(min, Math.trunc(n)));
}

export type ListQueryState = {
  q: string;
  page: number;
  pageSize: number;
  sort: string;
  dir: 'asc' | 'desc';
  band: string;
  /** tech_spec | code_snippet | all */
  kind: string;
};

export function readListQuery(defaults?: Partial<ListQueryState>): ListQueryState {
  const d: ListQueryState = {
    q: '',
    page: 1,
    pageSize: 25,
    sort: '',
    dir: 'asc',
    band: 'all',
    kind: 'all',
    ...defaults,
  };
  const dir = getQueryValue('dir');
  return {
    q: getQueryValue('q') ?? d.q,
    page: readInt('page', d.page),
    pageSize: readInt('rows', d.pageSize, 10, 200),
    sort: getQueryValue('sort') ?? d.sort,
    dir: dir === 'desc' ? 'desc' : dir === 'asc' ? 'asc' : d.dir,
    band: getQueryValue('band') ?? d.band,
    kind: getQueryValue('kind') ?? d.kind,
  };
}

export function writeListQuery(state: Partial<ListQueryState>, opts?: { replace?: boolean }) {
  setQuery(
    {
      q: state.q || null,
      page: state.page && state.page > 1 ? state.page : null,
      rows: state.pageSize && state.pageSize !== 25 ? state.pageSize : null,
      sort: state.sort || null,
      dir: state.sort ? state.dir || 'asc' : null,
      band: state.band && state.band !== 'all' ? state.band : null,
      kind: state.kind && state.kind !== 'all' ? state.kind : null,
    },
    opts,
  );
}

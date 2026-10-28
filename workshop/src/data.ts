import { getSelectedRunId } from './session';

export type MetricsRow = {
  run: {
    run_id: string;
    workflow: string;
    git_sha?: string;
    timestamp?: string;
    html_url?: string;
    provider_mode?: string;
  };
  metrics: Record<string, unknown>;
};

export type Latest = {
  schema_version: string;
  run: MetricsRow['run'];
  metrics: Record<string, unknown>;
  slo: Record<string, unknown>;
  links?: Record<string, string>;
  summary: string;
};

export type Manifest = {
  updated_at?: string;
  latest_run_id?: string;
  data_sha?: string;
  snapshots?: Array<{ run_id: string; path: string }>;
};

export type PipelineStep = {
  id: string;
  order: number;
  title: string;
  subtitle: string;
  command?: string;
  invariant?: string;
  inputs: string[];
  outputs: string[];
  deep: {
    what: string;
    how: string[];
    failure_modes: string[];
    metrics: string[];
    sample_artifact?: unknown;
  };
};

/** Annotated code/JSON panel inside a Sample journey step. */
export type JourneyPanel = {
  title: string;
  lang?: string;
  code: string;
  caption?: string;
  /** Soft shimmer to mark a key transition (e.g. YAML → evidence). */
  shimmer?: boolean;
};

export type JourneyDeep = {
  title?: string;
  body: string;
  panels?: JourneyPanel[];
};

export type SampleStep = {
  id: string;
  short?: string;
  title: string;
  /** Short “why this stage” blurb. */
  why?: string;
  /** Legacy alias for why — still rendered if why is absent. */
  narrative?: string;
  /** Deep technical body. Use [[term]] markers for glossary tooltips. */
  technical: string;
  commands?: string[];
  /** “What to notice” bullets. */
  notice?: string[];
  /** Legacy alias for notice. */
  checks?: string[];
  callouts?: string[];
  panels?: JourneyPanel[];
  deeper?: JourneyDeep;
  /** Glossary keys to surface as chips on this step. */
  chips?: string[];
  artifact_key?: string;
  artifact_label?: string;
  /** Transition label shown above shimmer panels (e.g. "YAML → ScanResult"). */
  highlight?: string;
};

export type SampleJourney = {
  title?: string;
  subtitle?: string;
  glossary?: Record<string, string>;
  steps: SampleStep[];
  artifacts?: Record<string, unknown>;
};

export type ADRSummary = {
  id: string;
  title: string;
  status: string;
  path: string;
  confidence_mix?: { high: number; medium: number; low: number };
  candidate_ids?: string[];
  body?: string;
};

export type CorpusEntry = {
  adr_id: string;
  statement_contains: string;
  modality: string;
  pattern_id: string;
  confidence_band: string;
  escalate_expected: boolean;
};

export type CorpusIndex = {
  schema_version: string;
  corpus_size: number;
  source?: string;
  entries: CorpusEntry[];
};

export type ActiveContext = {
  runId: string;
  prefix: string;
  artifactsFromSnapshot: boolean;
  latest: Latest | null;
  manifest: Manifest | null;
  runs: MetricsRow[];
};

function dataURL(path: string): string {
  const base = import.meta.env.BASE_URL || '/';
  const cleaned = `${base}data/${path}`.replace(/\/{2,}/g, '/').replace(':/', '://');
  return cleaned;
}

async function fetchJSON<T>(path: string): Promise<T | null> {
  try {
    const res = await fetch(dataURL(path));
    if (!res.ok) return null;
    return (await res.json()) as T;
  } catch {
    return null;
  }
}

async function fetchText(path: string): Promise<string | null> {
  try {
    const res = await fetch(dataURL(path));
    if (!res.ok) return null;
    return await res.text();
  } catch {
    return null;
  }
}

export function repoBase(): { owner: string; repo: string } {
  const parts = location.pathname.split('/').filter(Boolean);
  if (location.hostname.endsWith('github.io') && parts[0]) {
    return { owner: location.hostname.replace('.github.io', ''), repo: parts[0] };
  }
  return { owner: 'tyler-beach', repo: 'pf_2' };
}

export function workflowDispatchURL(file: string): string {
  const { owner, repo } = repoBase();
  return `https://github.com/${owner}/${repo}/actions/workflows/${file}`;
}

export function issueURL(title: string, body: string): string {
  const { owner, repo } = repoBase();
  const q = new URLSearchParams({ title, body });
  return `https://github.com/${owner}/${repo}/issues/new?${q.toString()}`;
}

export function githubBlobURL(repoPath: string): string {
  const { owner, repo } = repoBase();
  return `https://github.com/${owner}/${repo}/blob/main/${repoPath}`;
}

export const loadManifest = () => fetchJSON<Manifest>('manifest.json');
export const loadPipeline = () => fetchJSON<{ steps: PipelineStep[] }>('pipeline.json');
export const loadCorpusIndex = () => fetchJSON<CorpusIndex>('corpus.json');

/** Prefer the selected run snapshot, then any snapshot that ships a journey, then root. */
export async function loadSampleJourney(): Promise<SampleJourney | null> {
  const fromActive = await loadLinkJSON<SampleJourney>('sample-journey');
  if (fromActive?.steps?.length) return fromActive;
  const manifest = await loadManifest();
  for (const snap of manifest?.snapshots || []) {
    const prefix = `${snap.path.replace(/\/$/, '')}/`;
    const j = await fetchJSON<SampleJourney>(`${prefix}sample-journey.json`);
    if (j?.steps?.length) return j;
  }
  return fetchJSON<SampleJourney>('sample-journey.json');
}

export async function loadRuns(): Promise<MetricsRow[]> {
  const text = await fetchText('metrics/runs.jsonl');
  if (!text) return [];
  return text
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)
    .map((l) => JSON.parse(l) as MetricsRow);
}

async function resolvePrefix(runId: string, manifest: Manifest | null): Promise<{ prefix: string; ok: boolean }> {
  const listed = manifest?.snapshots?.find((s) => s.run_id === runId);
  if (listed?.path) {
    const prefix = `${listed.path.replace(/\/$/, '')}/`;
    const hit = await fetchJSON(`${prefix}latest.json`);
    if (hit) return { prefix, ok: true };
  }
  const guess = `snapshots/${runId}/`;
  const hit = await fetchJSON(`${guess}latest.json`);
  if (hit) return { prefix: guess, ok: true };
  return { prefix: '', ok: false };
}

export async function loadActiveContext(): Promise<ActiveContext> {
  const [manifest, runs, rootLatest] = await Promise.all([loadManifest(), loadRuns(), fetchJSON<Latest>('latest.json')]);
  const preferred =
    getSelectedRunId() ||
    manifest?.latest_run_id ||
    rootLatest?.run.run_id ||
    runs[runs.length - 1]?.run.run_id ||
    'latest';
  const { prefix, ok } = await resolvePrefix(preferred, manifest);
  let latest: Latest | null = null;
  if (ok) {
    latest = await fetchJSON<Latest>(`${prefix}latest.json`);
  }
  if (!latest && rootLatest) {
    const row = runs.find((r) => r.run.run_id === preferred);
    latest = row
      ? {
          ...rootLatest,
          run: { ...rootLatest.run, ...row.run },
          metrics: { ...rootLatest.metrics, ...row.metrics },
          summary: rootLatest.summary,
        }
      : rootLatest;
  }
  return {
    runId: preferred,
    prefix: ok ? prefix : '',
    artifactsFromSnapshot: ok,
    latest,
    manifest,
    runs,
  };
}

export async function loadLinkJSON<T>(key: string): Promise<T | null> {
  const ctx = await loadActiveContext();
  if (ctx.prefix) {
    const fromSnap = await fetchJSON<T>(`${ctx.prefix}${key}.json`);
    if (fromSnap) return fromSnap;
    const latest = ctx.latest;
    const rel = latest?.links?.[key];
    if (rel) {
      const nested = await fetchJSON<T>(`${ctx.prefix}${rel}`);
      if (nested) return nested;
    }
  }
  const rootLatest = await fetchJSON<Latest>('latest.json');
  const rel = rootLatest?.links?.[key];
  if (rel) return fetchJSON<T>(rel);
  return fetchJSON<T>(`${key}.json`);
}

export async function loadLatest(): Promise<Latest | null> {
  const ctx = await loadActiveContext();
  return ctx.latest;
}

export async function loadADRIndex(): Promise<{ adrs: ADRSummary[] } | null> {
  const ctx = await loadActiveContext();
  if (ctx.prefix) {
    const snap = await fetchJSON<{ adrs: ADRSummary[] }>(`${ctx.prefix}adrs/index.json`);
    if (snap) return snap;
  }
  return fetchJSON<{ adrs: ADRSummary[] }>('adrs/index.json');
}

export async function loadADR(id: string): Promise<ADRSummary | null> {
  const index = await loadADRIndex();
  const meta = index?.adrs.find((a) => a.id === id);
  if (!meta) return null;
  const ctx = await loadActiveContext();
  const path = meta.path.startsWith('adrs/') ? meta.path : `adrs/${meta.path}`;
  const body =
    (ctx.prefix ? await fetchText(`${ctx.prefix}${path}`) : null) ||
    (await fetchText(path)) ||
    meta.body ||
    '';
  return { ...meta, body };
}

export async function loadCosts(): Promise<Record<string, unknown> | null> {
  return loadLinkJSON<Record<string, unknown>>('costs');
}

export function previousRun(runs: MetricsRow[], runId: string): MetricsRow | undefined {
  const idx = runs.findIndex((r) => r.run.run_id === runId);
  if (idx <= 0) {
    // runs are chronological; find selected then take prior
    const ordered = runs;
    const i = ordered.findIndex((r) => r.run.run_id === runId);
    return i > 0 ? ordered[i - 1] : undefined;
  }
  return runs[idx - 1];
}

export function dataAge(iso?: string): { label: string; stale: boolean } {
  if (!iso) return { label: 'unknown age', stale: true };
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return { label: 'unknown age', stale: true };
  const mins = Math.max(0, Math.round((Date.now() - t) / 60000));
  if (mins < 60) return { label: `${mins}m ago`, stale: mins > 180 };
  const hours = Math.round(mins / 60);
  if (hours < 48) return { label: `${hours}h ago`, stale: hours > 24 };
  return { label: `${Math.round(hours / 24)}d ago`, stale: true };
}

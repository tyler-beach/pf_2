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

export type SampleStep = {
  id: string;
  title: string;
  technical: string;
  narrative: string;
  artifact_key?: string;
  callouts?: string[];
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

function dataURL(path: string): string {
  const base = import.meta.env.BASE_URL || '/';
  return `${base}data/${path}`.replace(/\/{2,}/g, '/').replace(':/', '://');
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

export const loadLatest = () => fetchJSON<Latest>('latest.json');
export const loadManifest = () => fetchJSON<Manifest>('manifest.json');
export const loadMetrics = () => fetchJSON<Record<string, unknown>>('metrics.json');
export const loadPipeline = () => fetchJSON<{ steps: PipelineStep[] }>('pipeline.json');
export const loadSampleJourney = () => fetchJSON<{ steps: SampleStep[]; artifacts: Record<string, unknown> }>('sample-journey.json');
export const loadADRIndex = () => fetchJSON<{ adrs: ADRSummary[] }>('adrs/index.json');
export const loadCosts = () => fetchJSON<Record<string, unknown>>('costs.json');

export async function loadRuns(): Promise<MetricsRow[]> {
  const text = await fetchText('metrics/runs.jsonl');
  if (!text) return [];
  return text
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)
    .map((l) => JSON.parse(l) as MetricsRow);
}

export async function loadLinkJSON<T>(key: string): Promise<T | null> {
  const latest = await loadLatest();
  const rel = latest?.links?.[key];
  if (!rel) return fetchJSON<T>(`${key}.json`);
  return fetchJSON<T>(rel);
}

export async function loadADR(id: string): Promise<ADRSummary | null> {
  const index = await loadADRIndex();
  const meta = index?.adrs.find((a) => a.id === id);
  if (!meta) return null;
  const body = await fetchText(meta.path);
  return { ...meta, body: body || meta.body || '' };
}

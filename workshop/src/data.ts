export type Manifest = {
  latest_run_id: string;
  updated_at: string;
  data_sha: string;
};

export type Latest = {
  schema_version: string;
  run: {
    run_id: string;
    workflow: string;
    git_sha: string;
    timestamp: string;
    html_url?: string;
    provider_mode?: string;
  };
  metrics: Record<string, unknown>;
  slo: Record<string, unknown>;
  links?: Record<string, string>;
  summary: string;
};

export type MetricsRow = {
  run: Latest['run'];
  metrics: Record<string, unknown>;
};

const bust = () => `?t=${Date.now()}`;

async function getJSON<T>(path: string): Promise<T | null> {
  try {
    const res = await fetch(`${path}${bust()}`);
    if (!res.ok) return null;
    return (await res.json()) as T;
  } catch {
    return null;
  }
}

export async function loadManifest(): Promise<Manifest | null> {
  return getJSON<Manifest>('data/manifest.json');
}

export async function loadLatest(): Promise<Latest | null> {
  return getJSON<Latest>('data/latest.json');
}

export async function loadRuns(): Promise<MetricsRow[]> {
  try {
    const res = await fetch(`data/metrics/runs.jsonl${bust()}`);
    if (!res.ok) return [];
    const text = await res.text();
    return text
      .split('\n')
      .map((l) => l.trim())
      .filter(Boolean)
      .map((l) => JSON.parse(l) as MetricsRow);
  } catch {
    return [];
  }
}

export async function loadLinkJSON<T>(name: string): Promise<T | null> {
  const latest = await loadLatest();
  const rel = latest?.links?.[name] || `${name}.json`;
  const candidates: string[] = [];
  if (latest?.run?.run_id) {
    candidates.push(`data/snapshots/${latest.run.run_id}/${rel}`);
    candidates.push(`data/snapshots/${latest.run.run_id}/${name}.json`);
  }
  candidates.push(`data/${rel}`);
  candidates.push(`data/${name}.json`);
  for (const path of candidates) {
    const value = await getJSON<T>(path);
    if (value) return value;
  }
  return null;
}

export function repoBase(): { owner: string; repo: string } {
  // Best-effort from Pages URL path; override via data/repo.json if present.
  const host = window.location.hostname;
  if (host.endsWith('github.io')) {
    const owner = host.replace('.github.io', '');
    const parts = window.location.pathname.split('/').filter(Boolean);
    return { owner, repo: parts[0] || 'pf_2' };
  }
  return { owner: 'tyler-beach', repo: 'pf_2' };
}

export function issueURL(title: string, body: string): string {
  const { owner, repo } = repoBase();
  return `https://github.com/${owner}/${repo}/issues/new?title=${encodeURIComponent(title)}&body=${encodeURIComponent(body)}`;
}

export function workflowDispatchURL(workflowFile: string): string {
  const { owner, repo } = repoBase();
  return `https://github.com/${owner}/${repo}/actions/workflows/${workflowFile}`;
}

const RUN_KEY = 'pf.selected_run_id';

export function getSelectedRunId(): string | null {
  try {
    return localStorage.getItem(RUN_KEY);
  } catch {
    return null;
  }
}

export function setSelectedRunId(runId: string | null) {
  try {
    if (!runId) localStorage.removeItem(RUN_KEY);
    else localStorage.setItem(RUN_KEY, runId);
  } catch {
    /* ignore */
  }
  window.dispatchEvent(new CustomEvent('pf:run-change', { detail: { runId } }));
}

export function queryParam(name: string): string | null {
  const q = location.hash.split('?')[1] || '';
  return new URLSearchParams(q).get(name);
}

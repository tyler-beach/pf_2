import { dataAge, loadActiveContext, loadRuns, workflowDispatchURL } from './data';
import { escapeHTML } from './ui';
import { getSelectedRunId, setSelectedRunId } from './session';

let bound = false;

export async function renderChrome() {
  const picker = document.getElementById('run-picker') as HTMLSelectElement | null;
  const ageEl = document.getElementById('data-age');
  const publish = document.getElementById('dispatch-publish') as HTMLAnchorElement | null;
  const assist = document.getElementById('dispatch-assist') as HTMLAnchorElement | null;

  const [ctx, runs] = await Promise.all([loadActiveContext(), loadRuns()]);
  const selected = getSelectedRunId() || ctx.runId;
  const ids = new Set<string>();
  if (ctx.manifest?.latest_run_id) ids.add(ctx.manifest.latest_run_id);
  for (const r of runs) ids.add(r.run.run_id);
  for (const s of ctx.manifest?.snapshots || []) ids.add(s.run_id);

  if (picker) {
    picker.innerHTML = [...ids]
      .map((id) => {
        const row = runs.find((r) => r.run.run_id === id);
        const label = row ? `${id} · ${row.run.workflow}` : id;
        return `<option value="${escapeHTML(id)}" ${id === selected ? 'selected' : ''}>${escapeHTML(label)}</option>`;
      })
      .join('') || `<option value="${escapeHTML(selected)}">${escapeHTML(selected)}</option>`;
  }

  if (ageEl) {
    const age = dataAge(ctx.manifest?.updated_at || ctx.latest?.run.timestamp);
    ageEl.textContent = age.stale ? `Stale · ${age.label}` : `Data ${age.label}`;
    ageEl.classList.toggle('stale', age.stale);
    ageEl.title = age.stale
      ? `Last publish ${ctx.manifest?.updated_at || 'unknown'} — refresh via Workshop Publish`
      : ctx.manifest?.updated_at || 'No publish timestamp';
    ageEl.setAttribute('role', age.stale ? 'status' : 'note');
  }

  if (publish) publish.href = workflowDispatchURL('workshop-publish.yml');
  if (assist) assist.href = workflowDispatchURL('llm-assist.yml');

  if (!bound && picker) {
    bound = true;
    picker.addEventListener('change', () => {
      setSelectedRunId(picker.value);
    });
  }
}

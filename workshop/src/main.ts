import 'uplot/dist/uPlot.min.css';
import 'diff2html/bundles/css/diff2html.min.css';
import { renderChrome } from './chrome';
import { bindCopyButtons, closeDrawer, loadingState } from './ui';
import {
  renderADRs,
  renderAssess,
  renderCandidates,
  renderCorpus,
  renderCosts,
  renderDocs,
  renderEvals,
  renderHome,
  renderMetrics,
  renderPipeline,
  renderProposals,
  renderRuns,
  renderSample,
} from './views';

const navGroups: Array<{ label: string; items: Array<[string, string]> }> = [
  {
    label: 'Command',
    items: [
      ['/', 'Overview'],
      ['/metrics', 'Metrics'],
      ['/pipeline', 'Pipeline'],
      ['/sample', 'Sample'],
    ],
  },
  {
    label: 'Evidence',
    items: [
      ['/adrs', 'ADRs'],
      ['/candidates', 'Candidates'],
      ['/proposals', 'Proposals'],
      ['/assess', 'Assess'],
    ],
  },
  {
    label: 'Ops',
    items: [
      ['/costs', 'LLM cost'],
      ['/runs', 'Runs'],
      ['/evals', 'Evals'],
      ['/corpus', 'Corpus'],
      ['/docs', 'Docs'],
    ],
  },
];

function routeParts(): { path: string; rest: string[] } {
  const raw = location.hash.replace(/^#/, '') || '/';
  const [pathPart] = raw.split('?');
  const segs = pathPart.split('/').filter(Boolean);
  if (!segs.length) return { path: '/', rest: [] };
  return { path: `/${segs[0]}`, rest: segs.slice(1) };
}

function nav() {
  const el = document.getElementById('nav')!;
  const { path } = routeParts();
  el.innerHTML = navGroups
    .map((g) => {
      const items = g.items
        .map(([p, label]) => {
          const active =
            p === '/'
              ? path === '/'
              : path === p || (p !== '/' && location.hash.startsWith(`#${p}`));
          return `<a href="#${p}" class="${active ? 'active' : ''}">${label}</a>`;
        })
        .join('');
      return `<div class="group">${g.label}</div>${items}`;
    })
    .join('');
}

let renderGen = 0;

async function render() {
  const gen = ++renderGen;
  closeDrawer();
  nav();
  const app = document.getElementById('app')!;
  document.body.classList.add('is-loading');
  app.innerHTML = loadingState();
  await renderChrome();
  if (gen !== renderGen) return;
  const { path, rest } = routeParts();
  switch (path) {
    case '/':
      await renderHome(app);
      break;
    case '/metrics':
      await renderMetrics(app);
      break;
    case '/pipeline':
      await renderPipeline(app, rest[0]);
      break;
    case '/sample':
      await renderSample(app, rest[0]);
      break;
    case '/adrs':
      await renderADRs(app, rest[0]);
      break;
    case '/candidates':
      await renderCandidates(app);
      break;
    case '/proposals':
      await renderProposals(app);
      break;
    case '/assess':
      await renderAssess(app);
      break;
    case '/costs':
      await renderCosts(app);
      break;
    case '/runs':
      await renderRuns(app);
      break;
    case '/evals':
      await renderEvals(app);
      break;
    case '/corpus':
      await renderCorpus(app);
      break;
    case '/docs':
      await renderDocs(app, rest[0]);
      break;
    case '/kpis':
      document.body.classList.remove('is-loading');
      location.hash = '#/metrics';
      return;
    default:
      app.innerHTML = `<div class="empty"><h1>Not found</h1><p class="muted" style="margin-top:0.5rem"><a href="#/">Back to overview</a></p></div>`;
  }
  if (gen !== renderGen) return;
  document.body.classList.remove('is-loading');
  bindCopyButtons(app);
}

document.getElementById('nav-toggle')?.addEventListener('click', () => {
  document.getElementById('sidebar')?.classList.toggle('open');
});

window.addEventListener('hashchange', () => {
  document.getElementById('sidebar')?.classList.remove('open');
  void render();
});
window.addEventListener('pf:run-change', () => {
  void render();
});
void render();

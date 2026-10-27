import 'uplot/dist/uPlot.min.css';
import 'diff2html/bundles/css/diff2html.min.css';
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

async function render() {
  nav();
  const app = document.getElementById('app')!;
  const { path, rest } = routeParts();
  switch (path) {
    case '/':
      return renderHome(app);
    case '/metrics':
      return renderMetrics(app);
    case '/pipeline':
      return renderPipeline(app, rest[0]);
    case '/sample':
      return renderSample(app, rest[0]);
    case '/adrs':
      return renderADRs(app, rest[0]);
    case '/candidates':
      return renderCandidates(app);
    case '/proposals':
      return renderProposals(app);
    case '/assess':
      return renderAssess(app);
    case '/costs':
      return renderCosts(app);
    case '/runs':
      return renderRuns(app);
    case '/evals':
      return renderEvals(app);
    case '/corpus':
      return renderCorpus(app);
    case '/docs':
      return renderDocs(app);
    case '/kpis':
      location.hash = '#/metrics';
      return;
    default:
      app.innerHTML = `<div class="empty"><h1>Not found</h1><p class="muted" style="margin-top:0.5rem"><a href="#/">Back to overview</a></p></div>`;
  }
}

document.getElementById('nav-toggle')?.addEventListener('click', () => {
  document.getElementById('sidebar')?.classList.toggle('open');
});

window.addEventListener('hashchange', () => {
  document.getElementById('sidebar')?.classList.remove('open');
  void render();
});
void render();

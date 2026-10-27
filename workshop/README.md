# Pathfinder Workshop UI

Vite + TypeScript SPA published to GitHub Pages.

```bash
npm ci
npm run dev    # http://localhost:5173
npm run build
```

Data is read from `public/data/` (`latest.json`, `manifest.json`, `metrics/runs.jsonl`, `snapshots/<run_id>/`). CI publish merges the `workshop-data` branch into `public/data` before build.

Deep links open GitHub Issues / Actions; the static site holds no secrets.

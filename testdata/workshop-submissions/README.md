# Workshop submissions (body of evidence)

50 real third-party excerpts for Pathfinder workshop evaluation:

| Kind | Count | Directory |
|------|------:|-----------|
| `tech_spec` | 25 | `tech_spec/` |
| `code_snippet` | 25 | `code_snippet/` |

Each issue Markdown body includes:

- Title suffix `[tech_spec]` or `[code_snippet]`
- HTML comment `submission_kind: …`
- Provenance (`source_url`, `repo`, `path`, `license`, `retrieved`)
- Structured entry in `manifest.json`

Excerpts are workshop-sized (tens of lines) from public MIT/Apache/BSD (and a few AGPL/CC-BY) projects — not vendored whole repos.

## Assess all 50

```bash
# Optional: pin scanners (auto-detected under ~/.cache/pathfinder-tools when present)
# export OPENGREP_BIN=... OPENGREP_SHA256=... CHECKOV_BIN=... CHECKOV_SHA256=...
./scripts/assess_workshop_submissions.sh
```

When OpenGrep + Checkov are available, code fences are extracted to a sandbox, scanned, and assessed with `--issue-surface` / `--submission-kind` / `--scope` (applicable-evidence path). Prose-only `tech_spec` submissions and scanner-less environments fall back to assess-only applicability filtering (analyzer rows N/A — not a pre-epic flood).

Outputs:

- `results/<id>/{issue.md,assessment.json,report.md,results.sarif,meta.json,assess_meta.json}` (+ `sandbox/` / `analyzer-evidence/` when scanned)
- `results/index.json` / `results/submissions.json` (kind counts, verdicts, `assess_mode`, applicable counts)
- `results/candidates.json` (kind-tagged candidates for workshop KPIs)
- `workshop-snapshot/` ready for `scripts/workshop_publish_data.py`

Publish into local workshop data (and commit for Pages):

```bash
python3 scripts/workshop_publish_data.py \
  --snapshot testdata/workshop-submissions/workshop-snapshot \
  --data-dir workshop/public/data
```

Then open the workshop (`cd workshop && npm run dev`) → Metrics / Candidates / Runs.
Filter candidates with `#/candidates?kind=tech_spec` or `kind=code_snippet`.

Live Pages: https://tyler-beach.github.io/pf_2/

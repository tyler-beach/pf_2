# Pathfinder PR scan (reviewdog + Security SARIF)

Paste a GitHub **PR URL** (this repository or another) into the **Pathfinder PR scan** issue template. Pathfinder assesses the PR head with repository evidence mode, then publishes structured feedback through:

1. **reviewdog** — sparse inline comments / checks / annotations on deviations
2. **GitHub code scanning SARIF** — Security tab alerts (CodeQL/SAST-like)

Both surfaces share one finding model and the same short comment body.

## How to use

1. **Issues → New issue → Pathfinder PR scan (reviewdog + Security)**
2. Paste the PR URL.
3. Check one or more **reviewdog** feedback modes and/or **Upload SARIF to GitHub code scanning**.
4. Keep the `[Pathfinder PR Scan]` title prefix (or the `pathfinder-pr-scan` label).
5. The workflow only runs for issue authors with `OWNER` / `MEMBER` / `COLLABORATOR` association (blocks public-issue confused-deputy abuse of review tokens).
6. Wait for the workflow; it comments a summary on the target PR and on the scan issue.

### Shared finding body (all reporters + Security)

```
[ERROR] ADR-003 · No dynamic shell
Why: Avoid shell invocation
Evidence: `internal/run.go:14`
Next: Fix deviation for REQ-003-01 — https://…/#/docs/pr-scan
```

N/A and PASS requirements are **never** inlined or uploaded. Inline comments are capped (default 25); the PR summary counts omitted noise and caps.

## reviewdog reporters supported

| Checkbox | Effect |
|---|---|
| `github-pr-review` | Inline review comments (diff-aware; outside-diff falls back to Check annotations in Actions) |
| `github-pr-check` | Checks API annotations |
| `github-check` | Commit Check Run |
| `github-pr-annotations` | Actions job annotations |
| `local` | Artifact-only (`reviewdog -reporter=local`) |

Filter modes: `added` (default), `diff_context`, `file`, `nofilter`. Fail level uses reviewdog `-fail-level` (`none`/`any`/`warning`/`error`).

## Security tab (SARIF)

When the Security checkbox is selected, Pathfinder emits SARIF 2.1.0 (`pathfinder-security.sarif`) with:

- Stable `ruleId` values (`pathfinder/<requirement_id>`)
- `partialFingerprints.primaryLocationLineHash` for dedupe across runs
- Rule help / remediation + docs `helpUri`
- Locations with file + line when known
- `automationDetails.id = pathfinder/pr-scan`

Upload uses the GitHub **code scanning SARIFS API** (`scripts/upload_code_scanning_sarif.sh`) against `refs/pull/<n>/head` so alerts associate with the scanned PR/commit. Same-repo runs can use `GITHUB_TOKEN` with `security-events: write`. Cross-repo needs a PAT/App token.

Code scanning must be available on the **target** repository (public repos: enabled by default for GitHub Advanced Security features that apply; private repos typically need GHAS).

## Cross-repo tokens

| Secret | Purpose |
|---|---|
| `PATHFINDER_REVIEW_TOKEN` | **Required** for other repos. Needs `contents:read`, `pull_requests:write`, `checks:write` on the target. Also used for clone + reviewdog + summary comment. |
| `PATHFINDER_SARIF_TOKEN` | Optional override for Security upload only (`security_events: write` + `contents:read`). Falls back to `PATHFINDER_REVIEW_TOKEN` / `GITHUB_TOKEN`. |

Same-repository scans can use the default `GITHUB_TOKEN` (workflow already grants `pull-requests`, `checks`, `security-events`).

### Limitations

- Fork PRs: commenting/Security upload still needs write access on the **base** repo; head may live on a fork (workflow clones `head.repo`).
- Private target repos: token must reach that repo; otherwise fetch/upload fails closed.
- `GITHUB_TOKEN` from this workflow **cannot** write to foreign repos — set `PATHFINDER_REVIEW_TOKEN`.
- Outside collaborators / anonymous issue authors cannot trigger scans (author-association gate).
- Pathfinder never executes PR workflows, `npm install`, or untrusted binaries from the head tree. Only pinned OpenGrep/Checkov + `adrctl` from this repository run.
- Duplicate sticky summaries for the same head SHA are skipped; reviewdog may still refresh comments depending on reporter.
- SARIF upload validates owner/repo/sha/ref shapes and bounds file size before calling the code-scanning API.

## Operator CLI

```bash
# Parse a PR URL
go run ./cmd/adrctl parse-pr-url 'https://github.com/org/repo/pull/123'

# Parse an issue form body
go run ./cmd/adrctl parse-pr-scan-issue --body /tmp/issue.md

# Convert assessment JSON → rdjson + Security SARIF + summary
go run ./cmd/adrctl assessment-to-rdjson \
  --assessment assessment.json \
  --rdjson-out pathfinder.rdjson \
  --sarif-out pathfinder-security.sarif \
  --summary-out pathfinder-pr-summary.md \
  --pr-url 'https://github.com/org/repo/pull/123' \
  --head-sha "$SHA"
```

## Relationship to existing flows

- **ADR Assessment** (`[ADR Assessment]`) — issue sandbox / corpus assess; unchanged.
- **CI reviewdog** — same-repo SARIF annotations on Pathfinder PRs; unchanged.
- **PR scan** — external or same-repo PR URL via issue template; additive.

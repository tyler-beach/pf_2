# Pathfinder PR scan (reviewdog + Security SARIF)

Paste a GitHub **PR URL** (this repository or another) into the **Pathfinder PR scan** issue template, or run **Actions → Pathfinder PR Scan → Run workflow**. Pathfinder assesses the PR head with repository evidence mode, then publishes structured feedback through:

1. **reviewdog** — sparse inline comments / checks / annotations on deviations
2. **GitHub code scanning SARIF** — Security tab alerts (CodeQL/SAST-like)

Both surfaces share one finding model and the same short comment body.

**Trust boundaries (grill):** no untrusted PR code execution; author-association gate on issue triggers; findings = deviations only (never N/A/PASS); comment cap + summary.

## How to use

### A. Issue template

1. **Issues → New issue → Pathfinder PR scan (reviewdog + Security)**
2. Paste the PR URL.
3. Check one or more **reviewdog** feedback modes and/or **Upload SARIF to GitHub code scanning**.
4. Keep the `[Pathfinder PR Scan]` title prefix (or the `pathfinder-pr-scan` label).
5. The workflow only runs for issue authors with `OWNER` / `MEMBER` / `COLLABORATOR` association (blocks public-issue confused-deputy abuse of review tokens).
6. Wait for the workflow; it comments a summary on the target PR and on the scan issue.

### B. workflow_dispatch (no issue required)

1. **Actions → Pathfinder PR Scan → Run workflow**
2. Set `pr_url` and toggle feedback flags (`github_pr_review`, `upload_sarif`, …).
3. Run. Same pipeline as the issue path; issue-status comment is skipped.

### Shared finding body (all reporters + Security)

```
[ERROR] ADR-003 · No dynamic shell
Why: Avoid shell invocation
Evidence: `internal/run.go:14`
Next: Fix deviation for REQ-003-01 — https://…/#/docs/pr-scan
```

N/A and PASS requirements are **never** inlined or uploaded. Inline comments are capped (default 25); the PR summary counts omitted noise and caps.

## Secrets matrix

| Secret | Same-repo | Cross-repo | Required scopes |
|---|---|---|---|
| _(none / `GITHUB_TOKEN`)_ | Sufficient when workflow grants `pull-requests`, `checks`, `security-events` | **Cannot** write foreign repos | workflow permissions below |
| `PATHFINDER_REVIEW_TOKEN` | Optional override | **Required** | `contents:read`, `pull_requests:write`, `checks:write` on the **target** repo (also clone + reviewdog + summary comment). Add `security_events:write` if the same token uploads SARIF. |
| `PATHFINDER_SARIF_TOKEN` | Optional | Optional | `security_events:write` + `contents:read` on target. Overrides review token for SARIF upload only; falls back to `PATHFINDER_REVIEW_TOKEN` then `GITHUB_TOKEN`. |

### Same-repo vs cross-repo

| Mode | Token | Notes |
|---|---|---|
| Same repository | Default `GITHUB_TOKEN` | Workflow already grants `pull-requests: write`, `checks: write`, `security-events: write`. |
| Other public/private repo | `PATHFINDER_REVIEW_TOKEN` (PAT or GitHub App install token) | Must reach head clone URL + Checks/PR APIs on **base** repo. |
| Fork PR | Token with write on **base** | Head may live on a fork; workflow clones `head.repo`. |

## Status check name (branch protection)

reviewdog posts under the stable name **`pathfinder-pr-scan`**. Require this check in branch protection when using `github-pr-check` / `github-check` reporters. Do not rename without a coordinated docs + workflow bump.

## Known-FAIL fixture + dry-run

Fixture pack: [`testdata/pr-scan/known-fail/`](../testdata/pr-scan/known-fail/) (`internal/bad/shell.go` intentional shell composition, golden `assessment-fail.json`, golden issue body).

### Scripted local E2E (no GitHub PR / Actions click required)

Proves issue parse, `workflow_dispatch` request shape, converter (rdjson + Security SARIF, deviation-only), and pass/deviation assess oracles:

```bash
./scripts/pr_scan_local_e2e.sh
```

### Manual Actions click-path (same-repo FAIL PR)

1. Open (or reuse) a PR that includes `testdata/pr-scan/known-fail/internal/bad/shell.go`.
2. **Actions → Pathfinder PR Scan → Run workflow** with that `pr_url`, `catalog=live`, `github_pr_review=true`, optionally `upload_sarif=true`.
3. Confirm: check **`pathfinder-pr-scan`**, deviation-only inline comments, PR sticky summary with marker `pathfinder-pr-scan:<sha>`, optional Security tab finding.
4. Record the Actions run URL in the PR/issue for the dry-run checklist.

Cross-repo dry-run: point `pr_url` at another repo PR with `PATHFINDER_REVIEW_TOKEN` set; expect 403/clone failures when the token cannot reach the target (fail closed).

## reviewdog reporters supported

| Checkbox / dispatch input | Effect |
|---|---|
| `github-pr-review` | Inline review comments (diff-aware; outside-diff falls back to Check annotations in Actions) |
| `github-pr-check` | Checks API annotations |
| `github-check` | Commit Check Run |
| `github-pr-annotations` | Actions job annotations |
| `local` | Artifact-only (`reviewdog -reporter=local`) |

Filter modes: `added` (default), `diff_context`, `file`, `nofilter`. Fail level uses reviewdog `-fail-level` (`none`/`any`/`warning`/`error`).

## Security tab (SARIF)

When the Security checkbox / `upload_sarif` is selected, Pathfinder emits SARIF 2.1.0 (`pathfinder-security.sarif`) with:

- Stable `ruleId` values (`pathfinder/<requirement_id>`)
- `partialFingerprints.primaryLocationLineHash` for dedupe across runs
- Rule help / remediation + docs `helpUri`
- Locations with file + line when known
- `automationDetails.id = pathfinder/pr-scan`

Upload uses the GitHub **code scanning SARIFS API** (`scripts/upload_code_scanning_sarif.sh`) against `refs/pull/<n>/head` so alerts associate with the scanned PR/commit.

Code scanning must be available on the **target** repository (public repos: enabled by default for GitHub Advanced Security features that apply; private repos typically need GHAS).

## Applicability + scope

PR changed-path prefixes feed both scanner `--scope` and `assess-repo --scope` so non-intersecting corpus rows become `NOT_APPLICABLE` by applicability (Epic 2) instead of flooding reporters. Use `--assess-all` only in full-corpus CI.

## Workshop deep link

Scan issue comments include a Pages dig-in:

`https://tyler-beach.github.io/pf_2/#/runs?run_id=<GITHUB_RUN_ID>`

## Limitations

- Fork PRs: commenting/Security upload still needs write access on the **base** repo; head may live on a fork (workflow clones `head.repo`).
- Private target repos: token must reach that repo; otherwise fetch/upload fails closed.
- `GITHUB_TOKEN` from this workflow **cannot** write to foreign repos — set `PATHFINDER_REVIEW_TOKEN`.
- Outside collaborators / anonymous issue authors cannot trigger **issue** scans (author-association gate). `workflow_dispatch` is limited to users who can run Actions.
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
- **PR scan** — external or same-repo PR URL via issue template or `workflow_dispatch`; additive.
- **Reusable workflow** — see `.github/workflows/pathfinder-pr-scan-reusable.yml` + `examples/consumer-pr-scan.yml` (Epic 3).

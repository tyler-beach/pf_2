# Known-FAIL PR-scan fixture

Intentional OpenGrep hit (`internal/bad/shell.go`) for Pathfinder PR-scan dry-runs.

| Artifact | Purpose |
|---|---|
| `internal/bad/shell.go` | Source that should FAIL `REQ-003-01` when scanned |
| `assessment-fail.json` | Golden FAIL assessment (no scanner required) for converter E2E |
| `issue-body.md` | Golden issue-form body for `parse-pr-scan-issue` |

## Local E2E (no GitHub PR required)

```bash
./scripts/pr_scan_local_e2e.sh
```

## Manual Actions click-path (same-repo)

1. Open a PR that adds/changes `testdata/pr-scan/known-fail/internal/bad/shell.go` (so analyzer `--scope` includes `testdata`).
2. **Actions → Pathfinder PR Scan → Run workflow** (`workflow_dispatch`) — production demo / items 1–2:
   - `pr_url`: the PR URL
   - enable `github_pr_review` + `github_pr_check` (+ optional `upload_sarif`)
   - `catalog`: `live`
3. **App path** (after App install + `APP_ID` / `APP_PRIVATE_KEY`): **Actions → Pathfinder App PR Scan → Run workflow** with the same `pr_url` (see [docs/github-app-design.md](../../../docs/github-app-design.md)).
4. Or **Issues → Pathfinder PR scan** with the same URL (author must be OWNER/MEMBER/COLLABORATOR).
5. Expect reviewdog check name **`pathfinder-pr-scan`**, deviation-only comments, optional Security tab alert.

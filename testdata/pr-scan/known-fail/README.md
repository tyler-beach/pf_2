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

1. Open a PR that adds/changes `testdata/pr-scan/known-fail/internal/bad/shell.go` (or point at any PR that already contains it).
2. **Actions → Pathfinder PR Scan → Run workflow** (`workflow_dispatch`):
   - `pr_url`: the PR URL
   - enable `github_pr_review` + `upload_sarif`
   - `catalog`: `live`
3. Or **Issues → Pathfinder PR scan** with the same URL (author must be OWNER/MEMBER/COLLABORATOR).
4. Expect reviewdog check name **`pathfinder-pr-scan`**, deviation-only comments, optional Security tab alert.

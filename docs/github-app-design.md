# GitHub App design (Phase B)

**Status:** design only. Ship the [reusable workflow](../.github/workflows/pathfinder-pr-scan-reusable.yml) first; implement the App when multi-org install + least-privilege install tokens matter more than a PAT.

## Why an App (later)

| Reusable workflow + PAT | GitHub App |
|---|---|
| Works today for any repo that can `uses:` this repo | Better install UX across orgs |
| PAT is long-lived / user-scoped unless carefully minted | Short-lived installation tokens |
| Secrets copied per consumer repo | Central install; revoke per org |

## Permissions (requested)

| Permission | Access | Why |
|---|---|---|
| `checks` | write | reviewdog `github-pr-check` / `github-check` |
| `pull_requests` | write | summary comment + `github-pr-review` |
| `contents` | read | clone PR head as **data** only |
| `security_events` | write | optional code-scanning SARIF upload |
| `metadata` | read | default |

**Do not request:** `workflows` write, `administration`, `members`, or any write on `contents` (Pathfinder never pushes to consumer trees).

## Webhooks

| Event | Action | Behavior |
|---|---|---|
| `pull_request` | `opened`, `synchronize`, `reopened` | Queue the same scan pipeline as the reusable workflow (`pr_url` from payload) |
| `installation` / `installation_repositories` | `created` / `added` | Record install + repos; no scan |
| `ping` | — | Health |

Optional later: `issue_comment` with a `/pathfinder scan` command (still require install membership).

## Runtime

1. Verify webhook signature (`X-Hub-Signature-256`).
2. Exchange installation ID → installation access token (≤1h).
3. Invoke the same host-trusted pipeline: checkout Pathfinder pin → fetch PR head → OpenGrep/Checkov → assess (`--scope` + applicability) → reviewdog (`-name=pathfinder-pr-scan`) → optional SARIF.
4. Never execute workflows, package installs, or binaries from the PR head.

## Out of scope for MVP App

- UI marketplace listing polish
- Auto-merge / commit fixes
- Org-wide policy admin console (workshop Pages remains the analytics surface)

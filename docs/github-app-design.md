# GitHub App (Pathfinder PR Scan)

**Status:** Actions-native MVP shipped (see `feat(app): Actions-native GitHub App MVP for PR scan` on `main`). Reusable workflow remains the scan engine; the App supplies short-lived installation tokens instead of a long-lived PAT. A custom webhook server is **out of scope** for MVP (optional later).

**Workshop Pages:** operator walkthrough at [`#/docs/github-app`](https://tyler-beach.github.io/pf_2/#/docs/github-app) · integrations map [`#/docs/integrations`](https://tyler-beach.github.io/pf_2/#/docs/integrations) · PR scan [`#/docs/pr-scan`](https://tyler-beach.github.io/pf_2/#/docs/pr-scan).

| Reusable workflow + PAT | GitHub App (this MVP) |
|---|---|
| Works today for any repo that can `uses:` this repo | Better install UX; revoke per org/repo |
| PAT is long-lived / user-scoped unless carefully minted | Installation tokens ≤1h via `actions/create-github-app-token` |
| Secrets copied per consumer repo | Central App; per-repo install |

## What ships in-repo

| Artifact | Role |
|---|---|
| [`.github/pathfinder-app-manifest.json`](../.github/pathfinder-app-manifest.json) | Create-from-manifest permissions + events |
| [`.github/workflows/pathfinder-app-pr-scan.yml`](../.github/workflows/pathfinder-app-pr-scan.yml) | Host: `pull_request` / dispatch → mint token → reusable scan |
| [`.github/workflows/pathfinder-pr-scan-reusable.yml`](../.github/workflows/pathfinder-pr-scan-reusable.yml) | Scan pipeline (unchanged contract) |
| [`examples/consumer-pr-scan-app.yml`](../examples/consumer-pr-scan-app.yml) | Consumer copy-paste using App token |
| [`examples/consumer-pr-scan.yml`](../examples/consumer-pr-scan.yml) | Consumer using PAT (`PATHFINDER_REVIEW_TOKEN`) |

## Permissions (requested)

| Permission | Access | Why |
|---|---|---|
| `checks` | write | reviewdog `github-pr-check` / `github-check` |
| `pull_requests` | write | summary comment + `github-pr-review` |
| `contents` | read | clone PR head as **data** only |
| `security_events` | write | optional code-scanning SARIF upload |
| `metadata` | read | default |

**Do not request:** `workflows` write, `administration`, `members`, or any write on `contents` (Pathfinder never pushes to consumer trees).

The host/consumer workflows further **narrow** the minted token with `permission-checks`, `permission-pull-requests`, `permission-contents`, `permission-security-events` on `create-github-app-token`.

## Events (manifest)

| Event | Action | Behavior (MVP) |
|---|---|---|
| `pull_request` | `opened`, `synchronize`, `reopened` | Listed on the App for future webhook use; **Actions-native** path uses the repo workflow `on: pull_request` instead |
| `installation` / `installation_repositories` | `created` / `added` | Install bookkeeping only (no scan) |
| `ping` | — | Health (when webhook active) |

Manifest webhook is **inactive** (`hook_attributes.active: false`). Optional later: external webhook verifier (`X-Hub-Signature-256`) + `/pathfinder scan` comment command.

## Secrets checklist

| Name | Required | Where | Value |
|---|---|---|---|
| `APP_ID` | Yes\* | Repo/org **Actions secret** | Numeric App ID from App settings |
| `APP_PRIVATE_KEY` | Yes | Repo/org **Actions secret** | Entire PEM (including `BEGIN/END` lines). Escaped `\n` OK |
| `APP_CLIENT_ID` | Optional | Repo/org **Actions secret** | Client ID from App settings; when set, host workflow prefers it over `APP_ID` |

\*Provide `APP_ID` and/or `APP_CLIENT_ID` (at least one) plus `APP_PRIVATE_KEY`.

Still supported for PAT consumers: `PATHFINDER_REVIEW_TOKEN`, `PATHFINDER_SARIF_TOKEN` (see [pr-scan.md](pr-scan.md)).

## Create the App in GitHub UI (exact clicks)

### Option A — Create from manifest (fastest)

1. Open **https://github.com/settings/apps/new** while logged in as the owner that should own the App (user or org).
2. Or use the manifest flow: open  
   `https://github.com/settings/apps/new?state=pathfinder&manifest=…`  
   after URL-encoding the contents of [`.github/pathfinder-app-manifest.json`](../.github/pathfinder-app-manifest.json), **or** paste fields manually from Option B (same permissions).
3. Confirm **Create GitHub App** / accept the manifest permissions.
4. On the App page → **Private keys** → **Generate a private key** → download the `.pem`.
5. Note **App ID** and **Client ID** from the App’s General page.

### Option B — Manual create

1. GitHub avatar (top right) → **Settings** → **Developer settings** → **GitHub Apps** → **New GitHub App**.  
   Org: `https://github.com/organizations/<ORG>/settings/apps/new`
2. **GitHub App name:** `Pathfinder PR Scan` (must be unique).
3. **Homepage URL:** `https://github.com/tyler-beach/pf_2`
4. **Webhook:** uncheck **Active** (Actions-native MVP; no server).
5. **Repository permissions:**
   - Checks → **Read and write**
   - Contents → **Read-only**
   - Pull requests → **Read and write**
   - Security events → **Read and write**
   - Metadata → **Read-only** (default)
6. **Subscribe to events** (optional for MVP): Pull request, Installation, Installation repositories.
7. **Where can this GitHub App be installed?** → Only on this account (or Any account if you will distribute).
8. **Create GitHub App**.
9. **Private keys → Generate a private key** → save the `.pem`.
10. Copy **App ID** (and optional **Client ID**).

### Install on a repository

1. App settings → **Install App** (left nav) → choose account → **Only select repositories** → select `pf_2` (and later any consumer repo) → **Install**.
2. Confirm the install shows the permissions above.

### Paste secrets into the host / consumer repo

1. Target repo → **Settings** → **Secrets and variables** → **Actions** → **New repository secret**.
2. Create:
   - `APP_ID` = numeric App ID
   - `APP_PRIVATE_KEY` = full PEM contents
   - (optional) `APP_CLIENT_ID` = Client ID
3. Re-run **Actions → Pathfinder App PR Scan** (or open/sync a PR).

Until secrets exist, the host workflow **skips** with a notice (does not fail every PR).

## Runtime (Actions-native)

1. Workflow runs on `pull_request` / `workflow_dispatch` in the installed repo (trusted workflow file from default branch / PR base rules).
2. `actions/create-github-app-token` exchanges App credentials → installation access token (≤1h), scoped to the current repository + least-privilege permission inputs.
3. Caller passes that token as `review_token` / `sarif_token` into [pathfinder-pr-scan-reusable.yml](../.github/workflows/pathfinder-pr-scan-reusable.yml).
4. Same host-trusted pipeline: checkout Pathfinder pin → fetch PR head as **data** → OpenGrep/Checkov → assess → reviewdog (`-name=pathfinder-pr-scan`) → optional SARIF.
5. Never execute workflows, package installs, or binaries from the PR head.

## Trust / security grill

- **Least privilege:** App perms are read contents + write checks/PRs/security_events only; minted tokens request the same subset.
- **No untrusted code exec:** PR tree is input data to pinned host analyzers/`adrctl` only.
- **Author / install trust:** App scans only run where the App is installed and workflow files are trusted. Issue-based PR scan (separate workflow) still gates on `OWNER`/`MEMBER`/`COLLABORATOR`. `workflow_dispatch` requires Actions write. Fork PRs: write lands on **base**; head may be a fork clone URL.
- **Fail closed:** missing cross-repo token / App install → clone or API errors; no silent skip of auth failures once secrets are configured.
- **Do not** grant `contents: write`, `workflows`, or admin.

## Same-repo E2E

1. Install App on `tyler-beach/pf_2` and set `APP_ID` + `APP_PRIVATE_KEY`.
2. Open (or reuse) a PR that touches `testdata/pr-scan/known-fail/internal/bad/shell.go`.
3. **Actions → Pathfinder App PR Scan → Run workflow** with that `pr_url` (or push to the PR), `catalog=live`.
4. Confirm check **`pathfinder-pr-scan`**, sticky summary marker `pathfinder-pr-scan:<sha>`, optional Security upload.

Without App secrets yet, use the production dispatch path (same pipeline, `GITHUB_TOKEN`): **Actions → Pathfinder PR Scan → Run workflow** — see [pr-scan.md](pr-scan.md).

## Cross-repo notes

- Install the App on **each** target repo (or all repos in the org).
- Mint token with `repositories` including the target; for multi-repo installs expand the `repositories` input (comma-separated) or omit it for all installed repos in the account (broader token — prefer explicit lists).
- If a second-repo install is not possible from this environment: ship code + docs, run **same-repo** E2E, and record the Actions run URL. Cross-repo dry-run without install fails closed (403/clone).

## Out of scope for MVP App

- Marketplace listing polish
- Auto-merge / commit fixes
- Hosted webhook worker + signature verification service
- Org-wide policy admin console (workshop Pages remains the analytics surface)

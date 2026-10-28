# Dependency upgrade ladder

Controlled path to latest pins (audit baseline ~2026-10-28). Prefer **sequenced commits per ecosystem**, each green before the next. Prefer **Node 24 LTS** over Node 26 for CI.

## Rollback

Known-good tree backup (pre-upgrade snapshot):

`/Users/coding/backups/pf_2-backup-20261028-001828`

Also: `git revert` of the offending upgrade commit(s), or `git checkout <sha> -- <paths>` for a single pin.

## Ladder (order)

| Step | Ecosystem | Scope | Local gate |
|------|-----------|--------|------------|
| 0 | Docs | This file | n/a |
| 1 | Go | `go` toolchain + goldmark | `./scripts/go_test.sh` |
| 2 | workshop npm | diff/@types/diff, marked, typescript, vite, uplot (keep dompurify/diff2html) | `cd workshop && npm ci && npm run build` |
| 3 | Actions + Node | SHA-pin latest majors (checkout, setup-go, setup-node); Node → 24 LTS; leave Pages/upload-artifact if current | workflow review; Node steps use 24 |
| 4 | Analyzers + reviewdog | Checkov pip, OpenGrep release+SHA, reviewdog `go install` | fixture smoke (Checkov/OpenGrep); `./scripts/go_test.sh` |
| 5 | promptfoo | `eval/promptfoo` → latest | `cd eval/promptfoo && npm ci && npm run eval:offline` |

Pass/deviation assess oracles after Go or analyzer bumps when assessment defaults change. Do not silently leave half-upgraded pins: if a major breaks, **pin previous known-good with a WHY comment** and record it under Holdbacks below.

## Holdbacks

| Pin | Kept / dropped | WHY |
|-----|----------------|-----|
| `@types/diff` | **removed** (was 7.0.2) | `diff@9` ships its own typings; `@types/diff@8.0.0` is a deprecated stub. |
| `dompurify` / `diff2html` | kept at 3.4.17 / 3.4.56 | Already current at audit time; not bumped. |
| CI OpenGrep lang-extension oracle | adapted for `.sh` | Fixture `owasp_promote_misc.sh` yields an extra extension under OpenGrep 1.33; CI list now includes `sh`. |
| Node 26 (Current) | **held**; CI uses Node **24** LTS | Prefer LTS for CI stability unless the repo explicitly wants Current. |
| Pages / upload-artifact | left at current SHAs | Already latest majors at audit time. |

## Final pin table

| Component | From (audit) | To |
|-----------|--------------|-----|
| Go toolchain | 1.23 (`go.mod`) | **1.27** + `toolchain go1.27.3` |
| goldmark | v1.7.8 | **v1.8.6** |
| workshop `diff` | 8.0.3 | **9.0.0** |
| workshop `@types/diff` | 7.0.2 | **removed** (diff ships types) |
| workshop `marked` | 15.0.7 | **18.1.2** |
| workshop `typescript` | 5.8.2 | **7.0.2** |
| workshop `vite` | 6.4.5 | **8.3.5** |
| workshop `uplot` | 1.6.31 | **1.6.32** |
| workshop `dompurify` / `diff2html` | 3.4.17 / 3.4.56 | unchanged |
| Node (CI) | 22 | **24** LTS |
| actions/checkout | v5.1.0 | **v7.0.1** (`3d3c42e…`) |
| actions/setup-go | v6.5.0 | **v7.0.0** (`b7ad1da…`) |
| actions/setup-node | v5.0.0 | **v7.1.0** (`949feb2…`) |
| Pages / upload-artifact | already current | unchanged |
| OpenGrep | 1.30.0 / SHA `35779bdd…` | **1.33.0** / SHA `25458127…` (linux x86) |
| Checkov | 3.2.484 | **3.3.30** (BinarySHA256 computed at install) |
| reviewdog | v0.20.3 | **v0.21.2** |
| promptfoo | 0.114.7 | **0.125.1** |

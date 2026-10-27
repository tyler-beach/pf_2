# Open-source tool adoption decisions

## Purpose

This document turns the original analyzer shortlist into explicit integration decisions. A tool is adopted only when it closes a demonstrated capability gap, passes representative positive/negative fixtures, preserves provenance and coverage semantics, and has an acceptable license/supply-chain profile.

Pathfinder's core remains a deterministic assessment and evidence-orchestration engine. Existing Go/JSON verifiers, ADR ingestion, reviewed obligation registry, OpenGrep adapter, and Markdown/JSON/SARIF outputs remain authoritative. Do not make an optional tool a runtime dependency of the assessment core.

## Decisions

| Tool | Decision | Where it fits | Gate before adoption |
|---|---|---|---|
| [ADG/ADE](https://github.com/adr/ad-guidance-tool) | **Prototype as an optional authoring adapter; do not replace the obligation registry.** | ADG manages decision models and generates rule templates; ADE validates/compiles/verifies rule DSL through plugins. This can help architects author and syntax-check candidate rules, especially when the existing ecosystem has a suitable Go/.NET/filesystem plugin. | Verify exact rule semantics and supported plugins; map its rule/ADR IDs to Pathfinder source-bound obligation IDs; retain source hashes, human approval and append-only review history. No ADE output becomes blocking without parity fixtures and provenance. |
| [Arch-Go](https://github.com/arch-go/arch-go) | **Benchmark for Go repository architecture only.** | Independent package/dependency architecture rules and reports may complement the current Go AST import-boundary verifier at repository scope. | Run the same allowed/forbidden import and package-boundary fixtures through both engines. Compare false positives, exclusions, location quality, runtime and maintenance. Keep the standard-library parser for submitted snippets that lack module context. Do not enable duplicate checks until parity is proven. |
| [reviewdog](https://github.com/reviewdog/reviewdog) | **Keep; strengthen regression validation.** | Consumes normalized SARIF and publishes PR review annotations. It is a presentation/integration layer, not a scanner, verdict engine, or proof of coverage. | Validate the PR workflow on a same-repository PR containing a known finding and a clean case. Confirm annotation path/line/rule/message and least-privilege permissions. Fork PRs remain constrained by token permissions. |
| [Promptfoo](https://github.com/promptfoo/promptfoo) | **Defer runtime integration; prepare an optional eval harness only when LLM-assisted interpretation is implemented.** | Regression evaluation for prompts/models used in batch-time ADR interpretation, ambiguity handling, structured obligation extraction, and explanations. | No provider/model calls in standard CI. Use a fixed labeled dataset, deterministic assertions for schema/source citations/approval state, explicit opt-in credentials for model comparisons, and cost/latency recording. Promptfoo scores are not a compliance verdict. |

## Important distinction: ADG/ADE identity

The referenced [adr/ad-guidance-tool](https://github.com/adr/ad-guidance-tool) repository exposes ADG commands and an ADE-backed `adg enforce` workflow. Related plugins are separate projects with their own prerequisites and licenses. Do not confuse this project with other repositories also named ADG. Pin an exact release/commit and plugin version before any automated use.

## Implementation order

1. **Arch-Go benchmark first.** It is the smallest bounded experiment and tests a real current capability: repository-level Go architecture constraints. Keep it non-blocking until fixture parity and output quality are reviewed.
2. **Reviewdog validation next.** It is already configured in `.github/workflows/ci.yml`; add a same-repository PR regression that proves findings arrive as useful annotations. Do not infer success merely because the command exits zero.
3. **ADG/ADE adapter spike.** Start with rule validation and authoring only. Import/export reviewed rule candidates through a narrow adapter; keep Pathfinder's obligation/review schema and normalized evidence contract authoritative. Do not run arbitrary generated build/test commands from untrusted issue content.
4. **Promptfoo only when there is an LLM-backed feature to evaluate.** Current deterministic extraction and runtime assessment do not need it. When added, isolate it in a manually dispatched or credential-gated workflow with pinned version, fixed dataset, and explicit spend limits.

## Acceptance criteria shared by every integration

- Exact version or immutable commit/digest recorded.
- License reviewed for engine, plugins, dependencies, and rule packs independently.
- Vulnerable/negative and compliant/positive fixtures.
- Reproducible output and preserved raw output.
- Normalized findings include tool identity/version, rule ID, source path/range, scope, status, configuration/input hashes, and artifact provenance.
- Unsupported scope, missing evidence, tool error, timeout, and partial analysis never become PASS.
- The analyzer runs once per repository snapshot; findings are reused across obligations.
- New blocking checks are not enabled until CI demonstrates expected behavior on representative fixtures and a same-repository PR.

## Current state

- Arch-Go: a PR-triggered, fixture-only benchmark is now implemented in `.github/workflows/ci.yml`. It requires a clean dependency-boundary fixture to pass, a forbidden domain-to-infrastructure dependency to fail, and Pathfinder's existing clean/deviation issue fixtures to retain their expected verdicts. This is a benchmark, not yet a production repository gate; adoption depends on its CI result and a follow-up review of output parity.
- OpenGrep: pinned repository scanner and normalized evidence adapter are implemented; CI exercises vulnerable and clean multilingual fixtures.
- reviewdog: configured in the main CI workflow for same-repository pull requests; live annotation quality still requires an explicit PR-path regression.
- ADG/ADE: not integrated; authoring/validation prototype decision pending.
- Promptfoo: not integrated; intentionally deferred while no LLM-backed production feature exists.
- Runtime model calls: zero.

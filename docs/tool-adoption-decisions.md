# Open-source tool adoption decisions

## Purpose

This document turns the original analyzer shortlist into explicit integration decisions. A tool is adopted only when it closes a demonstrated capability gap, passes representative positive/negative fixtures, preserves provenance and coverage semantics, and has an acceptable license/supply-chain profile.

Pathfinder's core remains a deterministic assessment and evidence-orchestration engine. Existing Go/JSON verifiers, ADR ingestion, reviewed obligation registry, OpenGrep adapter, and Markdown/JSON/SARIF outputs remain authoritative. Do not make an optional tool a runtime dependency of the assessment core.

## Decisions

| Tool | Decision | Where it fits | Gate before adoption |
|---|---|---|---|
| [ADG/ADE](https://github.com/adr/ad-guidance-tool) | **Prototype as an optional authoring adapter; do not replace the obligation registry.** | ADG manages decision models and generates rule templates; ADE validates/compiles/verifies rule DSL through plugins. This can help architects author and syntax-check candidate rules, especially when the existing ecosystem has a suitable Go/.NET/filesystem plugin. | Verify exact rule semantics and supported plugins; map its rule/ADR IDs to Pathfinder source-bound obligation IDs; retain source hashes, human approval and append-only review history. No ADE output becomes blocking without parity fixtures and provenance. |
| [Arch-Go](https://github.com/arch-go/arch-go) | **Benchmark for Go repository architecture only.** | Independent package/dependency architecture rules and reports may complement the current Go AST import-boundary verifier at repository scope. | Run the same allowed/forbidden import and package-boundary fixtures through both engines. Compare false positives, exclusions, location quality, runtime and maintenance. Keep the standard-library parser for submitted snippets that lack module context. Do not enable duplicate checks until parity is proven. |
| [Conftest / OPA](https://github.com/open-policy-agent/conftest) | **Benchmark for structured configuration policies; keep existing JSON verifiers for current simple rules.** | Rego can express nested, cross-field, or multi-document configuration constraints that may exceed the built-in required-key and numeric-range verifiers. | Run clean and violating JSON fixtures through both engines; compare PASS/FAIL, evidence messages, reproducibility, and maintenance. Pin Conftest/OPA, verify release checksums, and review the engine's license and dependencies. Do not make Conftest a runtime dependency or switch a production binding without a demonstrated benefit and parity tests. |
| [reviewdog](https://github.com/reviewdog/reviewdog) | **Keep; strengthen regression validation.** | Consumes normalized SARIF and publishes PR review annotations. It is a presentation/integration layer, not a scanner, verdict engine, or proof of coverage. | Validate the PR workflow on a same-repository PR containing a known finding and a clean case. Confirm annotation path/line/rule/message and least-privilege permissions. Fork PRs remain constrained by token permissions. |
| [Promptfoo](https://github.com/promptfoo/promptfoo) | **Adopt for batch assist eval only.** Offline suite in `eval/promptfoo/` gates CI; live compare is credential-gated in `llm-assist.yml`. | Regression evaluation for prompts/models used in batch-time ADR interpretation (`assist-obligations`). | No provider/model calls in standard CI (offline fixture provider). Fixed labeled cases, assertions for schema/source citations/`approval_status` never approved/`executable=false`. Promptfoo scores are **not** a compliance verdict and never feed `assess` / PR gates. |

## Important distinction: ADG/ADE identity

The referenced [adr/ad-guidance-tool](https://github.com/adr/ad-guidance-tool) repository exposes ADG commands and an ADE-backed `adg enforce` workflow. Related plugins are separate projects with their own prerequisites and licenses. Do not confuse this project with other repositories also named ADG. Pin an exact release/commit and plugin version before any automated use.

## Implementation order

1. **Arch-Go benchmark first.** It is the smallest bounded experiment and tests a real current capability: repository-level Go architecture constraints. Keep it non-blocking until fixture parity and output quality are reviewed.
2. **Conftest/OPA benchmark for structured configuration.** Compare required-key and numeric timeout checks against the existing JSON verifiers using the same positive and negative inputs. Retain the existing implementation unless a more complex policy justifies the additional tool.
3. **Reviewdog validation next.** It is already configured in `.github/workflows/ci.yml`; add a same-repository PR regression that proves findings arrive as useful annotations. Do not infer success merely because the command exits zero.
4. **ADG/ADE adapter spike.** Start with rule validation and authoring only. Import/export reviewed rule candidates through a narrow adapter; keep Pathfinder's obligation/review schema and normalized evidence contract authoritative. Do not run arbitrary generated build/test commands from untrusted issue content.
5. **Promptfoo evaluates batch assist only.** Runtime assessment stays zero-LLM. Offline Promptfoo runs in `ci.yml`; live model comparison is isolated to `llm-assist.yml` with pinned version, fixed dataset, and spend limits.

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
- Conftest/OPA: a CI-only [Conftest v0.71.1](https://github.com/open-policy-agent/conftest/releases/tag/v0.71.1) benchmark is implemented and passed in [CI run 40557673362](https://github.com/tyler-beach/pf_2/actions/runs/40557673362). It verifies the checksums manifest and selected Linux release archive, then confirms the clean JSON fixture passes and the missing-key/out-of-range fixture fails in both Conftest and Pathfinder. Results are retained in the `conftest-json-policy-benchmark` artifact. This is a benchmark, not a production dependency; built-in JSON verifiers remain authoritative because the simple fixture shows parity but no replacement benefit yet.
- OpenGrep: pinned repository scanner and normalized evidence adapter are implemented; CI exercises vulnerable and clean multilingual fixtures.
- reviewdog: live same-repository PR annotations are now verified by the title-gated `[reviewdog-smoke]` regression in [CI run 40558768097](https://github.com/tyler-beach/pf_2/actions/runs/40558768097). The run confirmed an inline comment on `testdata/analyzer-fixtures/vulnerable/shell_command.go`, line 6, with the expected rule ID and message. The smoke remains opt-in by PR title and does not broaden findings into unrelated fixture files.
- ADG/ADE: not integrated; authoring/validation prototype decision pending.
- Promptfoo: offline suite integrated under `eval/promptfoo/`; live eval only in `llm-assist.yml`. Scores are not compliance verdicts.
- Runtime model calls on assess/PR gates: zero. Batch `assist-obligations` may record LLM calls on proposal artifacts only.
- Workshop: GitHub Pages SPA + `workshop-data` metrics history; see [workshop.md](workshop.md) and [kpi-dictionary.md](kpi-dictionary.md).

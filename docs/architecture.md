# ADR Compliance Engine Architecture

## Goal

Evaluate accepted Architectural Decision Records through independently addressable requirements, reusable deterministic verifiers, and auditable evidence. Normal assessment runs make zero LLM calls.

The existing engine is the foundation, not something to replace. Pathfinder evolves into an orchestration and evidence layer over established analyzers, with ADR Markdown ingestion and versioned obligation-to-pattern mappings in front of it. Scale comes from reusing a smaller pattern catalog across many ADRs, not from creating a custom verifier for every decision.

## Runtime flow

1. A GitHub Issue Form collects assessment mode, context, and source snippets.
2. The issue workflow passes the issue body as data to `adrctl`; submitted code is never executed.
3. `adr/requirements.json` is validated and accepted ADR requirements are selected.
4. Typed verifier/analyzer adapters evaluate supported, in-scope evidence.
5. The engine emits requirement-level results, an overall verdict, explicit coverage status (`COMPLETE`, `PARTIAL`, or `NONE`), Markdown, JSON evidence, and SARIF.
6. The workflow posts Markdown to the issue and retains machine-readable artifacts for 14 days.
7. Repository CI runs the same engine against tracked source and invokes configured analyzers once per repository snapshot; normalized findings are then mapped to all applicable obligations.
8. The PR workflow publishes supported SARIF findings through reviewdog.

## Existing foundations to preserve

- Go assessment engine and result model in `internal/assessment`.
- Current deterministic verifiers: `go-import-boundary`, `json-required-keys`, and `json-number-range`.
- Current `adr/requirements.json` catalog as a working compatibility path.
- Issue Markdown parsing for submitted Go/JSON snippets.
- Markdown, JSON, and SARIF report outputs.
- GitHub Actions workflows and existing Issue-based end-to-end scenarios.
- Zero runtime LLM calls.

Changes should be incremental. Keep existing verifier implementations as fallbacks and for snippet-based evidence. Replace or route a check through an external tool only after fixture-based parity demonstrates equivalent findings, location/evidence quality, and a worthwhile maintenance benefit.

## Requirement and obligation model

Each existing requirement has a stable ID, title, severity, verifier type, and verifier parameters. Preserve that contract during migration.

The target architecture separates three layers:
- **Source ADR:** immutable Markdown/version, metadata, rationale, scope, exceptions, and references.
- **Obligation:** source-cited candidate or approved interpretation with modality, applicability conditions, enforcement mode, owner, and version.
- **Verification pattern:** reusable analyzer/policy configuration and evidence contract that can serve multiple obligations.

A single ADR can contain multiple obligations; multiple ADRs can share a pattern; a qualitative ADR may be advisory or require review rather than hard pass/fail. Optional LLM-assisted extraction is batch-time only and must produce reviewable/versioned candidates. Runtime assessment remains deterministic.

See [Scaling ADR Compliance to Thousands of Decisions](scaling-adr-design.md) for the target model, open-source analyzer shortlist, migration gates, and coverage expansion.

## Current verifier behavior

- `go-import-boundary`: parses Go with the standard-library parser, scopes files by path prefix, and flags exact or child-package import matches.
- `json-required-keys`: parses a declared JSON configuration file and checks required top-level keys; missing keys produce line-addressable findings (line 1 for the document-level check).
- `json-number-range`: checks configured top-level numeric values against inclusive bounds.

## Analyzer integration strategy

Use thin, version-pinned adapters to run established tools and normalize results; do not embed large analysis frameworks into the assessment engine.

- **Go AST/parser:** continue to use the standard library as the low-dependency baseline, particularly for isolated Issue-submitted snippets.
- **Arch-Go:** benchmark for repository-wide Go package/dependency architecture rules against the current Go boundary fixtures.
- **Goldmark:** add to parse ADR Markdown into a real AST with source locations. This parser belongs to ingestion; the existing Issue snippet parser still serves its narrower input contract. Support MADR conventions without requiring every legacy ADR to be rewritten.
- **Conftest + OPA:** introduce for broader structured configuration and infrastructure policy families. Keep the small JSON checks until parity tests prove a net benefit from switching.
- **Opengrep:** primary multi-language structural/security pattern-engine candidate for an enterprise-open-source deployment; compare against ast-grep on representative rules. Keep Semgrep CE only as an optional benchmark/comparison, not an assumed default.
- **ast-grep:** alternative for tree-sitter-based syntax-pattern checks when it is simpler or has better fit for a particular pattern family.
- **Joern:** defer to requirements needing code-property-graph or deeper data-flow evidence unavailable from simpler analyzers; pilot language support and evidence quality before adoption.
- **gosec, Bandit, SpotBugs/Find Security Bugs:** add only when the relevant language and ADR coverage justifies those analyzers.
- **reviewdog:** retain as a PR annotation consumer, not as an analyzer or coverage oracle.

License is evaluated per engine, adapter, dependency, plugin, and ruleset. Pin versions/digests, preserve original tool outputs, and track an SBOM/license inventory. Do not assume a rule pack shares its engine's license. Analyzer selection and ruleset licensing must be reviewed before adoption or redistribution.

## Evidence normalization and execution

Each adapter should capture analyzer name/version, configuration hash, rule ID, completion status, scope/languages/files covered, errors/exclusions, source location, evidence excerpt/observation, original raw output reference, and evidence hash.

For a repository snapshot:
1. Discover the repository's supported source/configuration scopes.
2. Run each enabled analyzer once over its configured scope.
3. Preserve raw output and normalize findings.
4. Evaluate each obligation against the shared evidence set and its scope/exceptions.
5. Emit Markdown, JSON, and SARIF, retaining provenance and coverage information.

Findings may be reused by many obligations. Deduplicate normalized observations without dropping all obligation references. Cache results by immutable commit/tree SHA, analyzer version, configuration hash, and relevant inputs. A failed analyzer or incomplete scope is an evidence-collection failure, not a successful scan with zero findings.

## Verdicts, applicability, and coverage

Current behavior:
- `PASS`: every evaluated applicable mandatory requirement passed.
- `FAIL`: at least one requirement has reproducible violating evidence.
- `INDETERMINATE`: required evidence is absent, parsing failed, a verifier errored, or no requirement was applicable.
- Requirement-level `NOT_APPLICABLE`: current implementation uses this for an evidence item that doesn't match an explicit scope; do not extend that meaning to missing required repository files.
- Requirement-level `ERROR`: unsupported verifier type or analysis failure.

A missing file is never evidence of compliance. During the next schema iteration, make applicability independent from evidence availability and distinguish at least `EVIDENCE_MISSING`, `UNSUPPORTED`, and `REVIEW_REQUIRED` from `NOT_APPLICABLE`, `PASS`, and `FAIL`. Preserve compatibility and define merge-blocking versus warning behavior before those states enter CI policy.

The current `COMPLETE`/`PARTIAL`/`NONE` assessment coverage means only that the current catalog's requirements produced applicable PASS/FAIL results; it does not establish that every ADR in a large corpus was interpreted. Add separate corpus metrics for inventory, interpretation, approved pattern binding, evidence collection, and per-commit assessment coverage. Zero findings from an analyzer cannot prove completeness beyond its configured rules and supported scope.

Semantic preferences (for example, preferring async messaging to reduce coupling) should generally produce scoped evidence and advisory/review outcomes unless the accepted decision defines a clear mandatory invariant. Do not infer a blocking rule from intent alone.

## Security

- Issue content is untrusted input. It is written to a file through a quoted environment variable and parsed as Markdown/source text.
- Submitted code is never compiled, imported, tested, or executed.
- GitHub Actions are pinned to full commit SHAs.
- The issue assessment job has only the permissions needed to read source and comment on the issue.
- Anthropic credentials are not required or used by runtime workflows.
- Reviewdog PR comments are limited by GitHub token permissions, especially for forked pull requests.
- Analyzer binaries/rules are pinned and reviewed; rule content and repository evidence are treated as inputs, not instructions.

## Known MVP limitations

- Current built-in verifiers cover Go import boundaries and top-level JSON keys/numeric bounds; nested JSON paths and other configuration formats are not supported.
- Technical prose is not semantically inferred yet; unsupported or missing evidence remains indeterminate.
- The catalog contains example ADR requirements and should be expanded/ingested from accepted organizational ADRs.
- Issue comments are appended on each edit; a later iteration should update one bot-owned comment.
- Artifact retention is temporary and is not an audit archive.
- Issue-triggered Actions runs have validated the assessment workflow and report publishing. Reviewdog SARIF annotation behavior remains a separate PR-path validation.

## Local usage

```bash
go test ./...
go vet ./...
go run ./cmd/adrctl validate --requirements adr/requirements.json

go run ./cmd/adrctl assess \
  --input testdata/pass-issue.md \
  --requirements adr/requirements.json \
  --report /tmp/pass-report.md \
  --json-out /tmp/pass-assessment.json \
  --sarif-out /tmp/pass-results.sarif

go run ./cmd/adrctl assess \
  --input testdata/deviation-issue.md \
  --requirements adr/requirements.json \
  --report /tmp/deviation-report.md \
  --json-out /tmp/deviation-assessment.json \
  --sarif-out /tmp/deviation-results.sarif
```

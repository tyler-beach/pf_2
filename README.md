# ADR Compliance Engine

A zero-token, deterministic MVP for evaluating architectural requirements from GitHub Issues and repository source. The target architecture evolves this into a scalable evidence platform with composable analyzers and normalized outputs.

## What it does

- Models multiple independently assessable requirements per accepted ADR.
- Reuses typed deterministic verifiers for Go import boundaries and JSON keys/ranges, plus a version-pinned OpenGrep adapter that normalizes SARIF into source-scoped evidence.
- Accepts Markdown issue submissions. Language-tagged fences are sandboxed and scanned with pinned OpenGrep against the **200-ADR OWASP/AWS corpus** (`testdata/corpus/requirements.json`); submitted code is never executed. The small live catalog (`adr/requirements.json`) remains for repository CI.
- Emits a Markdown report, JSON evidence, and SARIF findings.
- Runs through GitHub Actions and posts the report to the originating issue.
- Runs repository CI and uses reviewdog to annotate SARIF findings on pull requests.
- Accepts a **PR URL** (this repo or another) via the Pathfinder PR scan issue template, then posts sparse reviewdog feedback and optional Security-tab SARIF (see [docs/pr-scan.md](docs/pr-scan.md)).
- Does not require an Anthropic API key; runtime LLM calls are zero.

## Start here

1. Open **Issues → New issue → ADR compliance assessment**.
2. Paste fenced code (python/go/javascript/…) and/or a tech spec ADR. The issue is assessed against the **full corpus** (200 OWASP Top 10 + AWS ADRs).
3. Keep the `[ADR Assessment]` title prefix.
4. The workflow sandboxes fences → OpenGrep → corpus assess, comments the report, uploads `workshop-inputs-*`, and on success **Workshop Publish** merges a snapshot into GitHub Pages.

### Scan an external PR (reviewdog + Security)

1. Open **Issues → New issue → Pathfinder PR scan (reviewdog + Security)**.
2. Paste a GitHub PR URL; check reviewdog reporters and/or **Upload SARIF to GitHub code scanning**.
3. For other repositories, configure `PATHFINDER_REVIEW_TOKEN` (and optional `PATHFINDER_SARIF_TOKEN`). Details: [docs/pr-scan.md](docs/pr-scan.md).

## Local validation

```bash
./scripts/go_test.sh
go vet $(go list ./... | grep -v '/node_modules/')
go run ./cmd/adrctl validate --requirements adr/requirements.json
```


Compliant fixture:

```bash
go run ./cmd/adrctl assess --input testdata/pass-issue.md --requirements adr/requirements.json --report /tmp/pass.md --json-out /tmp/pass.json --sarif-out /tmp/pass.sarif
```

Deviation fixture:

```bash
go run ./cmd/adrctl assess --input testdata/deviation-issue.md --requirements adr/requirements.json --report /tmp/deviation.md --json-out /tmp/deviation.json --sarif-out /tmp/deviation.sarif
```

Expected results are `PASS` for the compliant fixture and `FAIL` with two requirement-level deviations for the deviation fixture. The assessment command exits successfully when a report is produced; the report verdict is distinct from workflow execution status.

## Live GitHub Issue validation runs

These issues exercise the actual Issue-triggered workflow and include its generated assessment report:

- [#5 — JSON required-keys PASS](https://github.com/tyler-beach/pf_2/issues/5)
- [#6 — JSON missing-key deviation](https://github.com/tyler-beach/pf_2/issues/6)
- [#7 — malformed JSON returns INDETERMINATE](https://github.com/tyler-beach/pf_2/issues/7)
- [#8 — post-hardening JSON verifier regression](https://github.com/tyler-beach/pf_2/issues/8)
- [#9 — mixed Go + JSON PASS](https://github.com/tyler-beach/pf_2/issues/9)
- [#10 — mixed Go + JSON deviations](https://github.com/tyler-beach/pf_2/issues/10)
- [#12 — numeric range inclusive upper-bound PASS](https://github.com/tyler-beach/pf_2/issues/12)
- [#13 — timeout above maximum FAIL](https://github.com/tyler-beach/pf_2/issues/13)
- [#14 — numeric range inclusive lower-bound PASS](https://github.com/tyler-beach/pf_2/issues/14)
- [#15 — numeric verifier rejects string values](https://github.com/tyler-beach/pf_2/issues/15)
- [#16 — partial coverage is explicitly reported](https://github.com/tyler-beach/pf_2/issues/16)
- [#17 — complete mixed-language coverage](https://github.com/tyler-beach/pf_2/issues/17)

The mixed cases verify that multiple requirements across different verifier types are evaluated independently in one report. Issues #12–#15 exercise inclusive lower/upper boundaries, above-maximum values, and non-numeric values. Issues #16–#17 verify that JSON-only evidence reports PARTIAL coverage while mixed Go and JSON evidence reports COMPLETE coverage. Implementation work is tracked in [#11](https://github.com/tyler-beach/pf_2/issues/11).

## Specification-to-enforcement milestone

The staged contract for translating plain-English technical specifications into reviewed obligations, typed verifier bindings, and auditable evidence is documented in [Specification-to-enforcement milestone](docs/specification-to-enforcement.md). The representative [service runtime specification fixture](testdata/specifications/service-runtime.md) is a target-workflow fixture, not a claim that arbitrary prose is already automatically enforced.

## Workshop (GitHub Pages)

End-to-end operational console and KPI trends for Pathfinder runs (extract → confidence → assist → assess → Promptfoo). Snapshots are emitted by `adrctl workshop-emit`, stored on the `workshop-data` branch, and published by `.github/workflows/workshop-publish.yml`.

- [Workshop architecture + Pages bootstrap](docs/workshop.md)
- [KPI dictionary](docs/kpi-dictionary.md)
- Local UI: `cd workshop && npm ci && npm run dev` (seeded fixtures under `workshop/public/data`)
- First deploy: enable **Settings → Pages → Source: GitHub Actions**, then run CI or manually dispatch **Workshop Publish**

Batch LLM assist is confidence-gated (`adrctl assist-obligations`); default CI uses the offline provider. Promptfoo scores are **not** compliance verdicts.

## Architecture and limitations

- [Current architecture overview](docs/architecture.md)
- [Scalable architecture: ADR ingestion, normalized evidence, composite analyzers, and AWS deployment](docs/scalable-architecture.md)

The catalog is an example and should be replaced/expanded with accepted ADRs. The current implementation supports `go-import-boundary`, `json-required-keys`, `json-number-range`, and the `analyzer-rule` verifier backed by normalized OpenGrep SARIF. The CI fixture corpus currently covers 20 language-specific rule families; this is rule-family fixture coverage, not blanket semantic support for every language. See [language support and coverage](docs/language-support.md) for the exact list and enforcement boundaries. Issue-snippet assessments intentionally do not execute external analyzers; repository analyzer requirements are marked not applicable in that mode, with partial coverage visible. Each report includes `coverage_status` (`COMPLETE`, `PARTIAL`, or `NONE`) and the number of requirements with applicable evidence. A `PASS` verdict is scoped to evaluated requirements; it must not be treated as a complete assessment when coverage is `PARTIAL` or `NONE`. Unsupported evidence produces `INDETERMINATE` or a requirement-level error, not a pass.

The scalable design proposes composable analyzers such as Opengrep, Joern, and language-specific tools behind one versioned evidence contract. These are design candidates, not implemented integrations in the current MVP.


## Scaling toward a large ADR corpus

Pathfinder is being evolved incrementally: the existing Go assessment engine, deterministic Go/JSON verifiers, Issue workflow, and Markdown/JSON/SARIF outputs remain the foundation. The scale strategy adds Goldmark-based ADR ingestion, reviewed/versioned obligation records, reusable verification patterns, and thin adapters over established analyzers. Each enabled analyzer runs once per repository snapshot and its findings are mapped to applicable obligations.

The current `adr/requirements.json` remains supported while this ingestion path matures. Runtime assessment continues to make zero LLM calls; any optional assisted interpretation belongs in a separate batch-time workflow and requires review before enforcement.

See [docs/scaling-adr-design.md](docs/scaling-adr-design.md) for the migration plan and analyzer choices, and [docs/architecture.md](docs/architecture.md) for the current runtime architecture.


## ADR Markdown ingestion (initial implementation)

Pathfinder can build a deterministic source index for ADR Markdown using Goldmark:

```sh
go run ./cmd/adrctl ingest-adrs --root adrs --json-out adr/ingested-adrs.json
```

The input directory should contain ADR documents (MADR-style or legacy Markdown), each with a level-one title. The generated index records a stable ID (an `ADR-###` identifier from the title when present, otherwise the filename), status when present, section text, 1-based heading line ranges, normalized repository-relative source paths, and a SHA-256 of each original file. Documents are processed in lexical path order; duplicate IDs and malformed Markdown fail the command rather than producing a partial index.

**Important:** this is source ingestion, not automatic approval or obligation generation. The generated index does not overwrite `adr/requirements.json`, and no natural-language statement becomes an enforceable compliance rule without an explicit reviewed requirement/verifier mapping. Runtime assessment remains deterministic and makes zero LLM calls.

For a runnable first pass, initialize the review registry once (skip this command if the registry already exists; initialization never overwrites review history), then run against the checked-in plain-English technical specification samples in report-only mode:

```sh
# One-time setup in a fresh checkout
go run ./cmd/adrctl review-init --output adr/obligation-reviews.json

# Repeatable report-only run against the sample corpus
go run ./cmd/adrctl specification-check \
  --root . \
  --source-root testdata/specifications \
  --requirements adr/requirements.json \
  --registry adr/obligation-reviews.json \
  --report-only
```

For a real repository, set `--source-root` to its ADR or plain-English technical-specification Markdown directory. A source file needs a level-one Markdown title; if the title has no ADR ID, Pathfinder uses the filename as its stable document ID. Blocking bindings also require the source document to declare `Accepted` status and the matching catalog entry to be accepted. The command regenerates the source and candidate indexes, assesses the repository, joins evidence to current reviews, and enforces blocking bindings by default; use `--report-only` while bootstrapping or reviewing pending candidates. For `analyzer-rule` requirements, pass both `--analyzer-evidence` and `--analyzer-sarif` from a verified `scan-opengrep` run. The orchestrator never creates or approves reviews automatically. See [the full workflow contract](docs/specification-to-enforcement.md).

## Validated analyzer language coverage

The repository-scan adapter has rule-level, vulnerable/clean fixture coverage for Go, Python, JavaScript, TypeScript, Java, C#, PHP, Ruby, Bash, C, C++, Rust, Kotlin, Scala, Elixir, Dart, Swift, Lua, Visual Basic, and OCaml. The fixtures assert that the expected rule families match intentionally vulnerable examples and that the corresponding clean examples produce no findings.

See [docs/language-support.md](docs/language-support.md) for the explicit rule/extension matrix and limitations. The [tool adoption decisions](docs/tool-adoption-decisions.md) document the Arch-Go benchmark and the adoption gates for ADG/ADE, reviewdog, and Promptfoo. This is tested rule-family coverage, not a claim that every possible vulnerability or every language feature is covered.

## Multi-language analyzer (implemented)

Repository CI downloads OpenGrep v1.30.0 from the upstream release and verifies the binary SHA-256 before execution. Checked-in rules in `rules/pathfinder-security.yml` (merged from `rules/pathfinder-security-core.yml` plus OWASP/AWS packs via `scripts/merge_opengrep_rules.py`) are exercised against vulnerable and clean fixtures. The CI gate requires the legacy shell/eval families plus key OWASP/AWS rules to produce findings in vulnerable fixtures and no findings for clean fixtures. The labeled MADR corpus under `testdata/corpus` covers 200 OWASP Top 10 + AWS security ADRs (`python3 scripts/generate_security_corpus.py`).

Scan a repository and retain both raw and normalized evidence:

```sh
go run ./cmd/adrctl scan-opengrep \
  --root . \
  --config rules/pathfinder-security.yml \
  --binary /path/to/pinned/opengrep \
  --version 1.30.0 \
  --cache-dir "$HOME/.cache/pathfinder/analyzer-cache" \
  --manifest-out analyzer-evidence/run-manifest.json \
  --json-out analyzer-evidence/opengrep.json \
  --sarif-out analyzer-evidence/opengrep.sarif
```

Use normalized evidence when evaluating the repository:

```sh
go run ./cmd/adrctl assess-repo \
  --root . \
  --requirements adr/requirements.json \
  --analyzer-evidence analyzer-evidence/opengrep.json \
  --analyzer-sarif analyzer-evidence/opengrep.sarif \
  --report report.md \
  --json-out assessment.json \
  --sarif-out results.sarif
```

Before findings are used, Pathfinder re-parses the paired raw SARIF and verifies its SHA-256, the recorded configuration file's SHA-256, and the current repository-tree digest. Evidence from another source snapshot or changed rules is rejected rather than silently reused. These digests provide integrity and freshness checks, not a digital signature. Analyzer-rule obligations require a complete scan, input/configuration/raw-output SHA-256 provenance, and at least one discovered path in the declared scope. Missing or partial scan evidence is `EVIDENCE_MISSING` and makes the verdict `INDETERMINATE`; a finding without a source path also fails closed. Findings outside an obligation's declared path prefixes do not violate that obligation.

## Issue-submission language coverage

Issue submissions retain common source and configuration snippets with explicit language labels, including Go, JSON, Python, JavaScript/TypeScript, Java, C#, PHP, Ruby, Rust, shell, YAML, Terraform/HCL, Kotlin, Scala, C/C++, SQL, HTML/XML, and Vue. Only Go and JSON have direct deterministic snippet verifiers today. Other submitted languages are listed under `unassessed_source_files`; they force a partial-coverage `INDETERMINATE` result instead of being ignored or implicitly passing. Issue workflows never execute submitted code or launch external analyzers against issue-provided snippets.

Assessments also record `submission_kind`: `tech_spec` (ADR / English prose) vs `code_snippet` (source or IaC). Pass `--submission-kind` to `assess` / `assess-repo`, or let auto-detect infer fence-only issues as `code_snippet`, ADR-like prose as `tech_spec`, and repository trees as `code_snippet`. Workshop KPIs trend these kinds separately (`kpi.assess.submission_kind`, `kpi.submission.*_count`).

### Repository rule bindings

CI proves **20 OpenGrep rule families** with vulnerable/clean fixtures (see [language support](docs/language-support.md)). The accepted example catalog in `adr/requirements.json` binds those fixture-proven families to enforceable `analyzer-rule` requirements (ADR-003 through ADR-006), each with explicit path/extension scope. Fixture coverage and catalog enforcement are kept aligned for those families; a completed, integrity-verified inventory with no matching extension/scope is explicitly `NOT_APPLICABLE`; a missing or incomplete inventory remains `EVIDENCE_MISSING` and cannot pass. Reusable pattern definitions live in [`adr/patterns.json`](adr/patterns.json).

### Analyzer fixture isolation

The intentionally vulnerable samples under `testdata/analyzer-fixtures/` are scanned separately by the CI fixture test. The repository compliance scan excludes that directory from both the OpenGrep invocation and the verified scanned-path inventory. This keeps known-positive test fixtures from being mistaken for production-source violations while still requiring the fixture job to detect them. No other test or repository directory is excluded by this fixture-specific rule.

## Review-only ADR obligation candidates

After ingesting ADR Markdown, Pathfinder can emit deterministic candidate obligations with exact source line and SHA-256 provenance:

~~~sh
go run ./cmd/adrctl ingest-adrs --root adrs --json-out adr/ingested-adrs.json
go run ./cmd/adrctl extract-obligations --index adr/ingested-adrs.json --json-out adr/obligation-candidates.json
~~~

The extractor uses explicit normative cues such as must, shall, should, may, prohibited, or required and records a conservative reusable-pattern hint. Every candidate remains pending review, uses review-required enforcement, and sets executable to false. The index is deterministic and includes the source-index digest, source SHA-256, repository-relative path, exact source line, section heading, modality, candidate kind, and exception candidates. It rejects stale source indexes, duplicate ADR IDs, unsafe paths, symlinked source paths, and oversized source files.

This is candidate discovery, not natural-language policy enforcement. It does not synthesize or modify adr/requirements.json, call an LLM, approve any obligation, or make a statement enforceable. An ADR owner must review the source, modality, scope, exceptions, pattern mapping, and regression fixtures before a requirement can gate CI.


## Issue-driven regression assertions

The issue workflow can enforce a regression's expected overall verdict by including exactly one marker in the issue title:
- `[expected:PASS]` for a compliant submitted scope (which may still have `PARTIAL` coverage).
- `[expected:FAIL]` for a demonstrated violation.
- `[expected:INDETERMINATE]` when required evidence is missing or a submitted language is not assessed.

The assessment comment and JSON/SARIF artifacts are published before the assertion runs, so a failed regression preserves evidence for diagnosis. Issues without a marker remain ordinary assessments and do not have their verdict asserted.


## Human review registry for ADR obligation candidates

Candidate extraction is intentionally separate from approval and enforcement. A reviewer decision is stored in an append-only, versioned registry with a source snapshot, source SHA-256, exact line range, reviewer identity, RFC3339 timestamp, HTTPS review reference, rationale, and an incrementing revision.

Initialize a new ledger once; the command refuses to overwrite an existing one:

~~~sh
go run ./cmd/adrctl review-init --output adr/obligation-reviews.json
~~~

Create a proposal JSON file using snake_case fields such as candidate_id, decision, requirement_id, pattern_id, enforcement_mode, reviewer, reviewed_at, review_reference, and rationale. Then append and validate the decision:

~~~sh
go run ./cmd/adrctl record-obligation-review \
  --index adr/obligation-candidates.json \
  --requirements adr/requirements.json \
  --registry adr/obligation-reviews.json \
  --proposal review-proposal.json \
  --coverage-out adr/obligation-review-coverage.json

go run ./cmd/adrctl validate-obligation-reviews \
  --index adr/obligation-candidates.json \
  --requirements adr/requirements.json \
  --registry adr/obligation-reviews.json \
  --json-out adr/obligation-review-coverage.json \
  --strict
~~~

Pending, rejected, and superseded decisions cannot bind a requirement or claim enforcement. Blocking/advisory approval requires a current source snapshot, an accepted ADR, an existing requirement owned by the same ADR, and a compatible reviewed verifier pattern. Semantic/manual-review candidates cannot block CI. A changed candidate source is stale until re-reviewed; a removed candidate's history must be explicitly superseded. The registry never edits adr/requirements.json automatically. See [docs/obligation-review.md](docs/obligation-review.md) for the decision lifecycle and limitations.


### Deterministic plan, manifest, and local cache

Every repository OpenGrep scan is executed as a validated single-task plan. The plan digest and cache key include the source snapshot, exact executable SHA-256, version, configuration/rule-set SHA-256, repository-relative path scope, source extensions, timeout, and declared resource envelope. The manifest ID is a stable idempotency key for duplicate assessments of the same snapshot and plan; it excludes wall-clock timestamps and attempt counters.

The scan CLI writes a manifest to `analyzer-evidence/run-manifest.json` by default. A private file cache stores normalized evidence and paired raw SARIF under SHA-256 filenames, uses atomic replacement and restricted permissions, rejects symlink/corrupt cache entries, and treats cache failures as warnings while still running the analyzer. Cache hits are accepted only when source/configuration/executable/rule-set identities and raw-output digest match the planned task. CI runs the same plan twice and asserts the second run uses the identical cached result.

Timeouts and per-file bounds are enforced by the local OpenGrep adapter. `max-memory-mb` is an explicit worker resource profile in the plan identity; remote orchestrators must enforce it with their container/task runtime. The standalone local CLI does not claim to impose an OS-level memory limit. Runtime assessment continues to make zero LLM calls.

## Deterministic analyzer plans

Create a versioned plan manifest without executing an analyzer:

```sh
go run ./cmd/adrctl plan-repo \
  --root . \
  --analyzer opengrep \
  --version 1.30.0 \
  --config rules/pathfinder-security.yml \
  --binary-sha256 35779bdd72e92129c8df2a77f0c55e8c08356801ea92591ef32108d6b28d564c \
  --scope . \
  --timeout 5m \
  --max-retries 1 \
  --max-concurrency 2 \
  --json-out analyzer-evidence/plan.json
```

Verify that the manifest still matches the current source/configuration snapshot before using its task keys:

```sh
go run ./cmd/adrctl verify-plan --root . --manifest analyzer-evidence/plan.json
```

The schema records the global input-tree digest, sorted per-file inventory and hashes, analyzer/config identity, task scope, cache key, idempotency key, attempt/timeout/output limits, and a global concurrency cap. Cache keys change when source content, configuration, analyzer version, or scoped inventory changes. Planning and verification are deterministic; neither command runs the scanner. An empty declared scope is recorded explicitly as `EMPTY`, never silently treated as proof of compliance. The manifest is an execution contract; it is not itself a scan result.

Execute the verified plan locally with bounded concurrency/retries, source-bound evidence validation, raw SARIF retention, and a private file cache. Keep the cache outside the repository so it cannot change the input snapshot during the run:

```sh
go run ./cmd/adrctl execute-plan \
  --root . \
  --manifest analyzer-evidence/plan.json \
  --cache-dir "$HOME/.cache/pathfinder/analyzers" \
  --json-out analyzer-evidence/execution.json \
  --sarif-dir analyzer-evidence/raw
```

The execution adapter currently supports OpenGrep. Other analyzer names are rejected as unsupported until their adapters are implemented; a planned analyzer is not assumed to have run merely because the plan was created. Only complete results whose source/configuration digests and scanned-file inventory match the plan can be cached and reused. See [docs/analyzer-plan-manifest.md](docs/analyzer-plan-manifest.md) for the execution contract and failure semantics.


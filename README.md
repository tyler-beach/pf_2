# ADR Compliance Engine

A zero-token, deterministic MVP for evaluating architectural requirements from GitHub Issues and repository source. The target architecture evolves this into a scalable evidence platform with composable analyzers and normalized outputs.

## What it does

- Models multiple independently assessable requirements per accepted ADR.
- Reuses typed deterministic verifiers for Go import boundaries and JSON keys/ranges, plus a version-pinned OpenGrep adapter that normalizes SARIF into source-scoped evidence.
- Accepts Markdown issue submissions and parses Go/JSON code fences without executing submitted code. Repository-level OpenGrep scans run in a separate CI stage; analyzer binaries are not run on issue-submitted snippets.
- Emits a Markdown report, JSON evidence, and SARIF findings.
- Runs through GitHub Actions and posts the report to the originating issue.
- Runs repository CI and uses reviewdog to annotate SARIF findings on pull requests.
- Does not require an Anthropic API key; runtime LLM calls are zero.

## Start here

1. Open **Issues → New issue → ADR compliance assessment**.
2. Include source snippets with a heading such as `### File: internal/domain/customer.go` followed by a fenced Go code block.
3. Create the issue with the default `[ADR Assessment]` title prefix.
4. The ADR Assessment workflow runs tests, evaluates supported requirements, comments the report, and uploads machine-readable evidence.

## Local validation

```bash
go test ./...
go vet ./...
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

## Architecture and limitations

- [Current architecture overview](docs/architecture.md)
- [Scalable architecture: ADR ingestion, normalized evidence, composite analyzers, and AWS deployment](docs/scalable-architecture.md)

The catalog is an example and should be replaced/expanded with accepted ADRs. The current implementation supports `go-import-boundary`, `json-required-keys`, `json-number-range`, and the `analyzer-rule` verifier backed by normalized OpenGrep SARIF. OpenGrep rules and language coverage are pinned and fixture-tested for Go, Python, JavaScript, TypeScript, Java, C#, PHP, and Ruby. Issue-snippet assessments intentionally do not execute external analyzers; repository analyzer requirements are marked not applicable in that mode, with partial coverage visible. Each report includes `coverage_status` (`COMPLETE`, `PARTIAL`, or `NONE`) and the number of requirements with applicable evidence. A `PASS` verdict is scoped to evaluated requirements; it must not be treated as a complete assessment when coverage is `PARTIAL` or `NONE`. Unsupported evidence produces `INDETERMINATE` or a requirement-level error, not a pass.

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

## Multi-language analyzer (implemented)

Repository CI downloads OpenGrep v1.30.0 from the upstream release and verifies the binary SHA-256 before execution. Checked-in rules in `rules/pathfinder-security.yml` are exercised against vulnerable and clean fixtures in Go, Python, JavaScript, TypeScript, Java, C#, PHP, and Ruby. The CI gate requires the expected rule families to produce findings in vulnerable fixtures and no findings for clean fixtures.

Scan a repository and retain both raw and normalized evidence:

```sh
go run ./cmd/adrctl scan-opengrep \
  --root . \
  --config rules/pathfinder-security.yml \
  --binary /path/to/pinned/opengrep \
  --version 1.30.0 \
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


### Repository rule bindings

The accepted example catalog maps each OpenGrep rule family to a reusable requirement. Analyzer requirements may declare `source_extensions` in addition to repository path prefixes. A completed, integrity-verified inventory with no matching extension/scope is explicitly `NOT_APPLICABLE`; a missing or incomplete inventory remains `EVIDENCE_MISSING` and cannot pass. The current sample bindings cover Go shell execution, dynamic evaluation in JavaScript/TypeScript/PHP/Python/Ruby, and process invocation in Java/C#.

### Analyzer fixture isolation

The intentionally vulnerable samples under `testdata/analyzer-fixtures/` are scanned separately by the CI fixture test. The repository compliance scan excludes that directory from both the OpenGrep invocation and the verified scanned-path inventory. This keeps known-positive test fixtures from being mistaken for production-source violations while still requiring the fixture job to detect them. No other test or repository directory is excluded by this fixture-specific rule.

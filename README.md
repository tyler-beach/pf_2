# ADR Compliance Engine

A zero-token, deterministic MVP for evaluating architectural requirements from GitHub Issues and repository source. The target architecture evolves this into a scalable evidence platform with composable analyzers and normalized outputs.

## What it does

- Models multiple independently assessable requirements per accepted ADR.
- Reuses typed deterministic verifiers; current implementations check Go import boundaries with the Go standard library parser and required top-level keys and numeric ranges in JSON configuration.
- Accepts Markdown issue submissions and parses Go code fences without executing submitted code.
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

The catalog is an example and should be replaced/expanded with accepted ADRs. The MVP supports `go-import-boundary`, `json-required-keys`, and `json-number-range`. Each report includes `coverage_status` (`COMPLETE`, `PARTIAL`, or `NONE`) and the number of requirements with applicable evidence. A `PASS` verdict is scoped to evaluated requirements; it must not be treated as a complete assessment when coverage is `PARTIAL` or `NONE`. Unsupported evidence produces `INDETERMINATE` or a requirement-level error, not a pass.

The scalable design proposes composable analyzers such as Opengrep, Joern, and language-specific tools behind one versioned evidence contract. These are design candidates, not implemented integrations in the current MVP.


## Scaling toward a large ADR corpus

Pathfinder is being evolved incrementally: the existing Go assessment engine, deterministic Go/JSON verifiers, Issue workflow, and Markdown/JSON/SARIF outputs remain the foundation. The scale strategy adds Goldmark-based ADR ingestion, reviewed/versioned obligation records, reusable verification patterns, and thin adapters over established analyzers. Each enabled analyzer runs once per repository snapshot and its findings are mapped to applicable obligations.

The current `adr/requirements.json` remains supported while this ingestion path matures. Runtime assessment continues to make zero LLM calls; any optional assisted interpretation belongs in a separate batch-time workflow and requires review before enforcement.

See [docs/scaling-adr-design.md](docs/scaling-adr-design.md) for the migration plan and analyzer choices, and [docs/architecture.md](docs/architecture.md) for the current runtime architecture.

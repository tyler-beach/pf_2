# ADR Compliance Engine

A zero-token, deterministic MVP for evaluating architectural requirements from GitHub Issues and repository source.

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

The mixed cases verify that multiple requirements across different verifier types are evaluated independently in one report. Issues #12 and #13 exercise the numeric verifier at the inclusive upper boundary and above the maximum. Implementation work is tracked in [#11](https://github.com/tyler-beach/pf_2/issues/11).

## Architecture and limitations

See [docs/architecture.md](docs/architecture.md). The catalog is an example and should be replaced/expanded with your accepted ADRs. The MVP supports `go-import-boundary`, `json-required-keys`, and `json-number-range`. Unsupported evidence produces `INDETERMINATE` or a requirement-level error, not a pass.

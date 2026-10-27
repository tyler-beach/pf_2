# ADR Compliance Engine

A zero-token, deterministic MVP for evaluating architectural requirements from GitHub Issues and repository source.

## What it does

- Models multiple independently assessable requirements per accepted ADR.
- Reuses typed deterministic verifiers; current implementation checks Go import boundaries with the Go standard library parser.
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

## Architecture and limitations

See [docs/architecture.md](docs/architecture.md). The catalog is an example and should be replaced/expanded with your accepted ADRs. The MVP supports Go import-boundary rules only. Unsupported evidence produces `INDETERMINATE`, not a pass.

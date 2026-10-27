# ADR Compliance Engine Architecture

## Goal

Evaluate accepted Architectural Decision Records through independently addressable requirements, reusable deterministic verifiers, and auditable evidence. Normal assessment runs make zero LLM calls.

## Runtime flow

1. A GitHub Issue Form collects assessment mode, context, and source snippets.
2. The issue workflow passes the issue body as data to `adrctl`; submitted code is never executed.
3. `adr/requirements.json` is validated and accepted ADR requirements are selected.
4. Typed verifier adapters evaluate supported, in-scope evidence.
5. The engine emits requirement-level results, an overall verdict, explicit coverage status (`COMPLETE`, `PARTIAL`, or `NONE`), Markdown, JSON evidence, and SARIF.
6. The workflow posts Markdown to the issue and retains machine-readable artifacts for 14 days.
7. Repository CI runs the same engine against tracked Go source and uses reviewdog to annotate PR findings.

## Requirement contract

Each requirement has a stable ID, title, severity, verifier type, and verifier parameters. ADR version is included in each result. Add requirements by extending the catalog when an existing verifier is sufficient; add Go code only for a genuinely new verifier type.

Current verifier:
- `go-import-boundary`: parses Go with the standard-library Go parser, scopes files by path prefix, and flags exact or child-package import matches.\n- `json-required-keys`: parses a declared JSON configuration file and checks that required top-level keys are present; missing keys produce line-addressable findings (line 1 for the document-level check).

Verdicts:
- `PASS`: every applicable mandatory requirement passed.
- `FAIL`: at least one requirement has reproducible violating evidence.
- `INDETERMINATE`: required evidence is absent, parsing failed, a verifier errored, or no requirement was applicable.
- Requirement-level `NOT_APPLICABLE`: no submitted/discovered file matched the requirement's explicit scope.
- Requirement-level `ERROR`: unsupported verifier type or analysis failure.

A missing file is never evidence of compliance. `PASS` describes only requirements with applicable evidence; coverage is tracked separately. `COMPLETE` means every accepted requirement produced a PASS or FAIL result, `PARTIAL` means only some did, and `NONE` means no requirement produced an applicable PASS/FAIL result. Results include source-file count, per-requirement status, evidence, findings, ADR version, coverage counts, and a runtime LLM call count of zero.

## Open-source integration strategy

- Go standard library AST/parser: baseline Go syntax and import evidence without a third-party runtime dependency.
- ast-grep: candidate adapter for multi-language structural checks after a benchmark demonstrates a need.
- Semgrep CE: alternative structural matcher to benchmark against ast-grep; do not run both by default.
- OPA/Conftest: add when requirements concern structured configuration or policy documents.
- Arch-Go: evaluate for richer Go architecture rules that exceed the initial import-boundary verifier.
- reviewdog: consumes SARIF from repository assessments to publish PR annotations.
- SCIP or Joern: defer until an ADR requires cross-file symbol resolution or data-flow evidence that local AST checks cannot establish.

No analyzer is treated as proof of completeness merely because it emits no findings. Each adapter must report coverage, tool version, errors, and evidence provenance.

## Security

- Issue content is untrusted input. It is written to a file through a quoted environment variable and parsed as Markdown/source text.
- Submitted code is never compiled, imported, tested, or executed.
- GitHub Actions are pinned to full commit SHAs.
- The issue assessment job has only the permissions needed to read source and comment on the issue.
- Anthropic credentials are not required or used by runtime workflows.
- Reviewdog PR comments are limited by GitHub token permissions, especially for forked pull requests.

## Known MVP limitations

- Verifiers currently cover Go import boundaries and top-level JSON keys/numeric bounds; nested JSON paths and other configuration formats are not supported.
- Technical prose is not semantically inferred; unsupported or missing evidence remains indeterminate.
- The catalog contains example ADR requirements and should be expanded with accepted organizational ADRs.
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

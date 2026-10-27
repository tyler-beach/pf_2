# ADR Compliance Engine Architecture

## Goal

Evaluate accepted Architectural Decision Records through independently addressable obligations, reusable verification patterns, composable analyzers, and auditable evidence. The current Go MVP is deterministic and makes zero runtime LLM calls. The long-term design supports thousands of ADRs and AWS-hosted analyzer orchestration without requiring a custom verifier per ADR.

## Runtime flow

1. Discover accepted ADRs and repository artifacts at a pinned revision.
2. Parse ADR Markdown and metadata, preserve provenance, and map reviewed obligations to reusable verification patterns.
3. Plan the relevant analyzers based on artifact types, active patterns, changed files, and cached evidence.
4. Run one or more composite analyzers. Candidates include native language AST parsers, Opengrep, Joern for graph/data-flow use cases, gosec, Bandit, SpotBugs/Find Security Bugs, configuration/IaC parsers, OpenAPI parsers, and policy tools where justified.
5. Normalize all tool outputs to the canonical evidence contract in [scalable-architecture.md](scalable-architecture.md). Keep raw output, producer/tool/config versions, source provenance, declared scope, diagnostics, and limitations.
6. Evaluate approved obligation patterns against normalized evidence.
7. Emit requirement-level results, verdicts, coverage, JSON evidence, Markdown, and SARIF; publish to GitHub checks/issues or a future API.

The current Issue-triggered GitHub Actions workflow and CLI remain the MVP execution surfaces. AWS is a future deployment target, not a prerequisite for the core contracts.

## Requirement and obligation model

Separate three concepts:

- **ADR record:** original decision, rationale, owner, scope, exceptions, status, and version.
- **Obligation:** an independently assessable statement derived from an ADR, including modality, scope, evidence requirements, confidence, provenance, and review status.
- **Verification pattern:** reusable logic such as import boundaries, required configuration, dependency restrictions, API compatibility, gateway routing, or architecture-graph constraints.

One ADR may produce several obligations; many obligations may reuse one pattern; one obligation may consume evidence from several analyzers. Candidate extraction and clustering can be assisted in batches, but interpretations must be reviewable and versioned. Semantic principles such as “prefer asynchronous messaging to reduce coupling” must not automatically become hard failures without an approved scope and observable verification proxy.

## Current MVP verifiers

- `go-import-boundary`: Go standard-library parser checks import boundaries.
- `json-required-keys`: checks required top-level JSON keys.
- `json-number-range`: checks numeric top-level JSON values against inclusive bounds.

Add catalog entries when an existing verifier/pattern suffices. Add a new verifier only when the capability is genuinely missing.

## Normalized evidence and outcomes

Analyzer adapters emit a common, versioned evidence schema with repository revision, artifact digest/path/range, producer and tool versions, rule/config digest, typed observation or finding, provenance, declared scope, status, diagnostics, and limitations. Raw outputs remain available for audit and reprocessing.

Outcomes must distinguish `PASS`, `FAIL`, `NOT_APPLICABLE`, `EVIDENCE_MISSING`, `UNSUPPORTED`, `REVIEW_REQUIRED`, `INDETERMINATE`, and `ERROR`. The current MVP's `COMPLETE`/`PARTIAL`/`NONE` coverage remains useful but should be complemented by ADR-corpus interpretation coverage, obligation mapping coverage, analyzer scope coverage, and evidence freshness. No findings is not equivalent to complete assessment.

## Open-source and composite analyzer strategy

- Native Go parser and other language-native parsers: fast, precise syntax and dependency facts.
- **Opengrep:** multi-language structural/pattern checks; evaluate compatibility and rule quality. Review engine and rule licenses separately.
- **Joern:** code property graph and deep cross-file/data-flow queries when local AST evidence is insufficient.
- **gosec**, **Bandit**, **SpotBugs + Find Security Bugs**: language-specific security evidence for applicable repositories.
- **OPA/Conftest**, IaC/configuration parsers, OpenAPI and dependency-manifest tools: select based on the ADR corpus and artifact types.
- **reviewdog:** publishes SARIF-derived annotations on pull requests.

These are composable candidates, not a mandate to run all tools. Avoid redundant scanners by default; benchmark precision, recall, coverage, runtime, memory, operational complexity, and licensing. Audit the analyzer engine, rule packs, plugins, and transitive dependencies independently. Every adapter must report scope, tool/config version, errors, and provenance. A scanner finding is evidence to evaluate, not automatically an ADR violation.

## AWS deployment direction

Use EventBridge for events and routing, Step Functions for orchestration, Lambda for short control-plane tasks and lightweight parsing, and ECS/Fargate or AWS Batch for isolated heavyweight analyzer containers. Use S3 for immutable raw outputs, normalized evidence bundles, and reports; use DynamoDB or Aurora for versioned ADRs, obligations, pattern mappings, run metadata, approvals, and evidence pointers. Choose storage based on access/query requirements rather than assuming a vector database is required. Optional semantic retrieval may suggest related ADRs and patterns but cannot establish compliance.

Workers should be least-privilege, isolated, resource-bounded, pinned by image digest, and denied unnecessary network access. Use idempotency keys, content hashes, incremental analysis, bounded concurrency, controlled retries, lifecycle policies, and metrics. Treat repository content and tool output as untrusted. See [the full scalable design](scalable-architecture.md) for contracts, AWS boundaries, rollout phases, and acceptance criteria.

## Security

- Issue content and repository source are untrusted input.
- Submitted snippets are parsed as data; they are not executed.
- Analyzer jobs run with least privilege and no production credentials.
- Any analyzer that requires builds or execution uses an explicit isolated sandbox.
- Pin GitHub Actions, analyzer images, tool versions, and rule/config revisions.
- Keep secrets out of evidence/logs; apply access controls and retention to raw artifacts.
- Runtime LLM credentials are not required by the current MVP. Any future batch-assisted ADR interpretation is separate from deterministic assessment and requires review/approval.

## Known MVP limitations

- Current deterministic checks cover Go import boundaries and top-level JSON keys/numeric bounds.
- Technical prose is not semantically inferred by the runtime.
- The catalog is illustrative and does not yet represent a full organizational ADR corpus.
- Coverage currently reflects catalog requirements evaluated by the MVP; corpus-level interpretation and mapping coverage remain future work.
- Issue comments may append on each edit; a later iteration should update one bot-owned comment.
- Workflow artifact retention is temporary and is not an audit archive.
- Reviewdog SARIF annotation behavior remains a separate PR-path validation.

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

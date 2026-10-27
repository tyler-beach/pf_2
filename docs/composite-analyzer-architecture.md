# Composite Analyzer Architecture

**Status:** proposed target architecture; the current Go CLI remains the working MVP. This document describes the path from a deterministic, single-process assessment to a scalable AWS-hosted composite analyzer platform.

## Design goals

1. Support thousands of ADRs without creating one bespoke scanner per ADR.
2. Reuse mature open-source analyzers and write adapters, not replacements for those analyzers.
3. Normalize every analyzer's output into one versioned Pathfinder evidence contract.
4. Keep an analyzer's silence distinct from proof of compliance; track applicability, coverage, failures, and provenance explicitly.
5. Keep runtime assessment deterministic and free of LLM dependencies. ADR interpretation assistance may be an offline, reviewable authoring workflow, not a hidden runtime verdict dependency.
6. Run analyzers in isolated, reproducible jobs that can scale independently in AWS.

## Core model: obligations are not analyzers

The architecture separates four concepts:

- **ADR:** the accepted architectural decision and its context, rationale, scope, exceptions, owner, version, and lifecycle.
- **Obligation:** a specific, reviewable statement derived from an ADR, with its own stable ID, applicability conditions, strength, evidence needs, and acceptance criteria.
- **Verification pattern:** a reusable way to evaluate a class of obligations. One pattern can serve many obligations across many ADRs.
- **Analyzer:** an external or built-in tool that produces code, configuration, dependency, or architecture evidence. An analyzer is not itself an ADR verdict.

A corpus of 4,000 ADRs should not imply 4,000 custom checkers. The intended model is many ADRs mapped to a smaller, governed catalog of reusable obligation patterns and a smaller number of analyzer capabilities. Analyze the repository once per commit/analyzer profile and correlate that evidence to all applicable obligations.

### Obligation types

| Type | Example | Evaluation approach |
|---|---|---|
| Exact deterministic constraint | `timeout_seconds` must be 1–120 | Typed config/schema verifier; reproducible PASS/FAIL |
| Structural code rule | Domain code must not import an adapter package | AST or pattern analyzer; reproducible PASS/FAIL when scope and evidence are adequate |
| Security pattern | Untrusted input must not reach a dangerous sink without validation | OpenGrep or a language-specific analyzer; report the rule and evidence |
| Architecture/topology | External ingress must pass through the approved gateway | Infrastructure/deployment/configuration inventory plus graph/relationship checks; may be indeterminate if the needed topology is not discoverable |
| Semantic principle | Prefer asynchronous messaging to reduce service coupling | Gather dependency, call, event, and integration evidence; ordinarily REVIEW_REQUIRED or confidence-scored assessment until an objective criterion is approved |
| Advisory design intent | Prefer small services with clear ownership | Human review or trend evidence; not automatically a hard gate |

Natural-language interpretation should propose obligations and candidate patterns, never silently convert an ambiguous principle into a hard FAIL rule. Store the source ADR section/quote and interpretation version with every generated obligation. Require review for new obligations, hard-gate changes, and low-confidence mappings.

## AWS target architecture

~~~mermaid
flowchart TD
  GH[GitHub event / repository scan request] --> IN[API Gateway + intake Lambda]
  IN --> BUS[EventBridge event]
  BUS --> Q[SQS work queue]
  Q --> SF[Step Functions assessment workflow]
  SF --> SNAP[ECS Fargate: checkout / source snapshot]
  SNAP --> S3[(S3: immutable source and run artifacts)]
  SNAP --> INV[Inventory + language / file discovery]
  INV --> PLAN[Analyzer plan and cache lookup]
  PLAN --> FAN[Step Functions Map: parallel analyzer jobs]
  FAN --> OG[ECS Fargate: OpenGrep adapter]
  FAN --> LANG[ECS Fargate: language-specific adapters]
  FAN --> GRAPH[ECS Fargate: Joern / graph analysis when needed]
  FAN --> CFG[ECS Fargate: config and policy analyzers]
  OG --> RAW[(S3: raw tool outputs)]
  LANG --> RAW
  GRAPH --> RAW
  CFG --> RAW
  RAW --> NORM[Normalization and validation]
  NORM --> EVID[(S3: normalized evidence)]
  NORM --> DB[(DynamoDB or Aurora PostgreSQL: run metadata, mappings, status)]
  EVID --> CORR[Requirement correlation and adjudication]
  DB --> CORR
  CORR --> REPORT[Markdown / JSON / SARIF report]
  REPORT --> OUT[GitHub check / PR comment / issue report]
  REPORT --> AUDIT[(S3: durable audit archive)]
~~~

This is a target deployment, not a claim that these AWS resources are deployed in the current MVP.

### AWS responsibilities

- **API Gateway and a small intake Lambda:** authenticate and validate webhook/request metadata, deduplicate events, and enqueue work. Keep source payloads small; do not run analyzers inside the intake function.
- **EventBridge and SQS:** decouple event production from work execution. Use a dead-letter queue, bounded retries, idempotency keys, and back-pressure.
- **Step Functions:** model orchestration, parallel analyzer fan-out, timeouts, retries, partial failures, and final aggregation. Use the optimized ECS/Fargate Run a Job integration for containerized analyzer tasks.
- **ECS Fargate:** run each analyzer/profile in an immutable container with explicit CPU, memory, timeout, filesystem, network, and IAM limits. Use separate task definitions where analyzers have different runtime or trust needs.
- **S3:** retain source snapshots where policy permits, raw outputs, normalized evidence, reports, tool manifests, and durable audit artifacts. Use encryption, restrictive bucket policies, lifecycle rules, and object versioning/retention as required.
- **DynamoDB or Aurora PostgreSQL:** store run state, source/commit identity, analyzer execution metadata, requirement mappings, review state, and queryable result metadata. Choose one based on query patterns; do not introduce both by default.
- **CloudWatch and tracing:** capture structured task/run logs, execution duration, queue age, failure rate, resource usage, and per-adapter metrics. Never log secrets or unrestricted source snippets by default.
- **GitHub integration:** publish a compact check/summary and links to full evidence; keep credentials in Secrets Manager or an appropriately scoped GitHub App installation. Scope permissions to the minimum needed.

For a small initial service, the same contracts can run locally or in GitHub Actions. Keep the Go core and adapters runnable outside AWS so local tests and the current issue-driven harness remain useful.

## Composite analyzer strategy

Treat analyzers as optional, versioned capabilities behind adapters. Do not run every engine on every repository. Discover languages and file types first, match obligations to required evidence, then run only the profiles that can produce that evidence.

| Capability | Preferred candidates | Use when | Design guidance |
|---|---|---|---|
| Multi-language structural patterns | [OpenGrep](https://github.com/opengrep/opengrep) | Language-aware structural matching and security patterns across common languages | Default candidate for the general pattern-engine slot; emit JSON/SARIF and retain rule IDs, fingerprints, locations, and tool version. Keep rule packs separately license-reviewed. |
| Lightweight syntax/structure matching | [ast-grep](https://github.com/ast-grep/ast-grep) | Fast syntax-node patterns where simpler structural matching is sufficient | Benchmark against OpenGrep; select one for overlapping use cases rather than paying to run both by default. |
| Go-specific security | [gosec](https://github.com/securego/gosec) | Go security checks, AST/SSA and data-flow checks | Preserve native rule IDs and SARIF/JSON outputs. Keep Pathfinder's custom Go import-boundary verifier until a replacement is demonstrated equivalent by tests. |
| Python security | [Bandit](https://github.com/PyCQA/bandit) | Python AST-based security checks | One language-specific adapter; do not reimplement its built-in security rules in Pathfinder. |
| Java / JVM bytecode | [SpotBugs](https://github.com/spotbugs/spotbugs) plus [Find Security Bugs](https://github.com/find-sec-bugs/find-sec-bugs) | Bytecode analysis and Java/Kotlin/Groovy security rules | A controlled build/artifact stage may be needed; do not execute untrusted project build scripts in the same isolation profile as source-only scans. |
| Deep code relationships / data flow | [Joern](https://github.com/joernio/joern) | Cross-function, interprocedural, code-property-graph queries when simpler analyzers cannot establish the evidence | Optional high-cost profile, triggered by applicable obligations. Model parser/build failures and unsupported languages as coverage gaps, not no-findings passes. |
| Declarative policy evaluation | [Open Policy Agent (OPA)](https://github.com/open-policy-agent/opa) and, where appropriate, Conftest | Evaluate normalized inventory, infrastructure/configuration facts, and organizational policy | Treat policy input and policy bundle versions as evidence provenance. OPA is a policy engine, not a source-code scanner. |
| Existing Pathfinder verifiers | Go standard-library parser and typed JSON checks | Exact checks already covered by current deterministic code | Keep these as first-class adapters. Add features here only when existing tools cannot express the requirement with acceptable precision. |

Open-source status is not the same as a blanket enterprise-use approval. At the time this design is being revised, upstream documentation describes OpenGrep under LGPL-2.1, gosec and Bandit under Apache-2.0, and SpotBugs / Find Security Bugs under LGPL-family licenses. Verify the exact license of each pinned release and all dependencies before production use. The license of a scanner does not automatically cover third-party rule packs. In particular, do not assume the upstream OpenGrep engine license applies to every available rules repository. Maintain an approved tool/rule inventory and have counsel or the organization's open-source review process approve distribution and operating model. Recheck licenses during upgrades.

### Adapter contract

Every adapter must implement the same lifecycle:

1. Validate its configuration and supported input.
2. Declare its tool name, exact version or image digest, enabled rule-set digest, supported languages/formats, scope, and capabilities.
3. Run within explicit resource/time limits and return a structured execution status even when the tool fails.
4. Preserve raw output as an immutable run artifact.
5. Emit normalized findings plus coverage metadata. Do not translate “tool returned zero findings” into “ADR passed.”
6. Include stable IDs/fingerprints so results can be deduplicated across adapters and runs.
7. Be contract-tested using fixtures for positive findings, clean input, unsupported input, parser errors, timeout, and tool failure.

An adapter should be thin. Parsing vendor output, normalizing locations/severity, attaching provenance, and converting errors are appropriate adapter responsibilities. Reimplementing analyzers' rules or data-flow engines is not.

## Normalized evidence contract

Use a versioned Pathfinder envelope as the canonical internal representation. SARIF remains a useful interchange/export format, but it does not by itself encode every Pathfinder-specific state such as requirement applicability, evidence completeness, or analyzer execution errors.

Each analyzer run should emit a run envelope containing:

- `schema_version`
- `run_id`, repository identity, commit/tree digest, and scan timestamp
- adapter, analyzer, version/image digest, ruleset digest, configuration digest, and supported capabilities
- `execution_status`: SUCCEEDED, PARTIAL, FAILED, TIMED_OUT, or SKIPPED
- `coverage`: discovered file/language counts, analyzed scope, excluded scope and reasons, parser failures, unsupported files, and whether coverage is sufficient for the requested capability
- an array of normalized findings
- raw artifact references and cryptographic hashes

A normalized finding should include:

~~~json
{
  "schema_version": "1.0",
  "finding_id": "stable-id",
  "analyzer": {
    "name": "opengrep",
    "version": "pinned-version",
    "ruleset_digest": "sha256:..."
  },
  "rule": {
    "id": "ORG.RULE.123",
    "name": "example-rule",
    "source_url": "https://example.invalid/rule",
    "category": "architecture"
  },
  "severity": "MEDIUM",
  "confidence": "HIGH",
  "message": "Forbidden dependency edge detected",
  "location": {
    "path": "internal/domain/customer.go",
    "start_line": 42,
    "start_column": 1,
    "end_line": 42,
    "end_column": 26
  },
  "evidence": {
    "kind": "source_span",
    "artifact_uri": "s3://evidence-bucket/run-id/raw/tool-output.json",
    "sha256": "sha256:...",
    "excerpt_redacted": "import example/adapter"
  },
  "fingerprint": "repository-relative-stable-fingerprint",
  "tags": ["dependency-boundary"],
  "related_locations": [],
  "raw_properties": {}
}
~~~

This is an illustrative contract, not the final Go struct or a promise that every analyzer populates every optional field. Define enums and required/optional fields in a schema before implementing adapters. Never treat a placeholder source URL or invented location as real evidence; adapters must omit unavailable fields.

### Normalization and correlation

1. **Preserve raw output.** Keep original tool JSON/SARIF, stdout/stderr metadata, exit code, and artifact checksum. Raw evidence is needed to audit adapter regressions.
2. **Normalize.** Map vendor severity into a documented common severity scale; retain original severity and rule ID in raw properties. Normalize paths against the scanned repository root, line/column conventions, fingerprints, confidence, and related locations.
3. **Deduplicate.** Prefer analyzer-native fingerprints, supplemented by rule ID + normalized path + source span + message identity. Do not merge semantically distinct detections merely because locations overlap.
4. **Correlate findings to obligations.** A mapping registry relates an obligation/pattern to acceptable analyzer rules, evidence kinds, scope, and minimum coverage. A finding can support multiple obligations; many findings may support one obligation.
5. **Adjudicate each obligation.** Apply deterministic acceptance criteria. Finding absence only supports PASS if the required analyzers completed, the evidence scope is sufficient, applicability is established, and the relevant rule/policy was enabled. Otherwise return the correct non-pass state.
6. **Export.** Produce the current Markdown/JSON/SARIF surfaces plus a run manifest with input commit, catalog version, adapter/tool/ruleset versions, coverage, and artifact hashes.

### Obligation assessment states

Keep analyzer execution state separate from the obligation verdict. The target vocabulary should distinguish at least:

- **PASS:** applicable obligation meets approved criteria and required evidence is complete.
- **FAIL:** evidence establishes a reproducible violation of an approved criterion.
- **NOT_APPLICABLE:** explicit scope/applicability logic says the obligation does not apply.
- **REVIEW_REQUIRED:** evidence is relevant but the principle requires an authorized human decision, or the automated interpretation is not yet approved.
- **EVIDENCE_MISSING:** the obligation applies but the evidence needed to evaluate it was not available.
- **UNSUPPORTED:** no registered analyzer/pattern can evaluate the obligation yet.
- **INDETERMINATE:** analysis could not produce a defensible outcome, such as parser errors or conflicting/insufficient signals.
- **ERROR:** internal evaluator or adapter failed; retain this separately from a genuine architecture violation.

The current MVP's report statuses are narrower. This target vocabulary is a migration design, not a claim that these states are already implemented. Until migrated, ensure missing evidence and partial coverage cannot be interpreted as complete compliance.

## Scaling ADR interpretation without one contract per ADR

Use a staged, auditable catalog pipeline:

1. **Ingest and parse ADRs.** Parse Markdown using a mature Go Markdown AST library such as [goldmark](https://github.com/yuin/goldmark); preserve heading hierarchy, blockquotes, lists, code blocks, and source ranges. Store original text and content digest.
2. **Extract candidate obligations.** Use rules and optionally a batch/offline LLM to propose statements, scope, strength, exception, candidate pattern, and evidence need. Persist model/prompt/version if used; do not let a runtime model decide a production PASS/FAIL.
3. **Classify and cluster.** Group equivalent obligations by intent and verification pattern, using both metadata and reviewer-curated mappings. Do not collapse obligations whose scope, exceptions, strength, or acceptance criteria differ.
4. **Map to reusable patterns.** Examples include dependency boundary, required configuration key/range, public endpoint authentication, encryption-at-rest declaration, prohibited dependency, event-driven coupling evidence, and approved gateway ingress. Each pattern references one or more analyzer capabilities and deterministic adjudication logic.
5. **Review and version.** Keep source ADR quote/range, proposed interpretation, reviewer, approval state, mapping version, and change history. New hard gates and low-confidence or semantic obligations require review.
6. **Measure before scaling.** Pilot on 100–200 representative ADRs across domains. Track extraction precision/recall from reviewer labels, percentage mapped to existing patterns, unsupported rate, applicability accuracy, false positives, evidence gaps, and analyzer cost/latency. Expand in cohorts only after the quality gate is met.

Use language models, if approved, for offline proposal and clustering assistance only. The accepted catalog and runtime verifier remain versioned, reviewable and deterministic. Semantic statements such as “prefer asynchronous messaging to reduce coupling” should first gather relevant graph/dependency evidence and surface it for review; do not fail a repository merely because it lacks an arbitrary numeric threshold that was never in the ADR.

## Security, reproducibility, and governance

- Treat repository source, ADR text, build metadata, and analyzer output as untrusted data.
- Use non-root containers, read-only source mounts where feasible, ephemeral task storage, least-privilege IAM, CPU/memory/time quotas, and controlled/no network egress by default.
- Never expose GitHub tokens or cloud credentials to analyzer containers. Retrieve source using a separate scoped stage and pass short-lived, least-privilege artifacts.
- Do not run arbitrary build/test scripts as part of source-only scanning. If bytecode/compiled artifacts are required, use a separate sandboxed build profile.
- Pin container images by digest, tools by version, analyzer configurations and rulesets by digest, and retain the effective profile in the run manifest.
- Inventory licenses for binaries, libraries, container base images, plugins, and rule packs. Recheck at upgrade time and preserve attribution/notice obligations as applicable.
- Make analysis idempotent. A recommended cache key includes repository/tree digest, analyzer image digest, configuration/ruleset digest, and relevant build-artifact digest. A cache hit is usable only when all key inputs match.
- Encrypt evidence at rest and in transit; define access, retention, and deletion policies for source and finding evidence. Redact secrets and avoid storing source excerpts where a hash plus secure raw artifact reference is sufficient.
- Keep finding verdict, analyzer execution status, and coverage as separate fields in API, persistence, and reports.

## Incremental implementation plan

1. Define the versioned normalized run/finding/coverage schema and contract tests in the Go codebase.
2. Add an adapter interface and normalize the existing Go import and JSON verifiers into that envelope without changing their semantics.
3. Add one external analyzer integration (OpenGrep is the proposed first benchmark), with a pinned image, fixture suite, raw-output retention, and license review.
4. Compare analyzer output and runtime against ast-grep; pick by precision, supported languages, operational effort, and license fit instead of running both by default.
5. Add language-specific adapters where repository inventory and accepted obligations justify them; keep Joern as an opt-in deep-analysis profile.
6. Add obligation-to-pattern mapping and separate requirement coverage from analyzer/file coverage.
7. Pilot ADR extraction and clustering on a labeled sample before processing the full 4,000-ADR corpus.
8. Introduce AWS orchestration only after local/container execution, schemas, idempotency, retry semantics, and evidence retention are stable. Deploy the worker path incrementally with queue/DLQ, Step Functions, Fargate, S3, metadata storage, monitoring, and GitHub integration.

## Upstream references

- [OpenGrep](https://github.com/opengrep/opengrep)
- [ast-grep](https://github.com/ast-grep/ast-grep)
- [gosec](https://github.com/securego/gosec)
- [Bandit](https://github.com/PyCQA/bandit)
- [SpotBugs](https://github.com/spotbugs/spotbugs)
- [Find Security Bugs](https://github.com/find-sec-bugs/find-sec-bugs)
- [Joern](https://github.com/joernio/joern) and [Joern code property graphs](https://docs.joern.io/code-property-graph/)
- [OPA](https://github.com/open-policy-agent/opa)
- [goldmark](https://github.com/yuin/goldmark)
- [Step Functions integration with ECS/Fargate](https://docs.aws.amazon.com/step-functions/latest/dg/connect-ecs.html)

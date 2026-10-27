# Scaling ADR Compliance to Thousands of Decisions

## Objective

Extend the existing Pathfinder/ADR Compliance Engine in `tyler-beach/pf_2`, not replace it. Preserve the Go assessment engine, existing deterministic verifiers, Markdown/JSON/SARIF reporting, GitHub Actions workflows, and Issue-triggered end-to-end tests.

The target is to support approximately 4,000 ADRs without maintaining 4,000 bespoke requirement contracts. Pathfinder becomes a small orchestration and evidence layer over existing analysis tools, with an automated, reviewable ADR ingestion pipeline in front of it.

**Runtime invariant:** normal repository assessment remains deterministic and makes zero LLM calls. Any optional AI-assisted interpretation is a separate offline authoring/ingestion step. It produces reviewable candidate artifacts, never runtime policy.

## 1. Extend the current repo incrementally

| Existing component | Keep | Evolve |
|---|---|---|
| `internal/assessment/assessment.go` | Existing assessment/result model and Go/JSON verifier behavior | Introduce analyzer adapters and normalized evidence so multiple tools can feed the same engine. Avoid a large switch statement becoming the permanent integration point. |
| `adr/requirements.json` | Keep it valid and supported as the current compatibility path | Gradually introduce source-backed ADR/obligation records and reusable pattern bindings. Generate indexes from actual ADR Markdown instead of treating this file as the long-term source of truth. |
| Issue Markdown parser | Keep constrained snippet parsing for issue-submitted evidence | Add a separate Goldmark-based ADR ingestion parser. A full ADR document parser and a small code-snippet parser have different jobs. |
| `internal/assessment/report.go` | Keep Markdown, JSON, and SARIF output | Add analyzer identity/version, source location, provenance, applicability reason, tool health, and missing-evidence/review states. |
| GitHub Actions | Keep Issue assessment, repository CI, and PR/SARIF paths | Add pinned analyzer steps as needed. Run each configured analyzer once per repository snapshot, then map its normalized output to all applicable ADR obligations. |
| Existing tests and live Issue scenarios | Keep as regression and E2E evidence | Add adapter parity, analyzer failure, unsupported scope, missing evidence, and corpus ingestion fixtures. |

The existing custom checks remain useful. Do not remove them simply because an external tool may overlap: first demonstrate parity, required evidence quality, and a real maintenance benefit.

## 2. Target architecture

```text
ADR Markdown + metadata + links
             |
             v
 Goldmark AST ingestion (source lines, hashes, headings)
             |
             v
 Candidate ADR interpretation / classification / clustering
             |
             v
 Human review and versioned obligation registry
             |
             +--------------------------+
             |                          |
             v                          v
 Reusable pattern registry       Scope, exceptions, owners
             |
             v
 Repository snapshot -> analyzer orchestrator -> tool adapters
                                              (once per repo snapshot)
                                                        |
                                                        v
                                            normalized evidence
                                                        |
                                                        v
                                      obligation-to-evidence evaluation
                                                        |
                                                        v
                                Markdown + JSON + SARIF + coverage metrics
```

Three concepts remain separate:

- **Source ADR:** the authoritative decision, rationale, context, alternatives, scope, exceptions, links, and version.
- **Obligation:** an approved or candidate interpretation of a specific statement in that ADR, with source provenance and enforcement mode.
- **Verification pattern:** a reusable method for gathering/evaluating evidence, such as a package-boundary rule, configuration policy, or architecture-graph check.

One ADR can produce multiple obligations; many ADRs can reference the same pattern; an ADR can also be explicitly advisory-only or not machine-verifiable.

## 3. ADR ingestion: parse documents before interpreting meaning

Add [Goldmark](https://github.com/yuin/goldmark) as the Go Markdown AST parser for the ingestion path. Use it to retain document structure: headings, paragraphs, lists, links, code fences, and source locations. Avoid extracting decisions from flattened text or regular expressions alone.

Support an established format such as [MADR](https://adr.github.io/madr/) while retaining older ADR files as they are. The parser should be tolerant of variation and report what it did not recognize; it should not require rewriting the entire corpus before adoption.

For each discovered ADR, retain:
- Stable ADR ID, canonical path, source hash, source lines, and content version.
- Title, status (proposed/accepted/superseded/retired), owner, effective date, and related ADR links where available.
- Original text as authoritative source; generated interpretations must never overwrite it.
- References to services, repositories, components, environments, and approved exceptions when those can be grounded in source metadata.

Make ingestion idempotent. Reprocessing the same content hash must not create duplicate records. Version the resulting index so assessment reports can state exactly which ADR and obligation versions were used.

## 4. Candidate interpretation and approval

The ingestion pipeline can use deterministic cues for headings, normative language, scope phrases, and exceptions. Optional LLM assistance may classify ambiguous prose in a separate batch job, but the output must remain a candidate requiring review. There must be no production/runtime model dependency.

Each candidate obligation should contain:
- Obligation ID and source ADR ID/version.
- Exact source quote, path, start/end lines, and quote/content hash.
- Normalized statement, kept separately from the quote.
- Modality: MUST, MUST NOT, SHOULD, SHOULD NOT, PREFER, MAY, or CONTEXTUAL.
- Type: hard invariant, policy constraint, design preference, qualitative goal, rationale, or exception.
- Scope (system/service/repository/language/environment), conditions, exclusions, and related decisions.
- Proposed reusable pattern, evidence sources, expected observations, limitations, and candidate confidence.
- Review/approval status and owner.

Do not force every paragraph into an enforceable rule. Explicitly classify rationale, context, and non-machine-verifiable decisions. Before a candidate can fail CI, a human owner must confirm its meaning, scope, exceptions, pattern, severity, and positive/negative regression fixtures. Changes to an ADR, interpretation, pattern binding, exception, or enforcement mode are versioned.

## 5. Reusable verification patterns

Use a catalog of patterns and parameters rather than one custom Go verifier per ADR. A pattern definition should state its inputs, analyzer requirements, applicability conditions, normalized output, blind spots, version, and test fixtures. Keep each ADR's own intent, scope, and exceptions in its obligation record.

Useful pattern families include:
- Configuration schema, required keys, types, and value bounds.
- Language-level structure and API-use rules.
- Package/dependency boundaries and allowed/forbidden graph edges.
- Security/data flow and source-to-sink relationships.
- Architecture graph constraints, service dependencies, and ingress/egress paths.
- Infrastructure/deployment policy.
- Advisory signals and human review for qualitative architectural preferences.

Illustrative record (a target schema, not a claim about current fields):

```yaml
obligation_id: OBL-ADR-017-02
adr_id: ADR-017
adr_version: 3
source:
  path: docs/adr/ADR-017.md
  start_line: 24
  end_line: 26
  quote_sha256: "<hash-of-original-text>"
statement: "External ingress must pass through the approved gateway."
modality: MUST
kind: hard_invariant
scope:
  systems: ["customer-platform"]
verification:
  pattern_id: architecture.ingress_via_gateway
  mode: hard
  evidence_sources: ["deployment-manifests", "service-routing-graph"]
exceptions: []
approval:
  status: approved
  policy_version: 1
```

Keep `adr/requirements.json` working during migration. New generated ADR indexes and obligation records should be added alongside it first; only consolidate or change the compatibility schema after parity tests prove the new path preserves current results.

## 6. Analyzer integrations: choose by capability, not by tool count

Create thin adapters that invoke version-pinned CLI tools and convert output to Pathfinder's evidence schema. Do not embed entire analysis frameworks into the Go assessment engine. The analyzer adapter should report tool identity/version, configuration, completion status, supported scope, original finding IDs, locations, and preserved raw output.

| Capability | Candidate | How it fits Pathfinder |
|---|---|---|
| Go AST and snippets | Go standard library parser (current implementation) | Keep as the low-dependency baseline, especially for issue-submitted snippets without full module context. |
| Repository-wide Go architecture | [Arch-Go](https://github.com/arch-go/arch-go) | Benchmark allowed/forbidden import/package rules against existing Go boundary fixtures. If it produces adequate evidence at repository scale, prefer it for those checks; keep the current parser for snippets and fallback cases. |
| Markdown AST | [Goldmark](https://github.com/yuin/goldmark) | Use for ADR document ingestion, section structure, and source locations; it is not a code analyzer. |
| Structured configuration policies | [Conftest](https://github.com/open-policy-agent/conftest) + [OPA](https://www.openpolicyagent.org/) | Introduce for larger, reusable JSON/YAML/Kubernetes/Terraform policy families. Keep the small current JSON required-key/range verifiers until equivalent results and a maintenance benefit are demonstrated. |
| Multi-language code patterns | [Opengrep](https://github.com/opengrep/opengrep) | Preferred general-purpose pattern-engine candidate to benchmark for reusable structural/security checks; supports JSON/SARIF outputs and is LGPL-2.1. Write/curate rules with a separate license review for every external rule pack. |
| AST structural search | [ast-grep](https://github.com/ast-grep/ast-grep) | Lightweight alternative for syntax-pattern checks. Benchmark against Opengrep; use both only when a demonstrated coverage gap justifies it. MIT-licensed. |
| Deeper data flow / code graphs | [Joern](https://github.com/joernio/joern) | Optional specialist analyzer when an ADR requires relationships that local AST/pattern checks cannot prove. Pilot its language support, complexity, and evidence quality before adoption. Apache-2.0. |
| Go security rules | [gosec](https://github.com/securego/gosec) | Add where Go security ADRs need its specific rule coverage; it emits SARIF. Apache-2.0. |
| Python security rules | [Bandit](https://github.com/PyCQA/bandit) | Add only for Python repositories/obligations that require it; Apache-2.0. |
| Java security analysis | SpotBugs + Find Security Bugs | Add for Java coverage only after validating support, evidence quality, and component licenses. |
| PR annotations | [reviewdog](https://github.com/reviewdog/reviewdog) | Keep as a consumer/publisher of findings, not an analyzer or completeness oracle. |

This shortlist is a decision framework, not an instruction to install every tool. First measure whether an existing tool covers a real gap. For code patterns, use Opengrep as the primary enterprise-open-source candidate and ast-grep as an alternative/companion based on benchmark results; Semgrep CE can be included as a comparison point, not assumed to be the default. Do not adopt Tree-sitter as a complete engine—it is a parser foundation. Defer Structurizr or a vector database until actual architecture-model or retrieval evaluations demonstrate value. Start with source metadata, hashes, lexical search, and clustering before adding semantic infrastructure.

### Enterprise licensing and supply-chain policy

- Pin tool releases or immutable digests and record exact versions/configuration in results.
- Review the engine, plugins, transitive dependencies, rulesets, and redistribution model separately.
- Opengrep's engine is LGPL-2.1; do not assume that a third-party rule repository uses the same license. Prefer internally authored or clearly compatible rules until each pack has been reviewed.
- Do not assume Semgrep rule packs share the engine's license. Treat Semgrep's separate rules license as a distinct legal review item and do not redistribute such rules without authorization.
- Maintain an SBOM/license inventory and review upgrade diffs. These are engineering recommendations, not legal advice.

## 7. Run analyzers once and normalize evidence

For each immutable repository snapshot:
1. Discover source files, languages, build metadata, manifests, generated-code paths, and configured scope.
2. Run each enabled analyzer once over its configured scope—not once per ADR.
3. Preserve original output and normalize findings into a common evidence record.
4. Evaluate each obligation's applicability and required evidence against the shared result set.
5. Emit the current Markdown/JSON/SARIF formats with additional provenance and coverage dimensions.

Normalized evidence should identify:
- Repository and immutable commit/tree SHA.
- Analyzer name, exact version/digest, configuration hash, rule ID, and run status.
- Artifact path, source range, evidence excerpt or structured observation, and evidence hash.
- Scope analyzed, languages/files supported, exclusions, parse errors, and whether the run completed.
- Original raw result reference and every obligation/pattern that consumes the evidence.

Cache results by commit/tree SHA, analyzer version, configuration hash, and relevant input hashes. Deduplicate equivalent findings while preserving all ADR/obligation references. Analyzer failures and incomplete scopes must be represented as failures of evidence collection, never as empty success. A successful analyzer run with no findings is only evidence about the rules and scope it actually covered.

## 8. Semantic ADRs and graduated enforcement

Example: **“Prefer asynchronous messaging to reduce coupling between services.”**

This is a preference and an intended outcome, not an absolute ban on synchronous calls. The system should:
1. Preserve the stated preference, rationale, scope, and exceptions.
2. Gather available architecture evidence such as synchronous HTTP/RPC edges, message-bus links, service dependency graphs, and call fan-out.
3. Produce an evidence-backed advisory signal for synchronous coupling across the named boundary.
4. Use a hard threshold only if the ADR or an approved policy defines one.
5. Otherwise return `REVIEW_REQUIRED` or an advisory result, not an automatic hard failure.

By contrast, “external ingress MUST pass through the approved gateway” can be a hard check if deployment/routing evidence is complete enough to establish the path. If manifests are missing or the graph cannot be resolved, the result must remain indeterminate or evidence-missing. Lack of a detected edge is not proof that a mandatory gateway is present.

## 9. Separate interpretation, applicability, verification, and assessment coverage

The current `COMPLETE`/`PARTIAL`/`NONE` field only describes the current catalog's applicable PASS/FAIL results. It does **not** prove that all ADRs have been interpreted or that every architectural principle is mechanically verifiable.

Add explicit, separately reported dimensions:
- **Inventory coverage:** current accepted ADRs discovered versus expected.
- **Interpretation coverage:** relevant ADR content classified as obligation, rationale, exception, or explicitly non-enforceable.
- **Binding coverage:** obligations tied to an approved reusable pattern, review process, or documented unsupported reason.
- **Evidence coverage:** required tools ran successfully over the relevant scope and provided enough evidence.
- **Assessment coverage:** applicable obligations with valid results for this repository commit.

The target schema should distinguish:
- `NOT_APPLICABLE`: the obligation does not apply by explicit scope/condition.
- `EVIDENCE_MISSING`: evidence required to assess the obligation was not provided or discovered.
- `UNSUPPORTED`: no approved evaluator can assess this obligation/evidence type yet.
- `REVIEW_REQUIRED`: evidence exists but human judgment is required.
- `INDETERMINATE`: analysis or evidence quality does not support a verdict.
- `PASS` / `FAIL`: valid evaluated outcomes for applicable deterministic checks.

Implement these statuses compatibly; do not silently redefine current fields. In particular, the existing issue-snippet workflow may legitimately lack a repository file, but repository assessment must not report a missing required file as `NOT_APPLICABLE` simply because it was absent from the evidence payload. Applicability and evidence availability must be independent fields.

Before enabling new semantic states in CI, explicitly define which states block a merge, warn, or require a human decision. Missing evidence, unsupported rules, and analyzer errors must never be converted into PASS.

## 10. Incremental delivery sequence

### Step A — Refactor without behavior changes
- Separate catalog/schema validation, analyzer dispatch, evidence normalization, and report generation.
- Preserve current Go/JSON behavior, unit tests, Issue workflows, and report/SARIF paths.
- Add a regression test for the distinction between true non-applicability and missing evidence.
- Define intended CI policy for new statuses before making them blocking.

### Step B — Add ADR ingestion
- Add Goldmark and tests for headings, lists, links, fenced code, unusual whitespace, line provenance, and malformed/legacy formats.
- Add MADR compatibility while preserving older ADRs.
- Build an idempotent source-backed ADR index and candidate obligation schema alongside `adr/requirements.json`; keep the current catalog runnable.

### Step C — Benchmark analyzers behind adapters
- Benchmark Arch-Go against existing Go boundary fixtures.
- Benchmark Conftest/OPA on representative structured-policy cases; retain the current simple JSON verifiers unless replacement clearly reduces maintenance without regressing evidence.
- Benchmark Opengrep and ast-grep on representative structural/security rules. Add language-specific tools only where a real ADR/repository requires them.
- Pin versions, retain raw output, normalize findings, and prove parity before switching a verifier's backend.

### Step D — Prove pattern reuse and interpretation quality
- Select a representative sample of 100–200 ADRs across teams and styles.
- Have architects label a subset to measure extraction precision/recall, scope accuracy, exception handling, and pattern reuse.
- Test compliant, violating, malformed, missing-evidence, unsupported-language, analyzer-error, and review-required scenarios.
- Track false positives, missed violations, run time, reused evidence, percentage of obligations mapped to existing patterns, and unclassified ADRs.

### Step E — Expand by capability, not raw ADR count
- Expand to the wider corpus only after quality gates are met.
- Add ownership views for unclassified/superseded ADRs, obligations without a supported pattern, unsupported evidence types, stale exceptions, and repeated review-required findings.
- Reassess bindings when an ADR, analyzer, ruleset, or scope changes.

## Success criteria

- The existing repo remains backward-compatible during migration; current Issue tests continue to serve as regression tests.
- No one-checker-per-ADR architecture is introduced.
- Many ADRs can map to a smaller, governed set of reusable patterns.
- Analyzer tools run once per repository snapshot, and findings are reused by applicable obligations.
- Every obligation and finding preserves source provenance and versions.
- Missing evidence, tool failure, unsupported cases, and semantic review are never reported as compliance.
- The proportion of the corpus interpreted, automatically assessed, advisory-only, unsupported, and awaiting review is visible.
- No new blocking analyzer is adopted without representative positive/negative fixtures, parity evidence, and license review.

# Specification-to-enforcement milestone

## Goal

Turn plain-English technical specifications and ADRs into a reviewable set of obligations that can be mapped to deterministic, language-appropriate verification patterns. Preserve the existing assessment engine and its zero-runtime-LLM behavior.

This is a staged capability, not a claim that arbitrary prose can already be compiled into trustworthy policy. Candidate extraction is discovery only; it must never activate enforcement by itself.

## End-to-end contract

For each submitted specification:

1. **Ingest and preserve** the exact source bytes, source identity, version, and line/section locations.
2. **Extract candidate obligations** with deterministic IDs and source citations. Keep modality (must/shall/should/may), scope, conditions, exceptions, and ambiguity visible. Never extract examples from fenced code as normative requirements.
3. **Classify confidence and ambiguity.** Unknown, compound, contradictory, subjective, or underspecified statements must be marked for review rather than silently normalized into executable rules.
4. **Review and approve.** A reviewer confirms the interpretation, applicability, severity, exceptions, and intended enforcement mode. Candidate extraction alone is not approval.
5. **Bind to a verification pattern.** Select a typed verifier or analyzer rule only if its input contract and language/configuration coverage match the approved obligation. A pattern can serve multiple obligations.
6. **Collect evidence.** Evidence must identify repository snapshot, source path and range, analyzer and rule versions, relevant configuration, and integrity hashes where available.
7. **Assess each obligation independently.** Use `PASS`, `FAIL`, `INDETERMINATE`, or `NOT_APPLICABLE` with a reason and evidence. Unsupported language, absent files, incomplete scan, stale evidence, parse failure, and ambiguous interpretation must not be reported as a pass.
8. **Report coverage honestly.** Distinguish requirement interpretation coverage from code-evidence coverage. A clean scan means only that enabled rules found no matching issue in the scanned scope.

## Example specification

Input:

> All outbound HTTP clients must set a finite request timeout no greater than 30 seconds. Automatic retries must be limited to three attempts. Each service must expose a health endpoint. Exceptions require an approved, documented interoperability constraint.

Expected candidate obligations (all initially review-required):

| Candidate | Meaning | Expected handling |
|---|---|---|
| A | Outbound HTTP calls have a finite timeout | Candidate; needs a supported implementation/configuration binding |
| B | Timeout maximum is 30 seconds | Candidate; numeric bound verifier only if the timeout source is identified |
| C | Retries are capped at three attempts | Candidate; must clarify whether “attempts” includes the initial request |
| D | Each service exposes a health endpoint | Candidate; needs an explicit endpoint/framework-specific observable pattern or manual review |
| E | Exceptions need approved documentation | Candidate exception/control obligation; never auto-infer approval |

The engine must not collapse this into a single generic “secure” result. If a requirement has no suitable verifier or the source of truth is not identified, preserve the candidate and report that it requires review or lacks evidence.

## Required data boundaries

- **Source document:** immutable input, source digest, parser version, document metadata.
- **Candidate obligation:** stable candidate ID, exact source citation, statement, modality, obligation kind, scope/conditions/exception hints, extraction method/version, and review-required state.
- **Approved binding:** reviewer and decision reference, approval revision, target requirement/pattern IDs, enforcement mode, source identity, and a digest of the exact owning ADR metadata plus requirement/verifier definition. If that definition changes or an older approval has no digest, the binding becomes stale and requires re-review.
- **Verification evidence:** repository snapshot, paths/locations, analyzer and rule versions, config/rule/input digests, scan status, and evidence completeness.
- **Assessment result:** per-obligation status, evidence references, explanation, coverage status, and explicit unknown reasons.

The review registry and accepted requirement catalog remain separate. Never mutate accepted requirements as a side effect of extraction.

## Language support contract

Language support must be reported per **rule family**, not just by parser availability. A language/rule binding is eligible for enforcement only when it has:

- a pinned analyzer/tool version and rule digest;
- a vulnerable fixture expected to produce the intended finding;
- a clean fixture expected not to produce that finding;
- stable source locations and normalized evidence;
- an explicit supported-file scope;
- regression tests for incomplete scans and unsupported files.

Repository scanning and issue-snippet assessment are separate surfaces. Never execute issue-submitted source code or imply that a repository-only analyzer ran on snippets when it did not.

## Milestone phases and gates

### Phase 1 — Contract and fixtures
- Document the candidate → review → binding → evidence → assessment lifecycle.
- Add a representative natural-language specification fixture and expected candidate/ambiguity cases.
- Align language-coverage documentation with tested rule families.
- Gate: examples are explicit about what is currently implemented versus target behavior.

### Phase 2 — Candidate quality
- Improve deterministic extraction for compound statements, conditions, exceptions, and ambiguity.
- Preserve exact source spans and stable identity.
- Add negative tests for descriptive prose, quoted/fenced examples, negation, and conflicting statements.
- Gate: repeat extraction produces byte-identical output; no candidate is executable by default.

### Phase 3 — Typed binding and evidence contracts
- Define a versioned, validated binding model between approved obligations and existing verifier types.
- Reject incompatible or cross-ADR bindings and stale source approvals.
- Make missing coverage and missing evidence visible per obligation.
- Gate: invalid bindings fail closed; existing requirements remain backward compatible.

### Phase 4 — End-to-end specification assessment
- Run the same specification through ingestion, candidate review, binding, repository analysis, and report generation.
- Add pass, fail, indeterminate, not-applicable, unsupported-language, and stale-evidence fixtures.
- Gate: all expected per-obligation outcomes match, with source citations and reproducible evidence.

### Phase 5 — Broader language and policy coverage
- Expand only rule families with vulnerable/clean fixtures and evidence-quality tests.
- Add language-specific AST/type-aware checks where structural patterns are insufficient.
- Gate: claims are made per verified rule family; no blanket claim of semantic support for every language.

## Current CLI workflow

The current implementation can join a repository assessment to reviewed candidate obligations without running a second, potentially different scan:

1. Generate a source-bound candidate index with `extract-obligations`.
2. Record reviewer decisions with `record-obligation-review` and validate the ledger with `validate-obligation-reviews`.
3. Produce repository-mode evidence with `assess-repo`.
4. Join the results using `assess-obligations`, which writes a per-candidate JSON report and Markdown table.

Example:

```sh
go run ./cmd/adrctl ingest-adrs --root adrs --json-out adr/ingested-adrs.json
go run ./cmd/adrctl extract-obligations --index adr/ingested-adrs.json --json-out adr/obligation-candidates.json --report adr/obligation-candidates.md
go run ./cmd/adrctl assess-repo --root . --json-out assessment.json
go run ./cmd/adrctl assess-obligations \
  --root . \
  --index adr/obligation-candidates.json \
  --ingestion-index adr/ingested-adrs.json \
  --requirements adr/requirements.json \
  --registry adr/obligation-reviews.json \
  --assessment assessment.json \
  --json-out obligation-assessment.json \
  --report obligation-assessment.md \
  --enforce-blocking
```

`--enforce-blocking` is an opt-in CI gate: after writing the audit artifacts, it returns a nonzero command result if any candidate remains pending review, or if any approved `blocking` obligation is `FAIL`, `INDETERMINATE`, `NOT_APPLICABLE`, or otherwise not `PASS`. This includes stale or missing evidence and prevents newly added obligations from bypassing enforcement just because they lack an approval. Explicitly rejected candidates are `NOT_APPLICABLE`. Without the flag, assessment remains report-only. Approved `advisory` and `review_required` bindings do not trigger the blocking gate; they remain visible in the report and cannot be mistaken for a blocking pass.

The join first rebuilds the candidate index from the supplied ingestion index and current ADR Markdown under `--root`; a stale or altered candidate index is rejected. It then requires repository-mode assessment evidence and compares the assessment source digest and any analyzer repository digest against the current `--root`. Stale snapshots, nonzero runtime LLM-call counts, missing reviews, stale review snapshots, unbound/manual-review candidates, missing requirement results, or PASS/FAIL results without supporting evidence cannot pass. Stale approved bindings are downgraded to `INDETERMINATE`; rerun `assess-repo` against the current tree. The report separates the underlying repository assessment verdict from the overall obligation verdict: any known failing approved obligation yields `FAIL`; otherwise unresolved or stale obligations yield `INDETERMINATE`; `PASS` requires at least one passing approved obligation and no failures or indeterminate obligations. Each candidate report row also exposes the digest of the exact approved requirement/verifier binding; the report records exact SHA-256 digests for the candidate index, review registry, and assessment artifact. Extraction also flags compound constraints, conditions/exceptions, subjective wording, and normative prose found in contextual ADR sections; these hints increase review visibility but never approve a candidate. A `review_required` binding also remains indeterminate for enforcement purposes even if its associated requirement result is clean.

## Security and correctness invariants

- Extraction is not approval.
- No unsupported or unbound obligation can block or pass as if verified.
- No missing, stale, incomplete, or failed analyzer evidence can produce a clean pass.
- Source paths must be repository-relative and traversal-safe.
- Analyzer subprocesses must remain pinned, bounded, and credential-restricted.
- Results must be deterministic for the same inputs, versions, and configuration.
- Runtime LLM calls remain zero unless a future, separately approved design explicitly changes that constraint; any optional batch-time model output must remain untrusted candidate data requiring review.

## Definition of done

The milestone is complete only when an ordinary English specification can travel through the entire lifecycle, every extracted obligation is traceable to its source, approved bindings invoke compatible verifiers, and the resulting per-obligation statuses are reproducible across compliant and non-compliant repositories. Documentation or candidate extraction alone does not satisfy this definition.

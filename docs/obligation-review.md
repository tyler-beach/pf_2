# Review registry for ADR obligation candidates

## Boundaries

The candidate index is a discovery artifact, not an executable policy. It is safe to regenerate from the source ADR corpus. The review registry is an append-only audit ledger and must be preserved separately. Do not delete it merely because an ADR or candidate disappeared.

## Decisions

- `pending_review`: the source-backed statement needs a human decision. No requirement or pattern binding is claimed.
- `approved`: a reviewer approved the current candidate interpretation. A machine-checkable binding may be recorded only when the target requirement exists, is active in an accepted ADR, belongs to the same ADR, and uses a verifier compatible with the selected pattern.
- `rejected`: retain the decision and rationale, but do not bind it to a requirement.
- `superseded`: retain the historical record while explicitly retiring a candidate that is no longer in the current candidate index.

Each decision is a new revision. Existing records are not modified. Registry persistence uses atomic replacement and mode 0600. Candidate source identity and SHA-256 are stored in each record. If a refreshed candidate index changes that identity or the source digest, the previous approval becomes stale and no longer contributes a blocking/advisory binding.

## Enforcement rules

- Candidate extraction never updates `adr/requirements.json`.
- A blocking binding is valid only for a candidate from an accepted ADR.
- Cross-ADR bindings are rejected.
- `manual_review` cannot bind to a requirement or become blocking/advisory.
- Rejected/pending/superseded decisions cannot contain requirement or pattern IDs.
- Strict validation fails on stale active decisions and unresolved orphaned history; non-strict validation still reports these counts for migration/recovery.
- A clean registry can still contain pending or unbound candidates. Coverage metrics must be read separately from code evidence coverage; zero blocking bindings does not mean all ADRs have been interpreted or assessed.

## CLI

Initialize a ledger once:

~~~sh
go run ./cmd/adrctl review-init --output adr/obligation-reviews.json
~~~

Record a review by providing a JSON proposal:

~~~json
{
  "candidate_id": "CAND-...",
  "decision": "pending_review",
  "enforcement_mode": "review_required",
  "reviewer": "security-architect@example.com",
  "reviewed_at": "2026-10-27T12:00:00Z",
  "review_reference": "https://github.com/owner/repo/issues/123",
  "rationale": "Need to verify applicability, exceptions, and observable evidence."
}
~~~

~~~sh
go run ./cmd/adrctl record-obligation-review \
  --index adr/obligation-candidates.json \
  --requirements adr/requirements.json \
  --registry adr/obligation-reviews.json \
  --proposal review-proposal.json \
  --coverage-out adr/obligation-review-coverage.json
~~~

Check history and current binding coverage:

~~~sh
go run ./cmd/adrctl validate-obligation-reviews \
  --index adr/obligation-candidates.json \
  --requirements adr/requirements.json \
  --registry adr/obligation-reviews.json \
  --json-out adr/obligation-review-coverage.json \
  --strict
~~~

The registry is not a cryptographically signed ledger; filesystem permissions and source hashes provide local integrity/freshness checks, not tamper-proof identity or external attestation. Use protected review workflows and repository controls for high-assurance approval.

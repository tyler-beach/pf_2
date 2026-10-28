# OWASP + AWS MADR labeled corpus

Target scale: **200** MADR ADRs under `adrs/` with matching rows in `labels.json`.

| Slice | Count | IDs |
|---|---|---|
| OWASP Top 10 (2021) | 120 | ADR-3001..3120 |
| AWS security practices | 80 | ADR-3201..3280 |

Each ADR uses MADR sections (`Status`, `Context`, `Decision`, `Consequences`). Normative language lives only in **Decision** and follows **RFC 2119** (`MUST` / `SHALL` for hard requirements, `MUST NOT` / `SHALL NOT` for prohibitions, `SHOULD` / `MAY` for softer guidance). Theme tags and fine-grained pattern IDs live in `taxonomy.yml` / `corpus_index.json`; harness labels use extract-compatible pattern families.

Regenerate from the checked-in taxonomy builder:

```bash
python3 scripts/generate_security_corpus.py
```

OpenGrep rule packs that accompany the corpus (shared across many ADRs, not 1:1):

- `rules/pathfinder-security-core.yml` — legacy shell/eval rules
- `rules/owasp-injection.yml`
- `rules/owasp-crypto.yml`
- `rules/owasp-ssrf.yml`
- `rules/owasp-secrets.yml`
- `rules/aws-sdk-misuse.yml`
- `rules/owasp-misc-promote.yml` — hardening, supply-chain, logging, abuse-control, plugin-boundary, provenance promotions

Merge packs into the CI default config with `python3 scripts/merge_opengrep_rules.py`.

Checkov (IaC) configuration and fixtures:

- `rules/checkov.yml` — pinned frameworks (terraform/cloudformation/kubernetes/dockerfile)
- `testdata/analyzer-fixtures/{vulnerable,clean}/iac/` — positive/negative Terraform fixtures

Label fields:

- `statement_contains`: unique marker expected in an extracted candidate
- `modality`, `pattern_id`, `catalog_pattern_id`, `confidence_band`
- `escalate_expected`: true when the candidate should land in the low-confidence LLM assist band

## Enforce pack (issue + sandbox assess)

`requirements.json` is the catalog used by **ADR Assessment** issues (all 200 ADRs). Regenerate after taxonomy changes:

```bash
python3 scripts/generate_security_corpus.py
python3 scripts/generate_corpus_bindings.py
go run ./cmd/adrctl validate --requirements testdata/corpus/requirements.json
```

Local sandbox mimic of an issue with code fences:

```bash
python3 scripts/issue_sandbox_extract.py --input /path/to/issue.md --out-dir /tmp/issue-src
# copy rules into the sandbox, then:
go run ./cmd/adrctl scan-opengrep --root /tmp/issue-src --config rules/pathfinder-security.yml ...
go run ./cmd/adrctl scan-checkov --root /tmp/issue-src --config rules/checkov.yml ...
go run ./cmd/adrctl assess-repo --root /tmp/issue-src --requirements testdata/corpus/requirements.json \
  --issue-surface \
  --analyzer-evidence analyzer-evidence/opengrep.json,analyzer-evidence/checkov.json \
  --analyzer-sarif analyzer-evidence/opengrep.sarif,analyzer-evidence/checkov.sarif \
  --report /tmp/report.md --json-out /tmp/assessment.json
```

## Extract harness

```bash
go run ./cmd/adrctl ingest-adrs --root testdata/corpus/adrs --json-out /tmp/corpus-ingested.json
go run ./cmd/adrctl extract-obligations --index /tmp/corpus-ingested.json --root testdata/corpus/adrs --json-out /tmp/corpus-candidates.json --min-confidence-report
python3 scripts/reuse_pilot_harness.py testdata/corpus/labels.json /tmp/corpus-candidates.json
```

Default CI always runs the synthetic reuse-pilot. The labeled corpus workflow is opt-in via `workflow_dispatch` or path filters (`.github/workflows/corpus-eval.yml`).

Harness output includes `recall_by_confidence` (and `precision_by_confidence`) so workshop-emit can publish band-level `kpi.harness.recall.*`.

## Analyzer coverage notes

- **Checkov** is implemented for AWS IaC patterns (`security.aws_sg`, `security.aws_network`, IAM/encryption/logging/backup configuration families, `configuration.authentication` password policy, `configuration.logging` retention/trails/object-lock, `infra.policy`). Issue/sandbox assess runs Checkov alongside OpenGrep and uses `--issue-surface` so unbound `manual-review` rows become `NOT_APPLICABLE` instead of forcing `INDETERMINATE`.
- **Still manual (by design):** catalog `manual_review` preferences; business-logic bounds; trust-boundary design; account root MFA/SCP process; org cost-allocation tag policies; shared SSH keypair fleet design. These stay `REVIEW_REQUIRED` in repo CI and `NOT_APPLICABLE` on `--issue-surface`.
- **Applicability (`applies_to`):** each generated requirement includes `applies_to.submission_kinds`, `analyzers`, and `path_globs` (extension globs). Assess filters non-applicable rows to `NOT_APPLICABLE` by applicability before verifier evaluation; pass `--assess-all` for full-catalog corpus CI. Share path scope with `assess --scope` / scanner `--scope` / PR changed paths. See `docs/architecture.md`.
- **Joern/CPG** remains deferred until at least **10 accepted ADRs** need cross-file/data-flow evidence OpenGrep cannot supply (`N=10`).

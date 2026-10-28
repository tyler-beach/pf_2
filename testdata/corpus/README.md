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

Merge packs into the CI default config with `python3 scripts/merge_opengrep_rules.py`.

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
go run ./cmd/adrctl scan-opengrep --root /tmp/issue-src --config rules/pathfinder-security.yml ...
go run ./cmd/adrctl assess-repo --root /tmp/issue-src --requirements testdata/corpus/requirements.json \
  --analyzer-evidence ... --analyzer-sarif ... --report /tmp/report.md --json-out /tmp/assessment.json
```

## Extract harness

```bash
go run ./cmd/adrctl ingest-adrs --root testdata/corpus/adrs --json-out /tmp/corpus-ingested.json
go run ./cmd/adrctl extract-obligations --index /tmp/corpus-ingested.json --root testdata/corpus/adrs --json-out /tmp/corpus-candidates.json --min-confidence-report
python3 scripts/reuse_pilot_harness.py testdata/corpus/labels.json /tmp/corpus-candidates.json
```

Default CI always runs the synthetic reuse-pilot. The labeled corpus workflow is opt-in via `workflow_dispatch` or path filters (`.github/workflows/corpus-eval.yml`).

Harness output includes `recall_by_confidence` (and `precision_by_confidence`) so workshop-emit can publish band-level `kpi.harness.recall.*`.

## Joern / Checkov deferral

Do **not** add Joern/CPG or Checkov/cfn-nag adapters until at least **10 accepted ADRs** need evidence those tools uniquely provide. Until then, AWS IaC-heavy obligations stay `manual_review` / `infra.policy`, and interpretation gaps use confidence-gated batch LLM assist + human review.

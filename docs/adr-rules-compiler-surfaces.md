# ADR → rules compiler: integration surfaces

Concrete Pathfinder contracts an **external ADR→rules compiler** must target. Runtime assessment stays deterministic (`runtime_llm_calls == 0`). The compiler authors reviewable artifacts and catalog/rules packs; it does not call models at assess/PR-scan time.

Related: [architecture.md](architecture.md), [obligation-review.md](obligation-review.md), [scaling-adr-design.md](scaling-adr-design.md), [specification-to-enforcement.md](specification-to-enforcement.md), [testdata/corpus/README.md](../testdata/corpus/README.md).

---

## Integration checklist (inputs → outputs → gates)

| Stage | Inputs | Outputs | Gates |
|---|---|---|---|
| Pattern catalog | Proposed `pattern_id`, verifier family | Row in `adr/patterns.json` (`id`, `title`, `verifier_types`, `inputs`, `applicability`, `blind_spots`, `fixture_refs`) | `patterns.Load` / `Validate`; digest via `ContentDigest`; approval uses `CompatibleVerifier(pattern_id, verifier.type)` |
| OpenGrep / Checkov rules | Semgrep-compatible YAML or Checkov config | Pack under `rules/*.yml`; merged default `rules/pathfinder-security.yml` | `python3 scripts/merge_opengrep_rules.py` (unique `- id:`); CI vulnerable/clean fixture scan |
| Binding catalog | Approved obligation + rule IDs + path scope | `adr/requirements.json` requirement with `verifier` (+ optional `applies_to`) | `adrctl validate --requirements`; analyzer-rule requires `analyzer`, non-empty `rule_ids`, `source_path_prefixes` |
| Extract | ADR Markdown + ingest index | Candidate index (`extract-obligations`) | Candidates never write requirements; `executable=false`, `approval_status=pending_review` |
| Optional LLM scaffold | tech_spec/mixed prose | Pending `extraction_method=llm` rows and/or assist `ProposalIndex` | Kind gate; HITL (`AssertExtractLLMHITL`); propose ≠ approve |
| Review → bind | `ReviewProposal` + candidates + requirements + patterns | Append-only `obligation-reviews.json` + `requirement_sha256` | Same-ADR, accepted ADR for blocking, pattern↔verifier compatibility, HTTPS `review_reference` |
| Evidence producers | Analyzer CLI SARIF/JSON | `analyzer.ScanResult` and/or `evidence.Bundle`/`Record` | Incomplete/ERROR ≠ empty PASS; digests + scanned paths required for analyzer-rule |
| Fixture proof | New rule IDs | Paired files under `testdata/analyzer-fixtures/{vulnerable,clean}/` (IaC under `iac/`) | CI: vulnerable fires (≥40 findings / ≥40 distinct rules + language set); clean has **0** findings; Checkov pins `CKV_AWS_24`/`25` |

---

## 1. `adr/patterns.json` — families and verifier types

**File:** `/workspace/adr/patterns.json`  
**Go:** `internal/patterns.Pattern`, `Catalog` (`schema_version: "1"`)

**Counts (current catalog):** **55** patterns.

| Coarse family (ID prefix) | Count | Role |
|---|---:|---|
| `configuration.*` | 22 | JSON keys/bounds + IaC/config baselines (many also allow `analyzer-rule`) |
| `security.*` | 22 | Source/SDK/OpenGrep (and some Checkov) signal families |
| `architecture.*` | 9 | Import/boundary and design constraints |
| `manual_review` | 1 | Human-only; cannot bind machine verifiers |
| `infra.*` | 1 | Checkov IaC policy (`infra.policy`) |

**Families are coarse by design:** OWASP/AWS themes collapse into shared IDs (e.g. `security.injection`, `security.aws_sg`, `configuration.authentication`). Fine-grained ADR themes live in corpus taxonomy (`testdata/corpus/taxonomy.yml`); harness labels map to these catalog pattern IDs. Many ADRs share one pattern; the compiler should **cluster** into existing IDs before proposing new ones.

**Union of `verifier_types` (5):**

| Type | Patterns allowing it (approx.) | Machine evidence |
|---|---:|---|
| `analyzer-rule` | 45 | OpenGrep or Checkov `ScanResult` / evidence envelope |
| `manual-review` | 38 | No automated PASS; `REVIEW_REQUIRED` (repo) / `NOT_APPLICABLE` (`--issue-surface`) |
| `json-required-keys` | 13 | Native JSON verifier |
| `json-number-range` | 6 | Native JSON bounds |
| `go-import-boundary` | 4 | Go AST import prefixes |

**Pattern record fields:** `id`, `title`, `verifier_types[]`, `inputs[]`, `applicability`, `blind_spots[]`, `fixture_refs[]`.

**Compiler rule:** choose a `pattern_id` whose `verifier_types` includes the verifier you will put on the requirement. Approval fails closed via `Catalog.CompatibleVerifier`.

---

## 2. Analyzer-rule bindings — `requirements.json` shape

**Compatibility catalog:** `/workspace/adr/requirements.json` (hand-maintained demo; 12 ADRs / 50 requirements today).  
**Corpus-scale catalog:** `/workspace/testdata/corpus/requirements.json` (generated: `scripts/generate_corpus_bindings.py`).  
**Go:** `internal/assessment.Catalog`, `ADR`, `Requirement`, `Verifier`, `AppliesTo`.

### Catalog shape

```json
{
  "schema_version": "1",
  "adrs": [{
    "id": "ADR-003",
    "title": "...",
    "version": 1,
    "status": "accepted",
    "requirements": [{
      "id": "REQ-003-01",
      "title": "...",
      "description": "...",
      "severity": "error",
      "verifier": { "...": "..." },
      "applies_to": {
        "submission_kinds": ["code_snippet"],
        "path_globs": ["**/*.go"],
        "analyzers": ["opengrep"]
      }
    }]
  }]
}
```

### `verifier` for `analyzer-rule` (required fields)

| Field | Purpose |
|---|---|
| `type` | `"analyzer-rule"` |
| `analyzer` | `"opengrep"` or `"checkov"` (must match evidence producer name) |
| `rule_ids` | Exact IDs as emitted in findings (OpenGrep: `rules.pathfinder.<id>`; Checkov: `CKV_*` / `CKV2_*`) |
| `source_path_prefixes` | Repo-relative path scope (e.g. `"."`, `"cmd/"`, `"internal/"`); required, non-empty |
| `source_extensions` | Optional extension filter (e.g. `".go"`, `".py"`) |

Validation (`Validate` in `assessment.go`): non-empty `analyzer`, unique non-empty `rule_ids`, repo-relative prefixes, extensions `.` + 1–11 alnum chars.

**Matching semantics:** finding `rule_id` ∈ `rule_ids` ∧ path under prefixes ∧ extension (if set) ∧ path ∈ scan inventory. Finding with matching rule but missing path / path outside inventory → `EVIDENCE_MISSING`. No matching findings + complete scan → **PASS**. Incomplete/ERROR scan → not treated as clean PASS.

### Other verifier shapes (non–analyzer-rule)

- `go-import-boundary`: `source_path_prefixes`, `forbidden_import_prefixes`
- `json-required-keys`: `file_path`, `required_json_keys`
- `json-number-range`: `file_path`, `numeric_json_bounds[{key,min,max}]`
- `manual-review`: no machine config

### `applies_to` (optional; defaults derived)

`submission_kinds`, `path_globs`, `analyzers`. Defaults: non-manual → `code_snippet`; path globs from prefixes/extensions/file_path; analyzers from `verifier.analyzer`. Assess filters non-applicable rows to `NOT_APPLICABLE` unless `--assess-all`.

**Demo catalog note:** most OpenGrep bindings use SARIF-style IDs `rules.pathfinder.*` (67 merged YAML rules; YAML `- id:` is `pathfinder.*` without the `rules.` prefix — adapters normalize).

---

## 3. OpenGrep rules layout and merge

**Directory:** `/workspace/rules/`

| Path | Role |
|---|---|
| `pathfinder-security-core.yml` | Legacy multilingual shell/eval pack (18 rules) |
| `owasp-injection.yml` | Injection sinks (8) |
| `owasp-crypto.yml` | Weak crypto (8) |
| `owasp-ssrf.yml` | SSRF fetches (6) |
| `owasp-secrets.yml` | Hardcoded secrets (6) |
| `aws-sdk-misuse.yml` | AWS SDK misuse (8) |
| `owasp-misc-promote.yml` | Hardening / supply-chain / logging / plugins (13) |
| **`pathfinder-security.yml`** | **Merged CI default (67 rules)** — do not hand-edit; regenerate |
| `checkov.yml` | Separate Checkov frameworks pin (not merged into OpenGrep) |

**Merge script:** `/workspace/scripts/merge_opengrep_rules.py`

```text
PACKS order → rules/pathfinder-security.yml
Fails on duplicate - id: values
```

After editing a pack: `python3 scripts/merge_opengrep_rules.py`.

**Runtime scan:** `adrctl scan-opengrep --config rules/pathfinder-security.yml` (PR/CI workflows). Checkov: `adrctl scan-checkov --config rules/checkov.yml`.

**Compiler outputs for a new OpenGrep rule:**

1. Add rule to the correct pack YAML (`- id: pathfinder....`).
2. Re-merge.
3. Bind as `rules.pathfinder....` in requirements (match adapter emission).
4. Add vulnerable + clean fixtures (below).
5. Optionally point `patterns.json` `fixture_refs` at the pack / fixture dirs.

---

## 4. Extract → review → binding (approval fields)

### Extract (`adrctl extract-obligations`)

**Go:** `internal/obligations.Candidate`, `Index`  
**Never** mutates `adr/requirements.json`.

Key candidate fields: `id`, `adr_id`, `statement`, `modality`, `kind`, `pattern_hint`, `source_path`, `source_sha256`, `source_line_start/end`, `approval_status` (`pending_review`), `enforcement_mode`, `executable=false`, `conditions`/`exceptions`/`ambiguity`, `confidence` / `confidence_p` / conformal set, optional `extraction_method` (`deterministic`|`llm`), `submission_kind`.

### Review proposal → registry

**Docs:** [obligation-review.md](obligation-review.md)  
**Go:** `ReviewProposal`, `ReviewRecord`, `Registry`, `CandidateSnapshot`

**Required on every recorded decision:**

| Field | Constraint |
|---|---|
| `candidate_id` | Must exist (or `superseded` only if history exists and candidate gone) |
| `decision` | `pending_review` \| `approved` \| `rejected` \| `superseded` |
| `enforcement_mode` | `blocking` \| `advisory` \| `review_required` |
| `reviewer` | Non-empty (≤256) |
| `reviewed_at` | RFC3339 |
| `review_reference` | HTTPS URL, no credentials |
| `rationale` | Non-empty (≤4000) |

**For `approved` (machine-checkable):**

| Field | Constraint |
|---|---|
| `pattern_id` | Required; must exist in `adr/patterns.json` (when catalog loaded) |
| `requirement_id` | Required unless `pattern_id == manual_review` |
| `requirement_sha256` | Auto-stamped from catalog binding snapshot; stale if requirement/ADR changes |
| Pattern ↔ verifier | `CompatibleVerifier(pattern_id, requirement.verifier.type)` |
| Same ADR | Requirement’s owning ADR ID == candidate `adr_id` |
| Blocking | Candidate ADR `status` must be `accepted`; requirement ADR must be `accepted` |

**`manual_review`:** may approve with `enforcement_mode=review_required` only; **no** `requirement_id`; cannot be blocking/advisory.

**Rejected / pending / superseded:** must **not** set `requirement_id` or `pattern_id`; enforcement must be `review_required`.

CLI: `record-obligation-review`, `validate-obligation-reviews --strict`, `review-init`.

---

## 5. Evidence envelope — what a new producer must emit

**Prefer canonical envelope** (evaluators match findings from bundles when present):

- **Schema:** `/workspace/schemas/evidence-v1.json`
- **Go:** `internal/evidence.Record`, `Bundle`, `Producer`, `Artifact`, `Finding`, `Provenance`, `Scope`
- **Adapter from ScanResult:** `evidence.FromScanResult`

### Per-record (`Record`) required conceptually

| Field | Notes |
|---|---|
| `schema_version` | `"1.0"` |
| `evidence_id` | Stable (fingerprint or hash) |
| `producer.analyzer` | Required; must align with binding `verifier.analyzer` |
| `producer.version` / digests | `ruleset_digest`, `config_digest`, `binary_digest` as `sha256:…` when known |
| `kind` | `scanner_finding` \| `analysis_limitation` \| `configuration_value` \| `syntax_fact` |
| `status` | `observed` \| `absent-within-declared-scope` \| `unsupported` \| `parse-error` \| `tool-error` \| `incomplete` |
| For findings | `finding.{id,rule_id}`, `artifact.path` (+ lines), `producer.rule_id` |
| `provenance` | `raw_artifact_sha256`, `extraction_method` |

### Bundle-level (run completeness)

`scan_completeness` / ScanResult `status`: `COMPLETE` \| `PARTIAL` \| `ERROR`  
Also: `input_sha256`, `raw_output_sha256`, `configuration_sha256`, `rule_set_sha256`, `binary_sha256`, `scanned_paths[]`, `errors[]`, `producers[]`.

**Producer invariant:** missing/incomplete evidence is **not** PASS. Empty findings are meaningful only when status is `COMPLETE` and scope was actually scanned.

**Legacy path still accepted:** `analyzer.ScanResult` JSON passed via `--analyzer-evidence` (normalized to envelope inside assess).

---

## 6. Existing LLM assist / extract-llm prompts

Batch-only scaffolding. **Never** auto-approve; assess/PR-scan stay zero-LLM.

### `extract-llm` (`internal/obligations/extract_llm.go`)

- Prompt version: `pathfinder-extract-llm-v1`
- Gate: `submission_kind` ∈ `tech_spec` \| `mixed` (or `--extract-llm-force`); env `PATHFINDER_EXTRACT_LLM=1`
- Asks model for JSON:

```text
{"candidates":[{
  "statement","modality","pattern_hint","section_heading",
  "conditions","exceptions","ambiguity","source_line_start","source_line_end"
}]}
```

- `modality`: `must|must_not|should|may|not_required`
- `pattern_hint`: known Pathfinder pattern id or `manual_review`
- Explicit: never mark approved/executable
- Mapped rows: `extraction_method=llm`, pending HITL; deterministic extract wins on dedupe

### `assist-obligations` (`internal/obligations/assist.go`)

- Prompt version: `pathfinder-assist-v1`
- System: JSON only with `normalized_statement`, `modality`, `suggested_pattern_id`, `rationale`; never approve/executable
- User prompt includes parent candidate id, ADR, source path/lines/SHA-256, confidence, escalation reasons, statement quote
- Output `Proposal`: those four fields + provenance hashes; `approval_status=pending_review`, `executable=false`
- Accept path: `accept-assist-proposal` still goes through normal review binding rules

---

## 7. Fixture / gate expectations for new rules

### OpenGrep (`.github/workflows/ci.yml`)

Fixtures: `testdata/analyzer-fixtures/vulnerable/*` and `clean/*` (language files at maxdepth 1; **not** `iac/`).

| Root | Expectation |
|---|---|
| **vulnerable** | `status==COMPLETE`; findings length **≥ 40**; **≥ 40** distinct `rule_id`s; must include core multilingual + sample OWASP/AWS IDs listed in CI; extensions set must equal `{bash,c,cpp,cs,dart,ex,go,java,js,kt,lua,ml,php,py,rb,rs,scala,sh,swift,ts,vb}` |
| **clean** | `status==COMPLETE` and **`findings.length == 0`** |

Config under test: merged `rules/pathfinder-security.yml`.

### Checkov

Fixtures: `testdata/analyzer-fixtures/{vulnerable,clean}/iac/` + `rules/checkov.yml`.

| Root | Expectation |
|---|---|
| **vulnerable** | COMPLETE; findings include `CKV_AWS_24` and `CKV_AWS_25` |
| **clean** | COMPLETE; those rule IDs **absent** |

### Compiler checklist for a new OpenGrep rule

1. Vulnerable fixture that **must** fire the rule (language-appropriate file under `vulnerable/`).
2. Clean twin under `clean/` that **must not** fire (keeps global clean-zero gate).
3. If introducing a new language extension, extend the CI extension-set assertion.
4. Bind exact emitted `rule_id` in requirements.
5. Pattern `fixture_refs` should cite the pack and/or fixture dirs.

### Corpus / binding regen (scale path)

```bash
python3 scripts/generate_security_corpus.py
python3 scripts/generate_corpus_bindings.py
go run ./cmd/adrctl validate --requirements testdata/corpus/requirements.json
```

---

## Quick path map

```text
ADR Markdown
  → adrctl ingest-adrs
  → adrctl extract-obligations [--extract-llm]
  → (optional) adrctl assist-obligations
  → human ReviewProposal → adrctl record-obligation-review
  → requirements row in adr/requirements.json (or corpus generator)
  → rules pack + merge_opengrep_rules.py
  → vulnerable/clean fixtures
  → adrctl scan-opengrep|scan-checkov → ScanResult / evidence.Bundle
  → adrctl assess-repo --analyzer-evidence …
```

**Do not:** emit executable candidates from the LLM; invent PASS from incomplete scans; bind `manual_review` to `analyzer-rule`; skip merge after pack edits; use YAML `- id:` without confirming SARIF/`rules.` prefix used in `rule_ids`.

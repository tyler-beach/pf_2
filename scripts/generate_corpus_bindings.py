#!/usr/bin/env python3
"""Generate testdata/corpus/requirements.json from the labeled OWASP/AWS corpus.

Maps catalog_pattern_id → OpenGrep rule families or manual-review so ADR Assessment
can evaluate issue snippets / sandboxes against the full 200-ADR pack.

  python3 scripts/generate_corpus_bindings.py
"""

from __future__ import annotations

import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CORPUS = ROOT / "testdata" / "corpus"
LABELS = CORPUS / "labels.json"
INDEX = CORPUS / "corpus_index.json"
OUT = CORPUS / "requirements.json"

# Rule IDs as stored in analyzer evidence / requirements (rules.<id>).
R = lambda *ids: [f"rules.{i}" for i in ids]

PATTERN_BINDINGS: dict[str, dict] = {
    "security.injection": {
        "rule_ids": R(
            "pathfinder.owasp-sql-string-concat-python",
            "pathfinder.owasp-sql-format-go",
            "pathfinder.owasp-child-process-exec-node",
            "pathfinder.owasp-os-system-python",
            "pathfinder.owasp-subprocess-shell-python",
            "pathfinder.owasp-raw-query-java",
            "pathfinder.go-shell-command",
            "pathfinder.python-dynamic-eval",
            "pathfinder.no-dynamic-eval",
            "pathfinder.java-runtime-exec",
        ),
        "extensions": [".py", ".go", ".js", ".ts", ".jsx", ".tsx", ".java"],
    },
    "security.crypto": {
        "rule_ids": R(
            "pathfinder.owasp-md5-python",
            "pathfinder.owasp-sha1-python",
            "pathfinder.owasp-md5-go",
            "pathfinder.owasp-sha1-go",
            "pathfinder.owasp-des-java",
            "pathfinder.owasp-insecure-tls-python",
            "pathfinder.owasp-insecure-skip-verify-go",
        ),
        "extensions": [".py", ".go", ".java"],
    },
    "security.secrets": {
        "rule_ids": R(
            "pathfinder.owasp-hardcoded-aws-key",
            "pathfinder.owasp-hardcoded-password-assign-python",
            "pathfinder.owasp-hardcoded-api-key-javascript",
            "pathfinder.owasp-hardcoded-bearer-go",
            "pathfinder.owasp-private-key-literal",
            "pathfinder.owasp-slack-token",
            "pathfinder.aws-hardcoded-secret-env-python",
            "pathfinder.aws-setenv-secret-go",
        ),
        "extensions": [".py", ".go", ".js", ".ts", ".java"],
    },
    "security.aws_secrets": {
        "rule_ids": R(
            "pathfinder.owasp-hardcoded-aws-key",
            "pathfinder.aws-hardcoded-secret-env-python",
            "pathfinder.aws-setenv-secret-go",
            "pathfinder.owasp-private-key-literal",
        ),
        "extensions": [".py", ".go", ".js", ".ts"],
    },
    "security.ssrf": {
        "rule_ids": R(
            "pathfinder.owasp-ssrf-http-get-python",
            "pathfinder.owasp-ssrf-urlopen-python",
            "pathfinder.owasp-ssrf-http-get-go",
            "pathfinder.owasp-ssrf-fetch-javascript",
            "pathfinder.owasp-ssrf-axios-javascript",
            "pathfinder.owasp-ssrf-httpurl-java",
            "pathfinder.aws-imds-v1-token-skip",
        ),
        "extensions": [".py", ".go", ".js", ".ts", ".java"],
    },
    "security.integrity": {
        "rule_ids": R(
            "pathfinder.owasp-pickle-loads-python",
            "pathfinder.owasp-yaml-unsafe-load-python",
        ),
        "extensions": [".py"],
    },
    "security.aws_s3": {
        "rule_ids": R(
            "pathfinder.aws-s3-public-read-acl-python",
            "pathfinder.aws-s3-public-read-acl-go",
            "pathfinder.aws-s3-public-read-write-python",
            "pathfinder.aws-put-bucket-acl-public-python",
        ),
        "extensions": [".py", ".go"],
    },
    "security.aws_encryption": {
        "rule_ids": R(
            "pathfinder.aws-disable-s3-encryption-python",
            "pathfinder.owasp-insecure-tls-python",
            "pathfinder.owasp-insecure-skip-verify-go",
        ),
        "extensions": [".py", ".go"],
    },
    "security.aws_compute": {
        "rule_ids": R("pathfinder.aws-imds-v1-token-skip"),
        "extensions": [".py", ".go"],
    },
    "architecture.access_control": {
        "rule_ids": R(
            "pathfinder.aws-s3-public-read-acl-python",
            "pathfinder.aws-s3-public-read-acl-go",
            "pathfinder.owasp-hardcoded-aws-key",
        ),
        "extensions": [".py", ".go", ".js", ".ts"],
    },
    "configuration.ssrf": {
        "rule_ids": R(
            "pathfinder.owasp-ssrf-http-get-python",
            "pathfinder.owasp-ssrf-http-get-go",
            "pathfinder.aws-imds-v1-token-skip",
        ),
        "extensions": [".py", ".go", ".js", ".ts"],
    },
    "configuration.aws_secrets": {
        "rule_ids": R(
            "pathfinder.owasp-hardcoded-aws-key",
            "pathfinder.aws-hardcoded-secret-env-python",
            "pathfinder.aws-setenv-secret-go",
        ),
        "extensions": [".py", ".go"],
    },
    "configuration.aws_s3": {
        "rule_ids": R(
            "pathfinder.aws-s3-public-read-acl-python",
            "pathfinder.aws-s3-public-read-write-python",
        ),
        "extensions": [".py"],
    },
    "configuration.tls_baseline": {
        "rule_ids": R(
            "pathfinder.owasp-insecure-tls-python",
            "pathfinder.owasp-insecure-skip-verify-go",
        ),
        "extensions": [".py", ".go"],
    },
    "configuration.crypto_baseline": {
        "rule_ids": R(
            "pathfinder.owasp-md5-python",
            "pathfinder.owasp-md5-go",
            "pathfinder.owasp-sha1-python",
            "pathfinder.owasp-sha1-go",
        ),
        "extensions": [".py", ".go"],
    },
    "security.source_pattern": {
        "rule_ids": R(
            "pathfinder.go-shell-command",
            "pathfinder.python-dynamic-eval",
            "pathfinder.no-dynamic-eval",
            "pathfinder.owasp-os-system-python",
            "pathfinder.owasp-hardcoded-aws-key",
        ),
        "extensions": [".py", ".go", ".js", ".ts", ".java", ".php", ".rb"],
    },
    "security.access_control": {
        "rule_ids": R(
            "pathfinder.aws-s3-public-read-acl-python",
            "pathfinder.aws-s3-public-read-acl-go",
            "pathfinder.owasp-hardcoded-aws-key",
        ),
        "extensions": [".py", ".go", ".js", ".ts"],
    },
    "security.authentication": {
        "rule_ids": R(
            "pathfinder.owasp-hardcoded-password-assign-python",
            "pathfinder.owasp-hardcoded-api-key-javascript",
            "pathfinder.owasp-hardcoded-bearer-go",
        ),
        "extensions": [".py", ".go", ".js", ".ts"],
    },
    "security.misconfiguration": {
        "rule_ids": R(
            "pathfinder.owasp-insecure-tls-python",
            "pathfinder.owasp-insecure-skip-verify-go",
            "pathfinder.aws-imds-v1-token-skip",
        ),
        "extensions": [".py", ".go"],
    },
    "security.secure_design": {
        "rule_ids": R(
            "pathfinder.owasp-child-process-exec-node",
            "pathfinder.go-shell-command",
            "pathfinder.owasp-os-system-python",
        ),
        "extensions": [".py", ".go", ".js", ".ts"],
    },
    "security.supply_chain": {
        "manual": True,
    },
    "security.logging": {
        "manual": True,
    },
    "security.aws_iam": {"manual": True},
    "security.aws_network": {"manual": True},
    "security.aws_logging": {"manual": True},
    "security.aws_sg": {"manual": True},
    "security.aws_root": {"manual": True},
    "security.aws_backup": {"manual": True},
}


def binding_for(catalog_pattern: str, extract_pattern: str) -> dict:
    if catalog_pattern in PATTERN_BINDINGS:
        return PATTERN_BINDINGS[catalog_pattern]
    if extract_pattern == "security.source_pattern":
        return PATTERN_BINDINGS["security.source_pattern"]
    if extract_pattern == "configuration.bounds_or_required_values":
        return {"manual": True}
    if extract_pattern == "architecture.dependency_or_boundary":
        return {"manual": True}
    return {"manual": True}


def main() -> int:
    labels = json.loads(LABELS.read_text(encoding="utf-8"))
    index = {e["id"]: e for e in json.loads(INDEX.read_text(encoding="utf-8"))["adrs"]}
    adrs = []
    analyzer = 0
    manual = 0
    for row in labels["expected_candidates"]:
        adr_id = row["adr_id"]
        meta = index.get(adr_id, {})
        catalog_pattern = row.get("catalog_pattern_id") or row["pattern_id"]
        extract_pattern = row["pattern_id"]
        bind = binding_for(catalog_pattern, extract_pattern)
        title = meta.get("title") or adr_id
        req_id = f"REQ-{adr_id.replace('ADR-', '')}-01"
        severity = "error" if row["modality"] in ("must", "must_not") else "warning"
        if bind.get("manual"):
            manual += 1
            verifier = {"type": "manual-review"}
            description = (
                f"Corpus obligation from {adr_id}: {row['statement_contains']}. "
                "Requires human review until an automated verifier is bound."
            )
        else:
            analyzer += 1
            verifier = {
                "type": "analyzer-rule",
                "analyzer": "opengrep",
                "rule_ids": bind["rule_ids"],
                "source_path_prefixes": ["."],
                "source_extensions": bind["extensions"],
            }
            description = (
                f"Corpus obligation from {adr_id}: {row['statement_contains']}. "
                f"Checked via OpenGrep rules for pattern {catalog_pattern}."
            )
        adrs.append(
            {
                "id": adr_id,
                "title": title,
                "version": 1,
                "status": "accepted",
                "requirements": [
                    {
                        "id": req_id,
                        "title": title[:120],
                        "description": description,
                        "severity": severity,
                        "verifier": verifier,
                    }
                ],
            }
        )

    catalog = {
        "schema_version": "1",
        "adrs": adrs,
        "meta": {
            "source": "testdata/corpus/labels.json",
            "generator": "scripts/generate_corpus_bindings.py",
            "analyzer_backed": analyzer,
            "manual_review": manual,
            "corpus_size": len(adrs),
        },
    }
    # Strip meta for ValidateCatalog if unknown fields break — check LoadCatalog
    OUT.write_text(json.dumps({"schema_version": "1", "adrs": adrs}, indent=2) + "\n", encoding="utf-8")
    meta_path = CORPUS / "requirements-meta.json"
    meta_path.write_text(json.dumps(catalog["meta"], indent=2) + "\n", encoding="utf-8")
    print(f"Wrote {len(adrs)} ADRs to {OUT} (analyzer={analyzer}, manual={manual})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

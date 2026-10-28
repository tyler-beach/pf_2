#!/usr/bin/env python3
"""Generate testdata/corpus/requirements.json from the labeled OWASP/AWS corpus.

Maps catalog_pattern_id → OpenGrep / Checkov / manual-review so ADR Assessment
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

# Rule IDs as stored in analyzer evidence / requirements (rules.<id> for OpenGrep).
R = lambda *ids: [f"rules.{i}" for i in ids]
# Checkov SARIF emits bare CKV_* identifiers.
C = lambda *ids: list(ids)

IAC_EXTENSIONS = [".tf", ".tfvars", ".yml", ".yaml", ".json", ".template"]

PATTERN_BINDINGS: dict[str, dict] = {
    "security.injection": {
        "analyzer": "opengrep",
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
        "analyzer": "opengrep",
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
        "analyzer": "opengrep",
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
        "analyzer": "opengrep",
        "rule_ids": R(
            "pathfinder.owasp-hardcoded-aws-key",
            "pathfinder.aws-hardcoded-secret-env-python",
            "pathfinder.aws-setenv-secret-go",
            "pathfinder.owasp-private-key-literal",
        ),
        "extensions": [".py", ".go", ".js", ".ts"],
    },
    "security.ssrf": {
        "analyzer": "opengrep",
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
        "analyzer": "opengrep",
        "rule_ids": R(
            "pathfinder.owasp-pickle-loads-python",
            "pathfinder.owasp-yaml-unsafe-load-python",
        ),
        "extensions": [".py"],
    },
    "security.aws_s3": {
        "analyzer": "opengrep",
        "rule_ids": R(
            "pathfinder.aws-s3-public-read-acl-python",
            "pathfinder.aws-s3-public-read-acl-go",
            "pathfinder.aws-s3-public-read-write-python",
            "pathfinder.aws-put-bucket-acl-public-python",
        ),
        "extensions": [".py", ".go"],
    },
    "security.aws_encryption": {
        "analyzer": "opengrep",
        "rule_ids": R(
            "pathfinder.aws-disable-s3-encryption-python",
            "pathfinder.owasp-insecure-tls-python",
            "pathfinder.owasp-insecure-skip-verify-go",
        ),
        "extensions": [".py", ".go"],
    },
    "security.aws_compute": {
        "analyzer": "opengrep",
        "rule_ids": R("pathfinder.aws-imds-v1-token-skip"),
        "extensions": [".py", ".go"],
    },
    "architecture.access_control": {
        "analyzer": "opengrep",
        "rule_ids": R(
            "pathfinder.aws-s3-public-read-acl-python",
            "pathfinder.aws-s3-public-read-acl-go",
            "pathfinder.owasp-hardcoded-aws-key",
        ),
        "extensions": [".py", ".go", ".js", ".ts"],
    },
    "configuration.ssrf": {
        "analyzer": "opengrep",
        "rule_ids": R(
            "pathfinder.owasp-ssrf-http-get-python",
            "pathfinder.owasp-ssrf-http-get-go",
            "pathfinder.aws-imds-v1-token-skip",
        ),
        "extensions": [".py", ".go", ".js", ".ts"],
    },
    "configuration.aws_secrets": {
        "analyzer": "opengrep",
        "rule_ids": R(
            "pathfinder.owasp-hardcoded-aws-key",
            "pathfinder.aws-hardcoded-secret-env-python",
            "pathfinder.aws-setenv-secret-go",
        ),
        "extensions": [".py", ".go"],
    },
    "configuration.aws_s3": {
        "analyzer": "opengrep",
        "rule_ids": R(
            "pathfinder.aws-s3-public-read-acl-python",
            "pathfinder.aws-s3-public-read-write-python",
        ),
        "extensions": [".py"],
    },
    "configuration.tls_baseline": {
        "analyzer": "opengrep",
        "rule_ids": R(
            "pathfinder.owasp-insecure-tls-python",
            "pathfinder.owasp-insecure-skip-verify-go",
        ),
        "extensions": [".py", ".go"],
    },
    "configuration.crypto_baseline": {
        "analyzer": "opengrep",
        "rule_ids": R(
            "pathfinder.owasp-md5-python",
            "pathfinder.owasp-md5-go",
            "pathfinder.owasp-sha1-python",
            "pathfinder.owasp-sha1-go",
        ),
        "extensions": [".py", ".go"],
    },
    "security.source_pattern": {
        "analyzer": "opengrep",
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
        "analyzer": "opengrep",
        "rule_ids": R(
            "pathfinder.aws-s3-public-read-acl-python",
            "pathfinder.aws-s3-public-read-acl-go",
            "pathfinder.owasp-hardcoded-aws-key",
        ),
        "extensions": [".py", ".go", ".js", ".ts"],
    },
    "security.authentication": {
        "analyzer": "opengrep",
        "rule_ids": R(
            "pathfinder.owasp-hardcoded-password-assign-python",
            "pathfinder.owasp-hardcoded-api-key-javascript",
            "pathfinder.owasp-hardcoded-bearer-go",
        ),
        "extensions": [".py", ".go", ".js", ".ts"],
    },
    "security.misconfiguration": {
        "analyzer": "opengrep",
        "rule_ids": R(
            "pathfinder.owasp-insecure-tls-python",
            "pathfinder.owasp-insecure-skip-verify-go",
            "pathfinder.aws-imds-v1-token-skip",
        ),
        "extensions": [".py", ".go"],
    },
    "security.secure_design": {
        "analyzer": "opengrep",
        "rule_ids": R(
            "pathfinder.owasp-child-process-exec-node",
            "pathfinder.go-shell-command",
            "pathfinder.owasp-os-system-python",
        ),
        "extensions": [".py", ".go", ".js", ".ts"],
    },
    # Promoted from manual-review: source/config scanners.
    "configuration.hardening": {
        "analyzer": "opengrep",
        "rule_ids": R(
            "pathfinder.owasp-debug-mode-python",
            "pathfinder.owasp-debug-mode-javascript",
            "pathfinder.owasp-hardcoded-password-assign-python",
        ),
        "extensions": [".py", ".js", ".ts", ".jsx", ".tsx"],
    },
    "security.supply_chain": {
        "analyzer": "opengrep",
        "rule_ids": R(
            "pathfinder.owasp-git-dependency-npm",
            "pathfinder.owasp-git-dependency-gomod",
        ),
        "extensions": [".json", ".mod", ".txt", ".go"],
    },
    "configuration.supply_chain": {
        "analyzer": "opengrep",
        "rule_ids": R(
            "pathfinder.owasp-git-dependency-npm",
            "pathfinder.owasp-git-dependency-gomod",
        ),
        "extensions": [".json", ".mod", ".txt", ".go"],
    },
    "architecture.supply_chain": {
        "analyzer": "opengrep",
        "rule_ids": R(
            "pathfinder.owasp-git-dependency-npm",
            "pathfinder.owasp-git-dependency-gomod",
        ),
        "extensions": [".json", ".mod", ".txt", ".go"],
    },
    "security.logging": {
        "analyzer": "opengrep",
        "rule_ids": R(
            "pathfinder.owasp-log-secret-python",
            "pathfinder.owasp-log-secret-javascript",
            "pathfinder.aws-cloudtrail-stop-logging",
        ),
        "extensions": [".py", ".js", ".ts"],
    },
    "security.aws_iam": {
        "analyzer": "opengrep",
        "rule_ids": R(
            "pathfinder.aws-iam-create-access-key",
            "pathfinder.owasp-hardcoded-aws-key",
        ),
        "extensions": [".py", ".go", ".js", ".ts"],
    },
    "security.aws_logging": {
        "analyzer": "opengrep",
        "rule_ids": R("pathfinder.aws-cloudtrail-stop-logging"),
        "extensions": [".py"],
    },
    # Checkov IaC bindings (true infrastructure assessment).
    "security.aws_network": {
        "analyzer": "checkov",
        "rule_ids": C("CKV_AWS_24", "CKV_AWS_23", "CKV2_AWS_11", "CKV_AWS_130"),
        "extensions": IAC_EXTENSIONS,
    },
    "configuration.aws_network": {
        "analyzer": "checkov",
        "rule_ids": C("CKV2_AWS_11", "CKV_AWS_130", "CKV_AWS_24"),
        "extensions": IAC_EXTENSIONS,
    },
    "architecture.aws_network": {
        "analyzer": "checkov",
        "rule_ids": C("CKV_AWS_130", "CKV2_AWS_11", "CKV_AWS_24"),
        "extensions": IAC_EXTENSIONS,
    },
    "security.aws_sg": {
        "analyzer": "checkov",
        "rule_ids": C("CKV_AWS_24", "CKV_AWS_25", "CKV_AWS_23", "CKV_AWS_277"),
        "extensions": IAC_EXTENSIONS,
    },
    "configuration.aws_sg": {
        "analyzer": "checkov",
        "rule_ids": C("CKV_AWS_23", "CKV_AWS_24", "CKV_AWS_25"),
        "extensions": IAC_EXTENSIONS,
    },
    "architecture.aws_sg": {
        "analyzer": "checkov",
        "rule_ids": C("CKV_AWS_24", "CKV_AWS_25", "CKV_AWS_23"),
        "extensions": IAC_EXTENSIONS,
    },
    "architecture.aws_iam": {
        "analyzer": "checkov",
        "rule_ids": C("CKV_AWS_40", "CKV_AWS_1", "CKV_AWS_49", "CKV_AWS_63"),
        "extensions": IAC_EXTENSIONS,
    },
    "configuration.aws_iam": {
        "analyzer": "checkov",
        "rule_ids": C("CKV_AWS_40", "CKV_AWS_1", "CKV_AWS_63"),
        "extensions": IAC_EXTENSIONS,
    },
    "configuration.aws_encryption": {
        "analyzer": "checkov",
        "rule_ids": C("CKV_AWS_3", "CKV_AWS_8", "CKV_AWS_16", "CKV_AWS_145", "CKV_AWS_35"),
        "extensions": IAC_EXTENSIONS,
    },
    "configuration.aws_logging": {
        "analyzer": "checkov",
        "rule_ids": C("CKV_AWS_67", "CKV_AWS_36", "CKV_AWS_66", "CKV_AWS_338"),
        "extensions": IAC_EXTENSIONS,
    },
    "configuration.aws_backup": {
        "analyzer": "checkov",
        "rule_ids": C("CKV_AWS_166", "CKV_AWS_16"),
        "extensions": IAC_EXTENSIONS,
    },
    "security.aws_backup": {
        "analyzer": "checkov",
        "rule_ids": C("CKV_AWS_166", "CKV_AWS_16"),
        "extensions": IAC_EXTENSIONS,
    },
    "configuration.aws_compute": {
        "analyzer": "checkov",
        "rule_ids": C("CKV_AWS_79"),
        "extensions": IAC_EXTENSIONS,
    },
    "infra.policy": {
        "analyzer": "checkov",
        "rule_ids": C(
            "CKV_AWS_24",
            "CKV_AWS_25",
            "CKV_AWS_40",
            "CKV_AWS_67",
            "CKV_AWS_79",
            "CKV2_AWS_11",
        ),
        "extensions": IAC_EXTENSIONS,
    },
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
    analyzer_counts: dict[str, int] = {}
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
            name = bind.get("analyzer", "opengrep")
            analyzer_counts[name] = analyzer_counts.get(name, 0) + 1
            verifier = {
                "type": "analyzer-rule",
                "analyzer": name,
                "rule_ids": bind["rule_ids"],
                "source_path_prefixes": ["."],
                "source_extensions": bind["extensions"],
            }
            tool = "OpenGrep" if name == "opengrep" else "Checkov"
            description = (
                f"Corpus obligation from {adr_id}: {row['statement_contains']}. "
                f"Checked via {tool} rules for pattern {catalog_pattern}."
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

    meta = {
        "source": "testdata/corpus/labels.json",
        "generator": "scripts/generate_corpus_bindings.py",
        "analyzer_backed": sum(analyzer_counts.values()),
        "analyzers": analyzer_counts,
        "manual_review": manual,
        "corpus_size": len(adrs),
    }
    OUT.write_text(json.dumps({"schema_version": "1", "adrs": adrs}, indent=2) + "\n", encoding="utf-8")
    meta_path = CORPUS / "requirements-meta.json"
    meta_path.write_text(json.dumps(meta, indent=2) + "\n", encoding="utf-8")
    print(
        f"Wrote {len(adrs)} ADRs to {OUT} "
        f"(analyzer={meta['analyzer_backed']} {analyzer_counts}, manual={manual})"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

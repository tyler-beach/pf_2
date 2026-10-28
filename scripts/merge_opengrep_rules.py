#!/usr/bin/env python3
"""Merge rules/packs into rules/pathfinder-security.yml for CI default config.

Legacy shell/eval rules stay first; OWASP/AWS packs append. Run after editing packs:

  python3 scripts/merge_opengrep_rules.py
"""

from __future__ import annotations

import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
RULES = ROOT / "rules"
OUT = RULES / "pathfinder-security.yml"
PACKS = [
    RULES / "pathfinder-security-core.yml",
    RULES / "owasp-injection.yml",
    RULES / "owasp-crypto.yml",
    RULES / "owasp-ssrf.yml",
    RULES / "owasp-secrets.yml",
    RULES / "aws-sdk-misuse.yml",
    RULES / "owasp-misc-promote.yml",
]


def extract_rules_block(text: str) -> str:
    text = text.strip() + "\n"
    if not text.startswith("rules:"):
        raise SystemExit(f"expected rules: root in pack content starting {text[:40]!r}")
    body = text[len("rules:") :].lstrip("\n")
    return body


def main() -> int:
    core = RULES / "pathfinder-security-core.yml"
    if not core.exists():
        # Bootstrap core from current pathfinder-security.yml once.
        current = OUT.read_text(encoding="utf-8")
        # Keep only original pathfinder.* shell/eval style ids (no owasp/aws packs).
        # If already merged, prefer existing core.
        core.write_text(current if "pathfinder.owasp-" not in current else current, encoding="utf-8")
        print(f"Bootstrapped {core} from {OUT}")

    chunks: list[str] = []
    ids: list[str] = []
    for path in PACKS:
        if not path.exists():
            raise SystemExit(f"missing pack {path}")
        body = extract_rules_block(path.read_text(encoding="utf-8"))
        chunks.append(f"# --- from {path.name} ---\n{body.rstrip()}\n")
        ids.extend(re.findall(r"^\s*- id:\s*(\S+)", body, flags=re.M))

    merged = "rules:\n" + "\n".join(chunks)
    if len(ids) != len(set(ids)):
        raise SystemExit(f"duplicate rule ids: {sorted({i for i in ids if ids.count(i) > 1})}")
    OUT.write_text(merged, encoding="utf-8")
    print(f"Wrote {len(ids)} rules to {OUT}")
    for rule_id in ids:
        print(f"  - {rule_id}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

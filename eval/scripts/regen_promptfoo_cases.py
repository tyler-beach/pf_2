#!/usr/bin/env python3
"""Regenerate eval/promptfoo/cases.json from a real obligation extract.

Usage:
  python3 eval/scripts/regen_promptfoo_cases.py /path/to/candidates.json [max_cases]
"""

from __future__ import annotations

import json
import sys
from pathlib import Path


def main() -> int:
    if len(sys.argv) < 2:
        print(__doc__.strip(), file=sys.stderr)
        return 2
    candidates_path = Path(sys.argv[1])
    max_cases = int(sys.argv[2]) if len(sys.argv) > 2 else 8
    payload = json.loads(candidates_path.read_text(encoding="utf-8"))
    low = [
        c
        for c in payload.get("candidates", [])
        if str(c.get("confidence", "")).lower() == "low"
    ]
    cases = []
    for c in low[:max_cases]:
        cases.append(
            {
                "vars": {
                    "candidate_id": c.get("id", ""),
                    "adr_id": c.get("adr_id", ""),
                    "source_path": c.get("source_path", ""),
                    "source_line_start": int(c.get("source_line_start") or 0),
                    "source_line_end": int(c.get("source_line_end") or 0),
                    "source_sha256": c.get("source_sha256", ""),
                    "confidence": c.get("confidence", "low"),
                    "escalation_reasons": ",".join(c.get("escalation_reasons") or []),
                    "statement": c.get("statement", ""),
                }
            }
        )
    if not cases:
        print("no low-confidence candidates found", file=sys.stderr)
        return 1
    out = Path(__file__).resolve().parents[1] / "promptfoo" / "cases.json"
    out.write_text(json.dumps(cases, indent=2) + "\n", encoding="utf-8")
    print(f"wrote {len(cases)} case(s) to {out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

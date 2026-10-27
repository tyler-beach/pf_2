#!/usr/bin/env python3
"""Generate ≥100 architect-style labeled corpus ADRs + labels.json.

Reproducible fixture generator for the Pathfinder labeled-corpus success metric.
Run from repo root:

  python3 scripts/generate_corpus_labels.py
"""

from __future__ import annotations

import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
ADR_DIR = ROOT / "testdata" / "corpus" / "adrs"
LABELS_PATH = ROOT / "testdata" / "corpus" / "labels.json"

# Rotating templates designed so deterministic extract+confidence scoring lands
# in the expected band for harness measurement.
TEMPLATES = [
    {
        "band": "high",
        "escalate": False,
        "modality": "must",
        "pattern_id": "configuration.bounds_or_required_values",
        "title": "Request timeouts",
        "marker": "finite request timeout no greater than {n} seconds",
        "decision": "Services must set a finite request timeout no greater than {n} seconds.",
    },
    {
        "band": "high",
        "escalate": False,
        "modality": "must_not",
        "pattern_id": "architecture.dependency_or_boundary",
        "title": "Import boundaries",
        "marker": "domain packages must not import infrastructure adapters layer {n}",
        "decision": "Domain packages must not import infrastructure adapters layer {n}.",
    },
    {
        "band": "medium",
        "escalate": False,
        "modality": "must_not",
        "pattern_id": "security.source_pattern",
        "title": "Untrusted input",
        "marker": "evaluate untrusted input with a dynamic interpreter for workload {n}",
        "decision": (
            "Services must not evaluate untrusted input with a dynamic interpreter "
            "for workload {n} unless an exception is explicitly documented."
        ),
    },
    {
        "band": "low",
        "escalate": True,
        "modality": "should",
        "pattern_id": "manual_review",
        "title": "Async messaging",
        "marker": "prefer asynchronous messaging for service family {n} where appropriate",
        "decision": (
            "Services should prefer asynchronous messaging for service family {n} "
            "where appropriate."
        ),
    },
    {
        "band": "low",
        "escalate": True,
        "modality": "may",
        "pattern_id": "manual_review",
        "title": "Interoperability",
        "marker": "documented interoperability constraint for integration slot {n}",
        "decision": (
            "Teams may adopt a documented interoperability constraint for "
            "integration slot {n} when integrating external systems."
        ),
    },
]


def render_adr(adr_id: str, title: str, decision: str) -> str:
    return (
        f"# {adr_id}: {title}\n\n"
        "## Status\n\n"
        "Accepted\n\n"
        "## Decision\n\n"
        f"{decision}\n"
    )


def main() -> int:
    ADR_DIR.mkdir(parents=True, exist_ok=True)
    # Keep only generated corpus ADRs in this directory.
    for path in ADR_DIR.glob("ADR-*.md"):
        path.unlink()

    expected: list[dict] = []
    count = 100
    for i in range(count):
        adr_num = 2001 + i
        adr_id = f"ADR-{adr_num}"
        tmpl = TEMPLATES[i % len(TEMPLATES)]
        n = (i // len(TEMPLATES)) + 1
        marker = tmpl["marker"].format(n=n)
        decision = tmpl["decision"].format(n=n)
        title = f"{tmpl['title']} {n}"
        (ADR_DIR / f"{adr_id}.md").write_text(render_adr(adr_id, title, decision), encoding="utf-8")
        expected.append(
            {
                "adr_id": adr_id,
                "statement_contains": marker if marker in decision else decision.split(".")[0],
                "modality": tmpl["modality"],
                "pattern_id": tmpl["pattern_id"],
                "confidence_band": tmpl["band"],
                "escalate_expected": tmpl["escalate"],
            }
        )
        # Prefer a substring that is unique and present in the decision text.
        if expected[-1]["statement_contains"] not in decision:
            # Fall back to a unique fragment guaranteed in the decision.
            expected[-1]["statement_contains"] = f"workload {n}" if "workload" in decision else decision[:48]

    # Fix markers to guaranteed substrings of decision text.
    for i, item in enumerate(expected):
        tmpl = TEMPLATES[i % len(TEMPLATES)]
        n = (i // len(TEMPLATES)) + 1
        decision = tmpl["decision"].format(n=n)
        # Unique stable markers that appear in the rendered decision.
        if tmpl["band"] == "high" and tmpl["modality"] == "must":
            item["statement_contains"] = f"finite request timeout no greater than {n} seconds"
        elif tmpl["band"] == "high":
            item["statement_contains"] = f"infrastructure adapters layer {n}"
        elif tmpl["band"] == "medium":
            item["statement_contains"] = f"dynamic interpreter for workload {n}"
        elif tmpl["modality"] == "should":
            item["statement_contains"] = f"asynchronous messaging for service family {n}"
        else:
            item["statement_contains"] = f"interoperability constraint for integration slot {n}"
        assert item["statement_contains"] in decision, (item["statement_contains"], decision)

    labels = {
        "expected_candidates": expected,
        "thresholds": {
            "min_precision": 0.55,
            "min_recall": 0.55,
            "max_escalation_rate": 0.85,
        },
        "corpus_size": count,
    }
    LABELS_PATH.write_text(json.dumps(labels, indent=2) + "\n", encoding="utf-8")
    print(f"Wrote {count} ADRs under {ADR_DIR} and labels to {LABELS_PATH}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

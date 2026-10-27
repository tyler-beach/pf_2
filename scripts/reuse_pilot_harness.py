#!/usr/bin/env python3
"""Deterministic ADR reuse / corpus harness.

Measures extraction precision/recall against labeled expectations, pattern-hint
reuse, confidence-band distributions, and escalation rates.
"""

from __future__ import annotations

import json
import sys
from collections import Counter, defaultdict
from pathlib import Path


def load_labels(path: Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def score(labels: dict, candidates: list[dict]) -> dict:
    expected = {item["statement_contains"]: item for item in labels.get("expected_candidates", [])}
    matched_markers: set[str] = set()
    matched_candidates = 0
    false_positives = 0
    pattern_counts: Counter[str] = Counter()
    modalities: Counter[str] = Counter()
    confidence_counts: Counter[str] = Counter()
    by_band_precision = defaultdict(lambda: {"matched": 0, "fp": 0})
    escalate_expected = 0
    escalate_observed = 0
    escalate_expected_markers = {
        item["statement_contains"]
        for item in labels.get("expected_candidates", [])
        if item.get("escalate_expected")
    }

    for candidate in candidates:
        statement = candidate.get("statement", "")
        band = (candidate.get("confidence") or "low").lower()
        confidence_counts[band] += 1
        pattern_counts[candidate.get("pattern_hint", "")] += 1
        modalities[candidate.get("modality", "")] += 1
        if band == "low":
            escalate_observed += 1

        hit_marker = None
        hit_meta = None
        for marker, meta in expected.items():
            if marker in statement:
                hit_marker = marker
                hit_meta = meta
                break
        if hit_marker is None:
            if candidate.get("kind") != "exception_candidate":
                false_positives += 1
                by_band_precision[band]["fp"] += 1
            continue
        matched_markers.add(hit_marker)
        matched_candidates += 1
        by_band_precision[band]["matched"] += 1
        if hit_marker in escalate_expected_markers:
            escalate_expected += 1
            if band != "low" and hit_meta.get("confidence_band") == "low":
                # label expected low but extractor disagreed; still counted via marker
                pass

    expected_count = len(expected)
    matched = len(matched_markers)
    precision = matched_candidates / max(matched_candidates + false_positives, 1)
    recall = matched / max(expected_count, 1)
    reusable = sum(1 for hint, count in pattern_counts.items() if hint != "manual_review" and count > 0)

    band_metrics = {}
    for band, stats in by_band_precision.items():
        total = stats["matched"] + stats["fp"]
        band_metrics[band] = {
            "matched_candidates": stats["matched"],
            "false_positives": stats["fp"],
            "precision": round(stats["matched"] / max(total, 1), 4),
        }

    expected_escalate_labels = len(escalate_expected_markers)
    return {
        "expected_candidates": expected_count,
        "extracted_candidates": len(candidates),
        "matched_markers": matched,
        "matched_candidates": matched_candidates,
        "false_positives": false_positives,
        "precision": round(precision, 4),
        "recall": round(recall, 4),
        "pattern_hint_counts": dict(pattern_counts),
        "distinct_non_manual_patterns": reusable,
        "modality_counts": dict(modalities),
        "confidence_counts": dict(confidence_counts),
        "precision_by_confidence": band_metrics,
        "escalation_rate": round(escalate_observed / max(len(candidates), 1), 4),
        "escalation_expected_labels": expected_escalate_labels,
        "escalation_observed_low": escalate_observed,
        "auto_assessable_hint_ratio": round(
            sum(pattern_counts[h] for h in pattern_counts if h != "manual_review") / max(len(candidates), 1),
            4,
        ),
    }


def main() -> int:
    if len(sys.argv) != 3:
        print("usage: reuse_pilot_harness.py <labels.json> <obligation-candidates.json>", file=sys.stderr)
        return 2
    labels = load_labels(Path(sys.argv[1]))
    candidates_doc = json.loads(Path(sys.argv[2]).read_text(encoding="utf-8"))
    candidates = candidates_doc.get("candidates", [])
    report = score(labels, candidates)
    thresholds = labels.get("thresholds", {})
    failures = []
    if report["precision"] < float(thresholds.get("min_precision", 0.0)):
        failures.append(f"precision {report['precision']} below {thresholds['min_precision']}")
    if report["recall"] < float(thresholds.get("min_recall", 0.0)):
        failures.append(f"recall {report['recall']} below {thresholds['min_recall']}")
    max_escalation = thresholds.get("max_escalation_rate")
    if max_escalation is not None and report["escalation_rate"] > float(max_escalation):
        failures.append(f"escalation_rate {report['escalation_rate']} above {max_escalation}")
    print(json.dumps(report, indent=2, sort_keys=True))
    if failures:
        print("THRESHOLDS FAILED: " + "; ".join(failures), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

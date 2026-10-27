#!/usr/bin/env python3
"""Validate SARIF 2.1.0 files against a supplied JSON Schema."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

from jsonschema import Draft7Validator, FormatChecker
from jsonschema.exceptions import SchemaError


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--schema", required=True, type=Path, help="Pinned SARIF JSON Schema file")
    parser.add_argument("sarif_files", nargs="+", type=Path, help="SARIF documents to validate")
    args = parser.parse_args()

    try:
        schema = json.loads(args.schema.read_text(encoding="utf-8"))
        Draft7Validator.check_schema(schema)
    except (OSError, json.JSONDecodeError, SchemaError) as exc:
        print(f"Invalid or unreadable SARIF schema {args.schema}: {exc}", file=sys.stderr)
        return 2

    validator = Draft7Validator(schema, format_checker=FormatChecker())
    failed = False
    for path in args.sarif_files:
        try:
            document = json.loads(path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError) as exc:
            print(f"{path}: cannot read valid JSON: {exc}", file=sys.stderr)
            failed = True
            continue

        errors = sorted(
            validator.iter_errors(document),
            key=lambda error: (tuple(str(part) for part in error.absolute_path), error.message),
        )
        if errors:
            failed = True
            for error in errors:
                location = "$"
                for part in error.absolute_path:
                    location += f"[{part}]" if isinstance(part, int) else f".{part}"
                print(f"{path}:{location}: {error.message}", file=sys.stderr)
        else:
            print(f"{path}: valid SARIF against {args.schema}")

    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())

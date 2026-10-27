#!/usr/bin/env python3
"""Check gofmt only on changed Go lines, allowing untouched legacy debt."""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import Path
from typing import Iterable

HUNK_RE = re.compile(
    r"^@@ -(?P<old_start>\d+)(?:,(?P<old_count>\d+))? "
    r"\+(?P<new_start>\d+)(?:,(?P<new_count>\d+))? @@"
)


def changed_ranges_from_git_diff(diff: str) -> list[tuple[int, int]]:
    """Return ranges of changed lines in the new-file coordinate system."""
    ranges: list[tuple[int, int]] = []
    for line in diff.splitlines():
        match = HUNK_RE.match(line)
        if not match:
            continue
        start = int(match.group("new_start"))
        count = int(match.group("new_count") or 1)
        if count > 0:
            ranges.append((start, start + count - 1))
    return ranges


def unformatted_changed_lines(
    gofmt_diff: str, changed_ranges: Iterable[tuple[int, int]]
) -> list[int]:
    """Return raw source lines that gofmt changes and that the Git diff added."""
    ranges = list(changed_ranges)
    if not gofmt_diff.strip() or not ranges:
        return []

    old_line = 0
    inside_hunk = False
    offenders: set[int] = set()
    for line in gofmt_diff.splitlines():
        match = HUNK_RE.match(line)
        if match:
            old_line = int(match.group("old_start"))
            inside_hunk = True
            continue
        if not inside_hunk:
            continue
        if line.startswith("\\"):
            continue
        if line.startswith("-"):
            if any(start <= old_line <= end for start, end in ranges):
                offenders.add(old_line)
            old_line += 1
        elif line.startswith("+"):
            # Added lines exist only in formatted output; the corresponding raw
            # source line, when present, is captured by the removed-side entry.
            continue
        elif line.startswith(" "):
            old_line += 1
        else:
            # Ignore diff headers or unexpected metadata without inventing lines.
            continue
    return sorted(offenders)


def run_git(*args: str, allow_failure: bool = False) -> str:
    result = subprocess.run(
        ["git", *args],
        check=False,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if result.returncode and not allow_failure:
        raise RuntimeError(result.stderr.strip() or f"git {' '.join(args)} failed")
    return result.stdout


def changed_go_files(base_sha: str | None, head_sha: str) -> list[str]:
    if base_sha:
        paths = run_git(
            "diff", "--name-only", "--diff-filter=ACMRT", "-z",
            base_sha, head_sha, "--", "*.go"
        )
    else:
        paths = run_git("ls-files", "-z", "*.go")
    return sorted(path for path in paths.split("\0") if path.endswith(".go"))


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-sha", default="")
    parser.add_argument("--head-sha", default="HEAD")
    args = parser.parse_args()

    base_sha = args.base_sha.strip()
    if not base_sha or re.fullmatch(r"0+", base_sha):
        base_sha = ""

    try:
        files = changed_go_files(base_sha or None, args.head_sha)
        if not files:
            print("No changed Go files to format-check.")
            return 0

        failed = False
        for name in files:
            path = Path(name)
            if not path.is_file():
                continue

            if base_sha:
                base_exists = subprocess.run(
                    ["git", "cat-file", "-e", f"{base_sha}:{name}"],
                    check=False,
                    stdout=subprocess.DEVNULL,
                    stderr=subprocess.DEVNULL,
                ).returncode == 0
                if base_exists:
                    git_diff = run_git(
                        "diff", "--unified=0", base_sha, args.head_sha, "--", name
                    )
                    changed = changed_ranges_from_git_diff(git_diff)
                else:
                    changed = [(1, max(1, len(path.read_text(encoding="utf-8").splitlines())))]
            else:
                changed = [(1, max(1, len(path.read_text(encoding="utf-8").splitlines())))]

            if not changed:
                continue

            formatted_diff = subprocess.run(
                ["gofmt", "-d", name],
                check=False,
                text=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
            )
            if formatted_diff.returncode:
                print(f"{name}: gofmt failed: {formatted_diff.stderr.strip()}", file=sys.stderr)
                failed = True
                continue

            offenders = unformatted_changed_lines(formatted_diff.stdout, changed)
            if offenders:
                print(f"{name}: gofmt changes modified line(s): {', '.join(map(str, offenders))}")
                failed = True
            elif formatted_diff.stdout.strip():
                print(f"{name}: existing formatting debt is outside modified lines; accepted")
            else:
                print(f"{name}: changed lines are gofmt-clean")

        if failed:
            print("Run gofmt -w on the changed Go statements and commit the result.", file=sys.stderr)
            return 1
        return 0
    except (OSError, RuntimeError) as exc:
        print(f"gofmt changed-line check failed: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())

#!/usr/bin/env python3
"""Extract fenced code blocks from an issue Markdown body into a sandbox tree.

Never executes code. Used by ADR Assessment so OpenGrep can evaluate snippets
against the corpus requirements pack.

  python3 scripts/issue_sandbox_extract.py --input issue.md --out-dir /tmp/issue-src
"""

from __future__ import annotations

import argparse
import re
from pathlib import Path

FENCE_RE = re.compile(
    r"```([a-zA-Z0-9_+#.\-]*)\s*\n(.*?)```",
    re.DOTALL,
)

LANG_EXT = {
    "python": ".py",
    "py": ".py",
    "go": ".go",
    "golang": ".go",
    "javascript": ".js",
    "js": ".js",
    "typescript": ".ts",
    "ts": ".ts",
    "tsx": ".tsx",
    "jsx": ".jsx",
    "java": ".java",
    "csharp": ".cs",
    "cs": ".cs",
    "php": ".php",
    "ruby": ".rb",
    "rb": ".rb",
    "bash": ".bash",
    "sh": ".sh",
    "shell": ".bash",
    "c": ".c",
    "cpp": ".cpp",
    "c++": ".cpp",
    "rust": ".rs",
    "rs": ".rs",
    "kotlin": ".kt",
    "kt": ".kt",
    "scala": ".scala",
    "swift": ".swift",
    "lua": ".lua",
    "dart": ".dart",
    "json": ".json",
}


def extract(markdown: str, out_dir: Path) -> list[Path]:
    out_dir.mkdir(parents=True, exist_ok=True)
    written: list[Path] = []
    idx = 0
    for match in FENCE_RE.finditer(markdown):
        lang = (match.group(1) or "").strip().lower()
        body = match.group(2)
        if not body.strip():
            continue
        # Skip pure markdown/text fences and empty language prose dumps.
        if lang in {"", "markdown", "md", "text", "txt", "diff", "console", "shell-session"}:
            # Still keep unlabeled fences that look like code (heuristic).
            if lang in {"markdown", "md", "text", "txt", "diff", "console", "shell-session"}:
                continue
            if lang == "" and not re.search(
                r"(def |class |function |package |import |require\(|package main|#include)",
                body,
            ):
                continue
            ext = ".txt"
        else:
            ext = LANG_EXT.get(lang)
            if ext is None:
                continue
        idx += 1
        path = out_dir / f"snippet_{idx:02d}{ext}"
        # Cap individual snippet size (256 KiB).
        data = body.encode("utf-8")
        if len(data) > 256 * 1024:
            data = data[: 256 * 1024]
        path.write_bytes(data)
        written.append(path)
    # Marker file so empty sandboxes are detectable.
    (out_dir / ".pathfinder-sandbox").write_text(
        f"snippets={len(written)}\n", encoding="utf-8"
    )
    return written


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", required=True)
    parser.add_argument("--out-dir", required=True)
    args = parser.parse_args()
    markdown = Path(args.input).read_text(encoding="utf-8")
    out = Path(args.out_dir)
    if out.exists():
        for child in out.iterdir():
            if child.is_file():
                child.unlink()
    files = extract(markdown, out)
    print(f"extracted {len(files)} snippet file(s) into {out}")
    for f in files:
        print(f"  {f.name}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

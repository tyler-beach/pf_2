#!/usr/bin/env bash
# Upload a SARIF 2.1.0 file to a GitHub repository's code scanning (Security tab).
# Works for same-repo or cross-repo when GH_TOKEN can write security_events.
#
# Usage:
#   upload_code_scanning_sarif.sh --owner O --repo R --sha SHA --ref REF --sarif FILE
#
# Prefer refs/pull/<n>/head for PR-associated uploads.
set -euo pipefail

owner=""
repo=""
sha=""
ref=""
sarif=""
tool="Pathfinder"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --owner) owner="$2"; shift 2 ;;
    --repo) repo="$2"; shift 2 ;;
    --sha) sha="$2"; shift 2 ;;
    --ref) ref="$2"; shift 2 ;;
    --sarif) sarif="$2"; shift 2 ;;
    --tool) tool="$2"; shift 2 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

if [[ -z "$owner" || -z "$repo" || -z "$sha" || -z "$ref" || -z "$sarif" ]]; then
  echo "usage: $0 --owner O --repo R --sha SHA --ref REF --sarif FILE [--tool Name]" >&2
  exit 2
fi

# Fail closed on values that could break API path segments or confuse operators.
if ! [[ "$owner" =~ ^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$ ]]; then
  echo "invalid owner: $owner" >&2
  exit 2
fi
if ! [[ "$repo" =~ ^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$ ]]; then
  echo "invalid repo: $repo" >&2
  exit 2
fi
if ! [[ "$sha" =~ ^[0-9a-fA-F]{40}$ ]]; then
  echo "invalid commit sha (want 40 hex): $sha" >&2
  exit 2
fi
if ! [[ "$ref" =~ ^(refs/heads/[A-Za-z0-9._/-]+|refs/pull/[0-9]+/head|refs/tags/[A-Za-z0-9._/-]+)$ ]]; then
  echo "invalid git ref for code scanning upload: $ref" >&2
  exit 2
fi
if ! [[ "$tool" =~ ^[A-Za-z0-9][A-Za-z0-9 ._-]{0,63}$ ]]; then
  echo "invalid tool name: $tool" >&2
  exit 2
fi
if [[ ! -f "$sarif" ]]; then
  echo "SARIF file not found: $sarif" >&2
  exit 1
fi
# Bound upload size to limit abuse of the code-scanning API from CI.
sarif_bytes="$(wc -c <"$sarif" | tr -d ' ')"
if [[ "$sarif_bytes" -gt 10485760 ]]; then
  echo "SARIF file too large (${sarif_bytes} bytes; max 10MiB)" >&2
  exit 1
fi
if [[ -z "${GH_TOKEN:-${GITHUB_TOKEN:-}}" ]]; then
  echo "GH_TOKEN or GITHUB_TOKEN is required" >&2
  exit 1
fi
export GH_TOKEN="${GH_TOKEN:-$GITHUB_TOKEN}"

# GitHub expects gzip+base64 for the REST upload endpoint.
if base64 --help 2>&1 | grep -q -- '-w'; then
  b64="$(gzip -c "$sarif" | base64 -w0)"
else
  b64="$(gzip -c "$sarif" | base64 | tr -d '\n')"
fi

echo "Uploading SARIF to ${owner}/${repo} ref=${ref} sha=${sha} tool=${tool}"
resp="$(mktemp)"
set +e
gh api \
  --method POST \
  "repos/${owner}/${repo}/code-scanning/sarifs" \
  -f commit_sha="$sha" \
  -f ref="$ref" \
  -f sarif="$b64" \
  -f tool_name="$tool" >"$resp" 2>"$resp.err"
status=$?
set -e
if [[ $status -ne 0 ]]; then
  echo "code-scanning SARIF upload failed:" >&2
  # Avoid dumping full tokenized error bodies; keep short stderr.
  head -c 2048 "$resp.err" >&2 || true
  echo >&2
  head -c 2048 "$resp" >&2 || true
  echo >&2
  exit "$status"
fi
cat "$resp"
echo
id="$(jq -r '.id // empty' "$resp" 2>/dev/null || true)"
if [[ -n "$id" ]]; then
  echo "sarif_upload_id=${id}"
fi

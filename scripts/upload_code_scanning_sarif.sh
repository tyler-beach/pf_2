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
if [[ ! -f "$sarif" ]]; then
  echo "SARIF file not found: $sarif" >&2
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
  cat "$resp.err" >&2 || true
  cat "$resp" >&2 || true
  exit "$status"
fi
cat "$resp"
echo
id="$(jq -r '.id // empty' "$resp" 2>/dev/null || true)"
if [[ -n "$id" ]]; then
  echo "sarif_upload_id=${id}"
fi

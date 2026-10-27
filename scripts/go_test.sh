#!/usr/bin/env bash
# Run Go tests for Pathfinder packages only (exclude node_modules trees).
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
pkgs="$(go list ./... | grep -v '/node_modules/' || true)"
if [ -z "$pkgs" ]; then
  echo "no go packages found" >&2
  exit 1
fi
# shellcheck disable=SC2086
go test "$@" $pkgs

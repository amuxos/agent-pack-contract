#!/usr/bin/env bash
set -euo pipefail

# Public-entrypoint regression: build.sh must produce exactly one Go release.
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

"$repo_root/build.sh" --dist-dir "$work/dist"

shopt -s nullglob
archives=("$work"/dist/agent-pack-contract_*.tar.gz)
shopt -u nullglob

if ((${#archives[@]} != 1)); then
  echo "ERROR: default build must produce exactly one Go archive, found ${#archives[@]}" >&2
  exit 1
fi
version="$(awk -F'"' '/^var Version = "/ {print $2; exit}' "$repo_root/go/internal/version/version.go")"
"$repo_root/scripts/install.sh" \
  --archive "${archives[0]}" \
  --bin-dir "$work/bin" \
  --expected-version "$version" >/dev/null

echo "OK: build.sh produced and installed the Go release ($version)"

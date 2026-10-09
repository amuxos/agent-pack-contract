#!/usr/bin/env bash
set -euo pipefail

# Build services may inject BUILD_VERSION for their own identity. That is not the
# Agent Pack Contract release version and must never override the Go canonical
# version. Only CUSTOM_VERSION is an explicit release-version override.
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

canonical_version="$(awk -F'"' '/^var Version = "/ {print $2; exit}' "$repo_root/go/internal/version/version.go")"
goos="$(cd "$repo_root/go" && go env GOOS)"
goarch="$(cd "$repo_root/go" && go env GOARCH)"

BUILD_VERSION='<ip_address_11>' \
SCM_OUTPUT_DIR="$work/canonical" \
SCM_PACKAGE_TARGETS="$goos/$goarch" \
  "$repo_root/scm_build_release.sh" >/dev/null

canonical_archive="$work/canonical/agent-pack-contract_${canonical_version}_${goos}_${goarch}.tar.gz"
if [[ ! -f "$canonical_archive" ]]; then
  echo "ERROR: SCM build did not use canonical version $canonical_version" >&2
  exit 1
fi
if ! grep -Fq '"version": "'"$canonical_version"'"' "$work/canonical/latest.json"; then
  echo "ERROR: SCM manifest did not use canonical version $canonical_version" >&2
  exit 1
fi

custom_version="9.8.7"
BUILD_VERSION='<ip_address_11>' \
CUSTOM_VERSION="$custom_version" \
SCM_OUTPUT_DIR="$work/custom" \
SCM_PACKAGE_TARGETS="$goos/$goarch" \
  "$repo_root/scm_build_release.sh" >/dev/null

custom_archive="$work/custom/agent-pack-contract_${custom_version}_${goos}_${goarch}.tar.gz"
if [[ ! -f "$custom_archive" ]]; then
  echo "ERROR: SCM build did not honor CUSTOM_VERSION=$custom_version" >&2
  exit 1
fi

echo "OK: SCM release version uses canonical source or explicit CUSTOM_VERSION"

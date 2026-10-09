#!/usr/bin/env bash
set -euo pipefail

# Install the agent-pack-contract binary for the current platform.
# Modeled on amux's scripts/install.sh (single binary).
#
#   scripts/install.sh --bin-dir ~/.local/bin
#   curl -fsSL '<release-bundle-url>' | tar -xzOf - ./install.sh | bash

bin_name="agent-pack-contract"
scm_latest_url="${AGENT_PACK_CONTRACT_SCM_URL:-https://github.com/amuxos/agent-pack-contract-releases/releases/latest/download/agent-pack-contract-scm-latest.tar.gz}"
bin_dir=""
archive=""
expected_version=""
expected_sha256=""

usage() {
  cat <<EOF
Usage: scripts/install.sh [--bin-dir <dir>] [--scm-url <url>] [--archive <tar.gz>]
                          [--expected-version <semver>] [--sha256 <digest>]

Install the agent-pack-contract binary for the current platform.

Options:
  --bin-dir <dir>   Install the binary into this directory (default: ~/.local/bin)
  --scm-url <url>   release bundle download URL
  --archive <tar.gz> Install from a local platform archive instead of downloading
  --expected-version <semver> Reject a binary whose --version differs
  --sha256 <digest> Require this SHA-256 for the selected platform archive

Environment:
  AGENT_PACK_CONTRACT_SCM_URL   Override the release bundle URL.
  AGENT_PACK_CONTRACT_BIN_DIR   Default install directory.
EOF
}

detect_os() {
  case "$(uname -s)" in
    Darwin) echo darwin ;;
    Linux) echo linux ;;
    *) uname -s | tr '[:upper:]' '[:lower:]' ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    arm64 | aarch64) echo arm64 ;;
    x86_64 | amd64) echo amd64 ;;
    *) uname -m ;;
  esac
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    echo "neither sha256sum nor shasum is available" >&2
    return 1
  fi
}

while (($#)); do
  case "$1" in
    --bin-dir) bin_dir="${2:?missing value for --bin-dir}"; shift ;;
    --scm-url) scm_latest_url="${2:?missing value for --scm-url}"; shift ;;
    --archive) archive="${2:?missing value for --archive}"; shift ;;
    --expected-version) expected_version="${2:?missing value for --expected-version}"; shift ;;
    --sha256) expected_sha256="${2:?missing value for --sha256}"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown arg: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done

bin_dir="${bin_dir:-${AGENT_PACK_CONTRACT_BIN_DIR:-${HOME:-}/.local/bin}}"
key="$(detect_os)_$(detect_arch)"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

if [[ -z "$archive" ]]; then
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  shopt -s nullglob
  local_matches=("$script_dir"/"${bin_name}"_*_"$key".tar.gz)
  shopt -u nullglob
  if ((${#local_matches[@]} == 1)); then
    archive="${local_matches[0]}"
  fi
fi

if [[ -z "$archive" ]]; then
  latest_archive="$tmp/latest.tar.gz"
  mkdir -p "$tmp/latest"
  echo "==> curl -fsSL $scm_latest_url"
  curl -fsSL -o "$latest_archive" "$scm_latest_url"
  tar -xzf "$latest_archive" -C "$tmp/latest"
  shopt -s nullglob
  matches=("$tmp/latest"/"${bin_name}"_*_"$key".tar.gz)
  shopt -u nullglob
  if ((${#matches[@]} != 1)); then
    echo "no $bin_name archive for platform $key in SCM latest package" >&2
    exit 1
  fi
  archive="${matches[0]}"
fi

checksum_file="$archive.sha256"
actual_sha256=""
if [[ -n "$expected_sha256" ]]; then
  actual_sha256="$(sha256_file "$archive")"
  if [[ "$actual_sha256" != "$expected_sha256" ]]; then
    echo "SHA-256 mismatch for $archive: expected $expected_sha256, got $actual_sha256" >&2
    exit 1
  fi
elif [[ -f "$checksum_file" ]]; then
  expected_sha256="$(awk 'NF {print $1; exit}' "$checksum_file")"
  actual_sha256="$(sha256_file "$archive")"
  if [[ -z "$expected_sha256" || "$actual_sha256" != "$expected_sha256" ]]; then
    echo "SHA-256 mismatch for $archive: expected ${expected_sha256:-<empty>}, got $actual_sha256" >&2
    exit 1
  fi
else
  echo "missing SHA-256 checksum for $archive (expected $checksum_file or --sha256)" >&2
  exit 1
fi

mkdir -p "$tmp/rel"
tar -xzf "$archive" -C "$tmp/rel"

binary="$tmp/rel/$bin_name"
if [[ ! -x "$binary" ]]; then
  # The archive wraps the binary in a single package dir; locate it.
  binary="$(find "$tmp/rel" -type f -name "$bin_name" -print -quit)"
fi
if [[ -z "$binary" || ! -x "$binary" ]]; then
  echo "archive does not contain an executable $bin_name" >&2
  exit 1
fi

if [[ -n "$expected_version" ]]; then
  actual_version="$("$binary" --version)"
  if [[ "$actual_version" != "$expected_version" ]]; then
    echo "version mismatch: expected $expected_version, got $actual_version" >&2
    exit 1
  fi
fi

mkdir -p "$bin_dir"
install -m 0755 "$binary" "$bin_dir/$bin_name"
echo "installed $bin_name -> $bin_dir/$bin_name"

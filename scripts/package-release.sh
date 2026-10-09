#!/usr/bin/env bash
set -euo pipefail

# Build and package the agent-pack-contract Go binary for one platform.
# Modeled on amux's scripts/package-release.sh (single binary).

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"

version="$(awk -F'"' '/^var Version = "/ {print $2; exit}' "$repo_root/go/internal/version/version.go")"
goos=""
goarch=""
dist_dir="$repo_root/dist"

usage() {
  cat <<EOF
Usage: scripts/package-release.sh [--version <v>] [--os <goos>] [--arch <goarch>] [--dist-dir <dir>]

Build the agent-pack-contract Go binary and package it as a tar.gz + sha256.

Options:
  --version <v>   SemVer embedded in the binary and package name (default: Go source version)
  --os <goos>     Package OS label (default: go env GOOS)
  --arch <goarch> Package arch label (default: go env GOARCH)
  --dist-dir <dir> Output directory (default: ./dist)
EOF
}

while (($#)); do
  case "$1" in
    --version) version="${2:?missing value for --version}"; shift ;;
    --os) goos="${2:?missing value for --os}"; shift ;;
    --arch) goarch="${2:?missing value for --arch}"; shift ;;
    --dist-dir) dist_dir="${2:?missing value for --dist-dir}"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown arg: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done

goos="${goos:-$(cd "$repo_root/go" && go env GOOS)}"
goarch="${goarch:-$(cd "$repo_root/go" && go env GOARCH)}"

if [[ ! "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  echo "invalid release version '$version'; expected SemVer X.Y.Z" >&2
  exit 1
fi
for component_name in goos goarch; do
  component="${!component_name}"
  if [[ ! "$component" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]]; then
    echo "invalid $component_name '$component'; expected one path component" >&2
    exit 1
  fi
done

mkdir -p "$dist_dir"
dist_dir="$(cd "$dist_dir" && pwd -P)"

bin_name="agent-pack-contract"
package="${bin_name}_${version}_${goos}_${goarch}"
archive="$dist_dir/$package.tar.gz"
checksum_file="$archive.sha256"
package_dir="$dist_dir/$package"

case "$package_dir" in
  "$dist_dir"/*) ;;
  *) echo "refusing package path outside dist dir: $package_dir" >&2; exit 1 ;;
esac

echo "==> go build GOOS=$goos GOARCH=$goarch VERSION=$version"
(
  cd "$repo_root/go"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath \
      -ldflags "-X github.com/amuxos/agent-pack-contract/go/internal/version.Version=$version" \
      -o "$dist_dir/$bin_name" ./cmd/agent-pack-contract
)

rm -rf "$package_dir" "$archive" "$checksum_file"
mkdir -p "$package_dir"
cp "$dist_dir/$bin_name" "$package_dir/$bin_name"
cp "$repo_root/LICENSE" "$repo_root/THIRD_PARTY_NOTICES" "$package_dir/"
chmod 0755 "$package_dir/$bin_name"
rm -f "$dist_dir/$bin_name"

COPYFILE_DISABLE=1 tar -C "$dist_dir" -czf "$archive" "$package"
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "$archive" | awk '{print $1}' > "$checksum_file"
elif command -v shasum >/dev/null 2>&1; then
  shasum -a 256 "$archive" | awk '{print $1}' > "$checksum_file"
else
  echo "neither sha256sum nor shasum is available" >&2
  exit 1
fi

echo "Packaged $archive"
echo "Checksum $checksum_file"

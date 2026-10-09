#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
candidate="${1:-$repo_root/bin/agent-pack-contract}"

if [[ ! -x "$candidate" ]]; then
  echo "ERROR: Go candidate is not executable: $candidate" >&2
  exit 1
fi

version="$("$candidate" --version)"
goos="$(cd "$repo_root/go" && go env GOOS)"
goarch="$(cd "$repo_root/go" && go env GOARCH)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

"$repo_root/scripts/package-release.sh" \
  --version "$version" \
  --os "$goos" \
  --arch "$goarch" \
  --dist-dir "$work/release"

archive="$work/release/agent-pack-contract_${version}_${goos}_${goarch}.tar.gz"
"$repo_root/scripts/install.sh" \
  --archive "$archive" \
  --bin-dir "$work/bin" \
  --expected-version "$version"

for notice in LICENSE THIRD_PARTY_NOTICES; do
  tar -xzOf "$archive" "agent-pack-contract_${version}_${goos}_${goarch}/$notice" > "$work/$notice"
  cmp "$repo_root/$notice" "$work/$notice"
done

installed="$work/bin/agent-pack-contract"
[[ "$("$installed" --version)" == "$version" ]]
"$installed" check \
  --repo-root "$repo_root/tests/fixtures/sample-pack" \
  --out "$work/agent-pack.manifest.json" >/dev/null
"$installed" validate "$work/agent-pack.manifest.json" >/dev/null

echo "OK: verified Go release archive, checksum, install, and CLI ($version $goos/$goarch)"

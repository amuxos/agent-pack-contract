#!/usr/bin/env bash
set -euo pipefail

# Assemble the portable release bundle for agent-pack-contract.
# Existing SCM_* variable names are retained as packaging API compatibility.

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
output_dir="${SCM_OUTPUT_DIR:-$repo_root/output}"

if [[ -z "$output_dir" || "$output_dir" == "/" || "$output_dir" == "." || "$output_dir" == "$repo_root" ]]; then
  echo "refusing unsafe output dir: ${output_dir:-<empty>}" >&2
  exit 1
fi

canonical_version="$(awk -F'"' '/^var Version = "/ {print $2; exit}' "$repo_root/go/internal/version/version.go")"
# BUILD_VERSION belongs to the SCM pipeline and is not guaranteed to be a
# Contract SemVer. Release overrides must be explicit.
version="${CUSTOM_VERSION:-$canonical_version}"
base_url="${CUSTOM_AGENT_PACK_CONTRACT_RELEASE_BASE_URL:-}"

rm -rf "$output_dir"
mkdir -p "$output_dir"

targets=()
if [[ -n "${SCM_PACKAGE_TARGETS:-}" ]]; then
  IFS=',' read -r -a targets <<< "$SCM_PACKAGE_TARGETS"
elif [[ -n "${CUSTOM_GOOS:-}" || -n "${CUSTOM_GOARCH:-}" ]]; then
  goos="${CUSTOM_GOOS:-$(cd "$repo_root/go" && go env GOOS)}"
  goarch="${CUSTOM_GOARCH:-$(cd "$repo_root/go" && go env GOARCH)}"
  targets=("$goos/$goarch")
else
  targets=("linux/amd64" "linux/arm64" "darwin/amd64" "darwin/arm64")
fi

target_keys=()
for target in "${targets[@]}"; do
  goos="${target%%/*}"
  goarch="${target##*/}"
  if [[ -z "$goos" || -z "$goarch" || "$goos" == "$goarch" ]]; then
    echo "invalid target '$target'; use os/arch, e.g. linux/amd64" >&2
    exit 1
  fi
  args=(--version "$version" --dist-dir "$output_dir" --os "$goos" --arch "$goarch")
  "$repo_root/scripts/package-release.sh" "${args[@]}"
  target_keys+=("${goos}_${goarch}")
done

json_escape() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//$'\n'/\\n}"
  value="${value//$'\r'/\\r}"
  value="${value//$'\t'/\\t}"
  printf '%s' "$value"
}

manifest="$output_dir/latest.json"
{
  printf '{\n'
  printf '  "version": "%s",\n' "$(json_escape "$version")"
  printf '  "assets": {\n'
  for i in "${!target_keys[@]}"; do
    key="${target_keys[$i]}"
    archive_name="agent-pack-contract_${version}_${key}.tar.gz"
    checksum_file="$output_dir/$archive_name.sha256"
    sha256="$(awk '{print $1}' "$checksum_file")"
    asset_url="$archive_name"
    if [[ -n "$base_url" ]]; then
      asset_url="${base_url%/}/$archive_name"
    fi
    comma=","
    if ((i == ${#target_keys[@]} - 1)); then
      comma=""
    fi
    printf '    "%s": {\n' "$key"
    printf '      "url": "%s",\n' "$(json_escape "$asset_url")"
    printf '      "sha256": "%s"\n' "$(json_escape "$sha256")"
    printf '    }%s\n' "$comma"
  done
  printf '  }\n'
  printf '}\n'
} > "$manifest"

# Drop the per-platform package dirs; only archives + sha256 + latest.json remain.
for key in "${target_keys[@]}"; do
  rm -rf "$output_dir/agent-pack-contract_${version}_${key}"
done

cp "$repo_root/scripts/install.sh" "$output_dir/install.sh"
chmod +x "$output_dir/install.sh"

echo "Prepared agent-pack-contract SCM release output at $output_dir"

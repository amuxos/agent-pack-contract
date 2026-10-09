#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

cmp schemas/agent-pack-manifest.schema.json \
  go/internal/schema/agent-pack-manifest.schema.json

unformatted="$(gofmt -l go)"
if [[ -n "$unformatted" ]]; then
  echo "ERROR: gofmt required for:" >&2
  printf '%s\n' "$unformatted" >&2
  exit 1
fi

mkdir -p bin
(
  cd go
  go vet ./...
  go test ./...
  go build -trimpath -o ../bin/agent-pack-contract ./cmd/agent-pack-contract
)
scripts/verify_go_release.sh bin/agent-pack-contract
ci/test-default-build.sh
ci/test-scm-build-release.sh
(
  cd go
  go run ./cmd/check-release-metadata --repo-root .. --tag-source remote
)

#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

ci/check.sh
(
  cd go
  go run ./cmd/check-release-metadata \
    --repo-root .. \
    --tag-source remote \
    --require-unpublished
)

#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

for command in go node npm pnpm staticcheck golangci-lint govulncheck; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "certification requires $command" >&2
    exit 1
  fi
done

make verify
scripts/verify_versions.sh "${1-}"
scripts/check_reproducible.sh
staticcheck ./...
golangci-lint run ./...
govulncheck ./...
npm audit --prefix sdk/typescript --audit-level=high

echo "local source certification passed"

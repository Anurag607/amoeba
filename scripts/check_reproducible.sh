#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/agentic-moe-repro.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM

version=${VERSION:-0.1.0}
commit=${COMMIT:-0000000000000000000000000000000000000000}
date=${BUILD_DATE:-1970-01-01T00:00:00Z}
ldflags="-s -w -buildid= -X github.com/anurgosw/agentic-moe/internal/buildinfo.Version=$version -X github.com/anurgosw/agentic-moe/internal/buildinfo.Commit=$commit -X github.com/anurgosw/agentic-moe/internal/buildinfo.Date=$date"

(
  cd "$root"
  CGO_ENABLED=0 SOURCE_DATE_EPOCH=0 go build -trimpath -ldflags "$ldflags" -o "$work/first" ./cmd/agentic-moe
  CGO_ENABLED=0 SOURCE_DATE_EPOCH=0 go build -trimpath -ldflags "$ldflags" -o "$work/second" ./cmd/agentic-moe
)

if ! cmp -s "$work/first" "$work/second"; then
  echo "reproducibility check failed: identical inputs produced different binaries" >&2
  exit 1
fi

echo "reproducible binary sha256: $(shasum -a 256 "$work/first" | awk '{print $1}')"

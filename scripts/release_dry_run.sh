#!/bin/sh
set -eu

repo_dir=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
release_dir="$repo_dir/.release"
cleanup() {
  rm -rf -- "$release_dir"
}
trap cleanup EXIT HUP INT TERM

cd "$repo_dir"

if [ "${1-}" != "--verify-only" ]; then
  if command -v goreleaser >/dev/null 2>&1; then
    goreleaser release --snapshot --clean --skip=publish --parallelism=1
  else
    printf '%s\n' 'goreleaser is required for release-check' >&2
    exit 1
  fi
  first_archives=$(find "$release_dir" -maxdepth 1 -type f \( -name '*.tar.gz' -o -name '*.zip' \) -exec shasum -a 256 {} \; | sed "s|  $release_dir/|  |" | sort)
  goreleaser release --snapshot --clean --skip=publish --parallelism=1
  second_archives=$(find "$release_dir" -maxdepth 1 -type f \( -name '*.tar.gz' -o -name '*.zip' \) -exec shasum -a 256 {} \; | sed "s|  $release_dir/|  |" | sort)
  if [ "$first_archives" != "$second_archives" ]; then
    printf '%s\n' 'release archives are not reproducible' >&2
    exit 1
  fi
  printf '%s\n' 'release archives are byte-reproducible'
fi

npm pack --dry-run ./sdk/typescript >/dev/null
test -f "$release_dir/checksums.txt"
archive_count=$(find "$release_dir" -maxdepth 1 -type f \( -name '*.tar.gz' -o -name '*.zip' \) | wc -l | tr -d ' ')
sbom_count=$(find "$release_dir" -maxdepth 1 -type f -name '*.sbom.json' | wc -l | tr -d ' ')
test "$archive_count" -eq 5
test "$sbom_count" -eq 5
(cd "$release_dir" && shasum -a 256 -c checksums.txt)

for archive in "$release_dir"/*.tar.gz; do
  contents=$(tar -tzf "$archive")
  printf '%s\n' "$contents" | grep -qx 'agentic-moe'
  printf '%s\n' "$contents" | grep -qx 'agentic-moe-eval'
  printf '%s\n' "$contents" | grep -qx 'LICENSE'
  printf '%s\n' "$contents" | grep -qx 'README.md'
done
for archive in "$release_dir"/*.zip; do
  contents=$(unzip -Z1 "$archive")
  printf '%s\n' "$contents" | grep -qx 'agentic-moe.exe'
  printf '%s\n' "$contents" | grep -qx 'agentic-moe-eval.exe'
  printf '%s\n' "$contents" | grep -qx 'LICENSE'
  printf '%s\n' "$contents" | grep -qx 'README.md'
done

cask="$release_dir/homebrew/Casks/agentic-moe.rb"
test -s "$cask"
grep -q 'github.com/Anurag607/amoeba/releases/download/' "$cask"

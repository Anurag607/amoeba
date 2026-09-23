#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
package_version=$(node -p "require('$root/sdk/typescript/package.json').version")
source_version=$(sed -n 's/^export const VERSION = "\([^"]*\)";$/\1/p' "$root/sdk/typescript/src/version.ts")

if [ -z "$package_version" ] || [ "$package_version" != "$source_version" ]; then
  echo "version mismatch: package.json=$package_version version.ts=$source_version" >&2
  exit 1
fi

if [ "${1-}" != "" ]; then
  tag=${1#v}
  if [ "$tag" != "$package_version" ]; then
    echo "version mismatch: tag=$tag package.json=$package_version" >&2
    exit 1
  fi
fi

echo "version $package_version is consistent"

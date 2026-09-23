#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/agentic-moe-package.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM

for command in go npm pnpm node; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "package smoke test requires $command" >&2
    exit 1
  fi
done
expected_version=$(node -p "require('$root/sdk/typescript/package.json').version")

(
  cd "$root"
  go build -trimpath -o "$work/agentic-moe" ./cmd/agentic-moe
  npm run build --prefix sdk/typescript
  npm pack "$root/sdk/typescript" --pack-destination "$work" >/dev/null
)
archive=$(find "$work" -maxdepth 1 -type f -name 'agentic-moe-*.tgz' -print -quit)
if [ -z "$archive" ]; then
  echo "npm pack did not create an archive" >&2
  exit 1
fi

mkdir "$work/npm" "$work/pnpm"
(
  cd "$work/npm"
  npm init --yes >/dev/null
  npm install --ignore-scripts "$archive" >/dev/null
  AGENTIC_MOE_BINARY="$work/agentic-moe" ./node_modules/.bin/agentic-moe version --json >/dev/null
  EXPECTED_VERSION="$expected_version" node -e "import('agentic-moe').then(m => { if (m.VERSION !== process.env.EXPECTED_VERSION) process.exit(1) })"
  AGENTIC_MOE_BINARY="$work/agentic-moe" node --input-type=module -e '
    import { AgenticMOEClient } from "agentic-moe";
    const client = await AgenticMOEClient.stdio({ command: process.env.AGENTIC_MOE_BINARY });
    const plan = await client.plan("review this code", { has_code_context: true });
    const manifest = await client.manifest();
    const invalid = await client.validateConfig("version: 99");
    await client.close();
    if (plan.expert.id !== "coding" || !manifest.transports.includes("stdio") || invalid.valid !== false) process.exit(1);
  '
)
node "$root/scripts/http_smoke.mjs" "$work/agentic-moe"
(
  cd "$work/pnpm"
  npm init --yes >/dev/null
  pnpm add --ignore-scripts "$archive" >/dev/null
  AGENTIC_MOE_BINARY="$work/agentic-moe" pnpm exec agentic-moe version --json >/dev/null
  EXPECTED_VERSION="$expected_version" pnpm exec node -e "import('agentic-moe').then(m => { if (m.VERSION !== process.env.EXPECTED_VERSION) process.exit(1) })"
)

echo "npm and pnpm package smoke tests passed"

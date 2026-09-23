# Releasing

Releases use semantic versioning. Before 1.0, a minor version may change a
public contract; patch versions remain backward compatible. Go module tags,
the npm package version, binary metadata, and release tag must agree.

1. Update `CHANGELOG.md`, `sdk/typescript/package.json`, its lockfile, and
   `sdk/typescript/src/version.ts`.
2. Run `make certify`; pass the intended tag to `scripts/certify.sh vX.Y.Z` to
   enforce tag/package consistency before tagging.
3. Install GoReleaser v2 and Syft, then run `make release-check`.
4. Inspect `.release/checksums.txt`, archive contents, SBOMs, and the generated
   Homebrew formula. Remove `.release/` after inspection.
5. Create the annotated `vX.Y.Z` tag and push it only to the canonical remote.

The release workflow builds macOS and Linux for amd64/arm64 and Windows amd64,
creates tar/zip archives, SHA-256 checksums, build metadata, and archive SBOMs,
publishes GitHub assets, attaches a GitHub build-provenance attestation, updates
the Homebrew tap, and publishes npm with provenance. The npm job waits for the
binary release to succeed. Actions are pinned to full commit digests, jobs use
least-privilege permissions, and dependency updates are proposed by Dependabot.
GitHub, Homebrew tap, and npm credentials are deployment-owned and must be
configured in repository secrets; local source builds need none.

`scripts/check_reproducible.sh` builds the CLI twice with fixed metadata and
requires byte-identical output. `scripts/verify_versions.sh` checks the npm and
TypeScript versions and optionally checks a release tag. Publication is not a
valid local verification step without an authenticated canonical GitHub remote,
Homebrew tap, and npm account.

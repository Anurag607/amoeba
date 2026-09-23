# Contributing

Use Go 1.25 or newer and Node 20 or newer. Read `AGENTS.md` for the repository
layout, file-size, approval, and verification contracts.

Before submitting a change:

1. Keep public behavior host-neutral and side effects host-owned.
2. Add offline tests for changed behavior.
3. Run `make verify`; use `make certify` for the full static-analysis and
   dependency-audit gate.
4. If release packaging changed, install GoReleaser and Syft, then run
   `make release-check`.
5. Update user and contract documentation in the same change.

Commits should be focused and use an imperative summary. Pull requests should
describe the contract changed, security implications, and verification run.

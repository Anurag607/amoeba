# AGENTS.md

Instructions for AI coding agents working on `agentic-moe`. The module is a
reusable Go control-plane library; concrete models, transports, databases,
sandboxes, credentials, and host side effects remain consumer-owned.

## Repository map

- Domain packages live directly under the repository root. Keep package roots
  focused on public contracts and composition for that domain.
- `adapter/` defines MCP and external-harness boundaries.
- `runtimekit/` is the stable composition facade; `mcpserver/` and
  `cmd/agentic-moe/` are thin protocol and CLI adapters.
- `provider/ollama/` owns bounded read-only Ollama discovery;
  `sdk/typescript/` delegates to the Go runtime over MCP.
- `execution/`, `continuation/`, `contextengine/`, and `trajectory/` own the
  durable execution and recovery contracts.
- `moe/` owns routing, delegation, provider selection, and adaptive planning.
- `test/` contains repository-wide invariant tests. Package behavior tests stay
  beside their Go package so they can exercise package-private contracts.

## Source organization (mandatory)

- Before editing or creating a file, inspect the surrounding package and its
  siblings. Put behavior in the narrowest package that owns the responsibility.
- Keep one primary type, workflow stage, protocol surface, persistence concern,
  or closely related contract family per file. Split by responsibility before a
  file approaches the hard size limit.
- Extend a cohesive existing package before creating another. A new child
  package needs one clear owner and must not be named `helpers`, `common`,
  `misc`, or `utils`.
- Parent package roots are composition boundaries. Do not accumulate transport,
  persistence, parsing, or domain-specific implementation there when a focused
  child package owns it better.
- Name files for their responsibility. Do not leave permanent `_new`, `_old`,
  `_v2`, `_migration`, or staging names.
- A move is complete only when callers and tests use the final location and the
  obsolete file, duplicate implementation, temporary bridge, and empty legacy
  directory are removed.
- Each maintained directory may contain at most 19 immediate production source
  files and at most 19 immediate test source files. Go tests remain co-located;
  the two counts are separate so private package contracts remain testable with
  the standard Go toolchain.
- Before creating a file, count its destination category. At 19 files, first
  carve out a cohesive child package or consolidate an already-split concern.
- Inspect the affected tree and `git diff --name-status` when Git metadata is
  available. Every added or moved file must have an intentional final location.

## File-size contract

- Every production and test source file has a hard ceiling of **700 lines**.
- New files should normally finish at **500–650 lines or fewer** so routine
  maintenance has room to grow. Smaller cohesive files are preferred; never pad
  a file to reach the range.
- Record the line count of every file in scope before editing. If a touched file
  is over 700 lines, splitting it is part of the same change. Do not add content
  to an over-limit file.
- Do not evade the ceiling with dense one-line declarations, formatting tricks,
  generated-looking compression, or unrelated helper dumping grounds.
- The only exception is one genuinely indivisible declaration over the limit.
  It must be explicitly approved and reported; a multi-responsibility file is
  never an exception.
- `go test ./test -run TestRepositoryLayout` is the executable repository-wide
  size, density, and metadata-junk assertion. It must pass before completion.

## Go conventions

- Use short professional names and one- or two-letter receivers.
- Comments explain why. Every exported package has a `// Package name ...`
  comment, and security-sensitive code states the threat it prevents.
- Wrap errors with operation context using `fmt.Errorf("operation: %w", err)`.
- Every goroutine accepts or closes over a cancellable `context.Context` and
  exits when it is cancelled.
- Preserve immutable identity, policy/catalog pinning, operation-envelope,
  owner isolation, prompt-injection guardrail, continuation, and effect-unknown
  reconciliation guarantees when touching those contracts.

## Tests

- Keep package tests beside their package and use the standard `testing`
  package. Repository-wide invariant tests live under `test/`.
- Tests are offline: no live model, account, credential, or network dependency.
- Add focused regression coverage for changed behavior, including concurrency,
  restart, or fault cases when the contract is sensitive to them.
- Do not weaken an assertion merely to accommodate an implementation change.

## Goal definition and approval

- For every request, first present a definite, exhaustive, ordered list covering
  implementation, tests, documentation, migration, verification, and cleanup.
- Obtain explicit user approval before editing files, running mutating commands,
  or beginning implementation.
- The approved list is the execution scope. If discovery adds required work,
  stop and obtain approval for the revised list.
- Track every approved item and report anything skipped, blocked, deferred, or
  changed in scope.

## Workflow

- Make the smallest cohesive change that satisfies the approved request. Do not
  refactor unrelated code or introduce an abstraction for one call site.
- Complete implementation, tests, documentation, migration, and cleanup before
  running test suites or builds.
- Update `README.md` and other existing contract documentation when behavior or
  public APIs change. Do not create change-summary Markdown files.
- Never edit generated output, dependency caches, `bin/`, `.dist/`, `vendor/`,
  or `node_modules/`.
- Remove `.DS_Store`, coverage output, temporary files, and obsolete sources.
- Before declaring completion, run `make verify`. For packaging changes, also
  run `make release-check` with GoReleaser v2 and Syft installed. Inspect the
  final file counts and confirm that no supported source file exceeds 700 lines.

## Verification

```bash
make verify
```

The target formats Go, runs the layout assertion, unit tests, race detection,
coverage, vet, and a full module build.

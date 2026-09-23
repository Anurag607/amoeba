---
name: dependency-upstream-verification
description: Before deciding behavior that depends on an external library, read its current docs, source, and types — don't infer from memory. Use when a bug involves a third-party API, when adding a new SDK, or when an upgrade changes runtime behavior.
category: workflow
priority: 30
---

# Dependency upstream verification

A surprising fraction of "we have a bug" tickets are actually "we
assumed the library does X but it does Y." Memory of an SDK is
stale; types and docs are not. Verify before deciding.

## When to verify

Always, before:

- Filing a bug whose root cause may be upstream behavior.
- Changing a call site because "the API probably accepts …".
- Asserting defaults, error semantics, retry behavior, or timing.
- Bumping a version across a major boundary.
- Adding a new SDK to the codebase.

If the change touches a method signature you haven't read this week,
verify.

## What "verify" means

1. **Pin the version.** `go list -m`, `npm ls`, `cargo tree`, etc.
   Make sure you're reading docs / source for the *exact* version
   the build resolves to. The headline website usually documents
   `latest`, which may have diverged.

2. **Read the source.** For Go modules, `go doc -all <pkg>` and
   `go env GOMODCACHE` + `find` is faster than the website. For
   npm, read `node_modules/<pkg>/dist/*.d.ts`. For Rust,
   `cargo doc --open`.

3. **Read the error path.** Most bugs are wrong assumptions about
   *failure* behavior: does this return an error or panic? Is the
   error retryable? Does the SDK log + swallow, or surface? Look at
   the source's `return err` lines, not the docs.

4. **Confirm the runtime behavior.** A 5-line throwaway repro in a
   `cmd/scratch/main.go` (deleted before commit) is cheaper than
   guessing. Especially useful for SDKs with hidden retry loops or
   global state.

## What counts as a source

In priority order:

1. The package source itself, at the resolved version.
2. The package's typed declarations (`.d.ts`, `.pyi`, generated
   stubs).
3. The current version's docs (not Stack Overflow).
4. A maintainer's recent issue or release-notes statement.

Stack Overflow answers and your own memory are tier-3 at best.

## Anti-patterns

- "It probably defaults to …" — verify or don't write that comment.
- Reading the README for `master` when you use a 6-month-old tag.
- Trusting a wrapping library's docs about the library it wraps —
  read the underlying lib's docs too.
- Skipping verification on "well-known" stdlib APIs whose behavior
  changed across language versions (e.g. Go context cancellation,
  Node fetch semantics).

---
name: commit-discipline
description: Make commits small, focused, conventional, and grouped so review and bisect stay fast. Use when staging changes, splitting a large diff, or deciding what belongs in one commit.
category: workflow
priority: 30
---

# Commit discipline

One commit = one logical change reviewers can hold in their head. The
goal is `git bisect` actually finding a culprit, and `git log
--oneline` reading like a changelog.

## Shape of a good commit

- **Subject** ≤ 72 chars, imperative mood, no trailing period.
  `fix: validate url before fetch`, not `Fixed the url validation`.
- **Conventional-ish prefix** when the repo uses one: `feat:`,
  `fix:`, `refactor:`, `perf:`, `docs:`, `test:`, `chore:`,
  `security:`. Stay consistent with the repo's existing style; don't
  invent.
- **Body** (optional, separated by blank line) explains *why*, not
  *what* — the diff already shows what. Wrap at ~72 chars.
- **Footer** for cross-references: `Fixes #123`, `Refs PR-456`.

## Grouping rules

Stage and commit by intent, not by file:

- One commit per behavior change.
- Pure refactors go in their own commit, separate from behavior
  changes. Mixing them makes review impossible.
- Test additions for a new behavior live with the behavior commit.
  Test fixes for unrelated flakes are their own commit.
- Generated / formatting-only changes are isolated commits with
  `chore:` or `style:` prefix so reviewers can skim them.

If you find yourself wanting to write "and also …" in a commit
message, split it.

## Staged-files hygiene

Before every commit:

- `git status` — confirm nothing accidental is staged. No `.env`,
  no editor scratch files, no compiled binaries.
- `git diff --staged` — actually read the diff. Catch debug prints,
  commented-out blocks, TODOs you meant to resolve.
- Run the formatter on staged files only. Don't bundle a
  whole-repo reformat into a feature commit.

## When in doubt

Three small commits cost the reviewer almost nothing; one giant
commit can take an hour to review. Optimize for the reviewer's
attention, not your own.

## Anti-patterns

- "WIP" or "fixup" commits left in the final history. Squash before
  push.
- Commits whose subject is a file name (`features.go`).
- Commits that change behavior *and* rename a function in one shot —
  reviewers can't tell the rename was no-op.
- Pushing with `--force` on a shared branch.
- Disabling commit hooks (`--no-verify`) to skip a failing check.
  Fix the check.

---
name: code-review
description: Review code in priority order — correctness, security, design, readability, tests — and give feedback that distinguishes blockers from preferences. Use when reviewing a PR, evaluating a diff, or grading your own change before pushing.
category: workflow
priority: 15
---

# Code review

A useful review answers two questions: *would this be safe to ship?*
and *is it the simplest version of the change?* Everything else is
noise. Work the checklist in priority order — don't bikeshed style
when the diff has a correctness bug.

## What to check, in order

1. **Correctness.** Does the diff do what its description claims? Are
   edge cases (nil, empty slice, error path, partial failure)
   handled? Mentally walk one happy path and one failure path.
2. **Security.** Input validation at boundaries, auth checks on every
   sensitive route, no secrets in logs/error messages, no injection
   vectors (SQL/Mongo/shell/regex). See the `mongo-injection-safety`,
   `ssrf-safe-http`, and `identity-pinning` skills.
3. **Design.** Is the abstraction level right? Is anything added
   that has only one caller and could be inlined? Are new public
   surfaces minimal?
4. **Readability.** Could someone unfamiliar follow the logic in one
   pass? Names match what the values represent? Comments explain
   *why*, not *what*?
5. **Tests.** Do the tests assert observable behavior, not
   implementation detail? Is the failure-path covered? Are flaky
   helpers (sleep, real network) avoided?
6. **Performance.** Only flag if you have concrete evidence of a
   bottleneck. Speculative micro-optimization is review noise.

## Giving feedback

- **Block vs nit.** Mark every comment as one of: `[blocker]`,
  `[suggest]`, `[nit]`, `[question]`. Reviewers without that
  shorthand often end up arguing over taste.
- **Phrase suggestions as questions** when intent is unclear:
  "Would moving this behind a flag let us roll back faster?" beats
  "this should be flagged."
- **Acknowledge what works.** A short "good catch on the SSRF dial
  check" costs you nothing and keeps morale alive across long
  reviews.
- **Don't pile on.** If a previous reviewer already flagged the same
  thing, +1 their comment instead of a parallel one.

## What not to do

- Suggest a refactor that doubles the size of the diff. File a
  follow-up.
- Block on style the project doesn't already enforce. Make it a lint
  rule first.
- Approve without reading the failure paths. Most production
  incidents live there.
- Re-review only the latest force-push. Read the *delta since you
  last looked*; GitHub's "Files changed" view loses context after
  rebases.

## When you're the author

Read your own diff like a reviewer before requesting one. The
review you don't need to ask for is the cheapest review.

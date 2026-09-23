---
name: architecture-decision-records
description: Capture significant technical decisions as short, dated ADRs — context, decision, consequences — so future readers know why the system looks the way it does. Use when making a decision that future engineers will second-guess, or when you find yourself wondering why a past one was made.
category: workflow
priority: 25
---

# Architecture decision records

Code shows *what* the system does. Tests show *what it should
do*. ADRs show *why it does it that way*. Without them, every
non-obvious choice eventually gets re-litigated by someone who
doesn't have the context — usually badly, under deadline.

## What's an ADR

A short, dated, immutable document recording one decision. Lives
in the repo (e.g., `docs/adr/0007-use-event-sourcing.md`) so it
moves with the code and survives wikis going dark.

Template:

```
# ADR-0007: Use event sourcing for the audit log

Date: 2026-03-15
Status: Accepted
Deciders: @alice, @bob

## Context
What's the situation that forces a decision?

## Decision
What did we decide?

## Consequences
What becomes easier, what becomes harder, what's now committed.

## Alternatives considered
What else we looked at and why we didn't pick it.
```

That's it. ADRs are short by design — usually 1–2 pages. Longer
than that, you're writing a tech spec; link to it instead.

## What deserves an ADR

- **Cross-cutting choices**: framework, language, database,
  serialization, protocol, deployment target.
- **Patterns we'll repeat**: "we use repository-per-aggregate";
  "errors are wrapped at handlers, not at every layer."
- **Non-obvious "no"s**: "we considered GraphQL and chose REST
  because X." This is more valuable than recording a "yes."
- **Boundaries** between modules / services that callers should
  rely on.

What doesn't:

- Day-to-day implementation details that touch one file.
- Style preferences (those are linter rules).
- Anything fully captured by code or tests.

## Status lifecycle

- **Proposed** — under review.
- **Accepted** — current.
- **Superseded by ADR-NNNN** — replaced; keep the old one,
  cross-link.
- **Deprecated** — no longer in force; nothing replaced it.

Never **delete** an ADR. The history is the point. A superseded
ADR explains why the current one looks weird if you don't know
the past.

## Immutability

Once accepted, you don't edit the body — you write a new ADR
that supersedes it. Reasons:

- The original captured what was true *then*; that's data.
- Editing breaks links from PRs / commit messages / external
  docs.
- "We changed our minds" is itself a useful signal; show the
  change, don't hide it.

You may edit metadata (status, "superseded by") on the original.

## Consequences honestly

The Consequences section is the one most people skimp on. Push
yourself to write the *bad* consequences too:

- "Easier to add new event types; harder to query the current
  state of an aggregate without a projection."
- "Locks us in to provider X; switching requires a migration
  ADR."
- "Increases ops surface — we now run a Kafka cluster."

A consequence-free decision is suspicious.

## Anti-patterns

- ADRs only for the easy decisions; the genuinely controversial
  ones never get written down (and get re-litigated forever).
- ADRs written months after the decision was made. Now they're
  archaeology, not records.
- One person decides, no review. The "Deciders" field exists
  because decisions deserve at least two thoughtful people.
- Editing an accepted ADR instead of superseding it.
- ADRs that try to be the comprehensive design doc. Keep them
  short; link out.

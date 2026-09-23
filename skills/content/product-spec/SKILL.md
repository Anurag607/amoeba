---
name: product-spec
description: Write a product spec (PRD) that defines desired behavior, scope, and validation before implementation — outcome, users, scenarios, acceptance criteria, non-goals. Use when starting a non-trivial feature or whenever "what does done mean?" is unclear.
category: workflow
priority: 20
---

# Product spec

Most "spec failures" aren't bad writing — they're missing
sections. A spec that names the outcome but skips scenarios will
ship the wrong feature, on time. The structure below catches the
ambiguities while they're cheap.

## Sections, in order

1. **One-line summary.** What changes for users when this ships?
   If you can't write this, stop and figure it out before
   anything else.
2. **Background / problem.** Who has the problem, how often, and
   what does it cost them today? Cite evidence — support tickets,
   metrics, user quotes — not "I think users want".
3. **Users and use cases.** Which user, doing what, in what
   state, with what goal. Personas are optional; concrete
   scenarios are not.
4. **Goals.** 3–5 sentences. Each is independently verifiable.
5. **Non-goals.** Things this spec *won't* address. Often more
   valuable than the goals — they prevent scope creep at review.
6. **Scenarios.** Step-by-step walkthroughs of the most important
   paths. Include the happy path, the empty state, one failure,
   and one edge case minimum.
7. **Behavior detail.** Per-scenario expected behavior down to UI
   copy and error messages where they matter.
8. **Acceptance criteria.** A checklist that's complete when the
   feature is done. Each item is observable — a user can verify
   it, a test can verify it, or a metric moves.
9. **Open questions.** Honest. Tag with owner and a date.

## Be specific about behavior

The single most common spec failure is "the system handles X
appropriately." That sentence is decorative.

Bad: *"Errors are shown to the user."*
Good: *"On 4xx from the upstream, show a toast with the message
`We couldn't reach <service>. Try again in a minute.` The user
remains on the current screen with their input preserved."*

If you can't write the second version, you don't yet know what
to build.

## Scope cuts in the spec

Decide cuts at spec time, not at "we're behind schedule" time:

- **In v1**: minimum that delivers the outcome.
- **Cut for v1, planned for later**: known follow-ups.
- **Out of scope**: things this feature deliberately won't do.

A spec that "covers everything someone might want" describes
nothing.

## Open questions are not weakness

A spec with 10 honest open questions is healthier than one with
zero. Hidden questions don't disappear; they become incidents.
Tag each:

- The question.
- Who decides.
- By when.
- The default if no decision is made.

## Living document

Specs change as implementation surfaces new information. When
that happens, edit the spec in place; don't ship a feature that
contradicts the spec and pretend they match. Keep a changelog at
the bottom — date, change, why.

## Anti-patterns

- "Spec" that's a screenshot and three bullets.
- Goals that aren't verifiable. "Users love it" — measured how?
- Scenarios for the happy path only.
- Implementation details (which framework, which database) in
  the product spec. That's the tech spec's job.
- "We'll figure it out in implementation." If it's a real
  decision, decide here — implementation is the worst place to
  make product calls under deadline pressure.
- Specs reviewed only by the author's manager. Get an engineer,
  a designer, and someone close to users — they each see
  different gaps.

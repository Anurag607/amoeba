---
name: tech-spec
description: Write a technical design doc after researching constraints — context, options considered, decision with rationale, rollout, risks. Use after the product spec is approved and before significant implementation, or whenever a change crosses module boundaries.
category: workflow
priority: 20
---

# Tech spec

A tech spec turns "we're going to build X" into "here is the
specific plan, here is what we considered and rejected, here is
what could go wrong." Its value is the *thinking it forces*, not
the document.

## Sections, in order

1. **Context.** What problem are we solving? Link the product
   spec. One paragraph; not a re-derivation of the PRD.
2. **Constraints.** What we *can't* change: existing data shapes,
   public APIs, performance budgets, deadlines, team
   capabilities. Constraints make the design space tractable.
3. **Current state.** How the relevant system works today. Code
   pointers, data flow, the parts the design will touch. Be
   factual; this section is auditable against the source.
4. **Options considered.** At least two. For each: sketch, pros,
   cons, cost. If there's only one option, your problem is
   under-specified or your imagination is.
5. **Decision.** The chosen option and the *specific* reasons.
   Reasons match the constraints — "simpler" is not a reason
   unless simplicity was a stated constraint.
6. **Design.** The chosen option in detail: interfaces, data
   model changes, sequence of operations, edge cases.
7. **Rollout.** How does this ship safely? Migration steps,
   feature flag, dark launch, staged rollout, fallback.
8. **Observability.** What new logs, metrics, traces, dashboards,
   alerts? "We'll add them later" is "we won't add them."
9. **Risks and mitigations.** Top 3–5. Each with a likelihood, an
   impact, and a mitigation.
10. **Open questions.** As in `product-spec`, with owners and
    dates.

## Decision rationale

The decision section is the document's reason for existing.
Future readers want to know *why* the obvious-looking choice
wasn't chosen:

- Tie reasons back to **named constraints**, not gut feel.
- Capture **what would change the decision** — "if upstream
  latency drops below 100ms p99, the simpler option becomes
  viable."
- A decision with no rationale is a decision you'll redo every
  time it's questioned.

## Rollout is not "merge and watch"

Cover at minimum:

- **How to deploy** safely — order of changes, dependencies, can
  it be done in one PR or does it need a sequence?
- **How to roll back** — and how long that takes. If rollback
  takes 4 hours, the change isn't really revertable; design
  differently.
- **Feature flag / canary** — what's the rollout cohort, how is
  it widened?
- **Migrations** — backfills, dual-writes, cutover plan, time to
  complete on production data size.

## Right-size the spec

A 30-page spec for a 30-line change wastes everyone's time. A
one-paragraph spec for a 6-month migration is malpractice. Match
depth to risk and reversibility:

- **Reversible, small surface**: a short note in the PR
  description may be enough.
- **Reversible, large surface**: a tech spec, modest depth.
- **Hard to reverse** (data migration, public API change,
  protocol change): a deep tech spec; expect to revise it twice
  before approval.

## Anti-patterns

- A spec that lists implementation tasks but skips the design.
  That's a project plan, not a tech spec.
- "Options considered" with one straw-man bad option and the
  preferred option. Reviewers see through this.
- Skipping rollout and observability because "they're standard."
  They're standard the way fire extinguishers are standard:
  missing means trouble.
- Writing the spec after the code is half-written. The spec was
  there to find the bugs *before* you wrote them.
- Spec that gets approved by one reviewer in 5 minutes. The
  approval isn't proof of quality; it's proof you didn't get a
  real review.

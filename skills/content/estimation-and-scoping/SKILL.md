---
name: estimation-and-scoping
description: Estimate work realistically — break down, include the unfun parts, use ranges not points, name what would invalidate the estimate. Use when committing to a deadline, scoping a milestone, or negotiating cuts under pressure.
category: workflow
priority: 22
---

# Estimation and scoping

Estimates are wrong on average. The goal isn't precision; it's
*calibrated* wrongness — knowing how wrong, and in which
direction. With that, you can plan; without it, you're guessing
in public.

## Break down first

A single big estimate is a wish. Split into chunks small enough
that you can imagine doing each:

- The riskiest sub-problem.
- The data shape / schema work.
- The implementation itself.
- The "unfun" parts (auth, validation, error paths, observability).
- Tests at each layer.
- Migrations / backfills.
- Docs, changelog, release.
- Integration / staging / rollout.

The unfun parts are usually 30–60% of total time and the most
commonly forgotten.

## Use ranges, not points

`5 days` is a hope. `3–8 days, with 5 most likely` carries
information about uncertainty. Communicate ranges to anyone
making decisions on top of the estimate.

A simple format:

- **Likely**: my median guess.
- **Optimistic**: if everything goes well.
- **Pessimistic**: if the riskiest thing bites.

The spread between optimistic and pessimistic is your
uncertainty — wide spread means do more design or a spike before
committing.

## Name what would invalidate the estimate

Every estimate has hidden assumptions. List them:

- "Assumes the upstream API is what the docs say."
- "Assumes no schema change beyond the new table."
- "Assumes I have access to staging by Tuesday."

Each assumption is a *re-estimate trigger*. When one breaks, you
update the estimate; you don't push through pretending it didn't.

## Two-stage estimation

For anything past a week:

1. **First pass** (15 min): split, range each chunk, sum.
2. **Spike** the riskiest 1–2 chunks for half a day to a day —
   a throwaway prototype, a hard read of the upstream API.
3. **Second pass**: re-estimate with what you learned.

The spike pays for itself: it usually finds the thing that would
have blown the estimate by 3×.

## Padding ≠ honesty

Don't pad the number silently. Better:

- Estimate honestly.
- Add explicit **contingency** as a separate line item, sized to
  uncertainty (e.g., 20% on a well-known surface, 50% on a new
  one).
- Now the recipient knows *what's planned work* vs *what's
  buffer* and can negotiate intelligently.

A single padded number gets cut by leadership "because there's
slack in there"; an explicit contingency line gets respected.

## Scope cuts under pressure

When the deadline is firm and the estimate doesn't fit, you
*always* have these cuts available:

- **Smaller v1** — fewer scenarios, fewer surfaces.
- **Reduce quality bar** — defer i18n, defer accessibility (with
  documented follow-up), keep them but ship later.
- **Ship behind a flag** — release the date, expose to fewer
  users.
- **Move the deadline** — sometimes the deadline is the
  negotiable variable.
- **Add people** — usually slows the team for weeks; rarely the
  right cut.

Name the cuts you're recommending and the cost of each. "We can
hit the date if we cut X, Y; here's what users won't get."

## After: calibrate

Track estimate vs actual. Over time you'll learn your personal
multiplier ("I estimate 1.4× too optimistic on infra work"). The
team's multiplier is a useful planning number too.

## Anti-patterns

- One-number estimates for multi-week projects.
- "It'll be done by Friday" with no breakdown. Friday of which
  week, by which definition of done?
- Estimating with the team that won't do the work.
- Refusing to estimate because "it's impossible to know." Give a
  range and the assumptions; that's the job.
- Re-using last quarter's estimate after the spec changed.
- "We'll just work harder." Schedules don't bend; quality does.

---
name: planning-breakdown
description: Turn an ambiguous goal into ordered, verifiable execution steps — each with a clear done condition and the smallest scope that proves direction. Use when starting a non-trivial change, scoping a feature, or rescuing a stalled effort.
category: workflow
priority: 20
---

# Planning and breakdown

Most stuck work is under-decomposed work: the task is described at
"build the auth system" level, no individual step has a clear done
condition, and progress is invisible. The cure is a layered plan
with verification at every boundary.

## Plan in layers

1. **Outcome.** One sentence: what is true when this is done? If
   you can't write it, you don't understand the goal yet — clarify
   before designing.
2. **Minimum viable slice.** What is the smallest end-to-end path
   that proves direction? Ship that first; you'll learn more from
   one running thing than from three half-finished ones.
3. **Phases.** Order the rest. Each phase ends at a *visible
   verification point* — a test passing, a demo runnable, a metric
   moving.

## Per-phase spec

A phase that doesn't have all four of these will stall:

- **Goal.** What changes about the system?
- **Surface.** Which files, modules, or services are touched?
- **Dependencies.** What must be true first? What downstream work
  blocks on this?
- **Verification.** How will we know it's done? Test name, metric
  name, demo step. If it's "looks right", it's not specified.

## Rules of thumb

- If a task can't be verified, it isn't specified enough.
- If a task spans multiple unrelated concerns, split it.
- If a step takes longer than half a day to review mentally, it's
  too big.
- Identify the **riskiest sub-problem first** and de-risk it early
  — a prototype, a spike, a hard read of the upstream API. Don't
  let the unknown sit at the end of the plan.

## Re-planning

The plan is a working hypothesis. When reality disagrees, replan,
don't push through. Signals you need to replan:

- Two consecutive phases came in much larger than estimated.
- A step you thought was independent turned out to depend on
  something not in the plan.
- The verification step for an early phase doesn't actually verify
  what you assumed.

Edit the plan in the same place you wrote it; don't keep a stale
plan and a "real" plan in your head.

## Anti-patterns

- "Plan" that is one bullet: "do the feature."
- All phases sized the same because the template said so.
- Verification = "code review approved." Reviewers approve diffs,
  not goals.
- Skipping the minimum viable slice in favor of the "real"
  architecture, then never getting to a runnable system.
- Replanning silently — the old plan and the new disagree, nobody
  knows which is current.

---
name: risk-assessment
description: Surface project risks early — likelihood × impact, owner, mitigation, trigger — and track them as the plan unfolds. Use when planning a non-trivial change, before any rollout, and during the kickoff of any cross-team project.
category: workflow
priority: 22
---

# Risk assessment

A risk you've named, sized, and assigned an owner is manageable.
An unnamed risk is an incident in waiting. Most "surprises" in
projects were known to someone — they just weren't written down.

## What counts as a risk

Anything that, if it happens, would invalidate the plan, the
deadline, or the design. Categories worth scanning:

- **Technical**: unknown library behavior, scale unknowns, a
  dependency that might not exist yet, performance unknowns.
- **Integration**: another team's deliverable, a third-party
  API, an org-wide migration in flight.
- **People**: key person availability, ramp-up time on
  unfamiliar code, vacation overlap with deadline.
- **External**: customer / market timing, regulatory deadline,
  press / launch coupling.
- **Operational**: rollback complexity, on-call coverage,
  monitoring gaps.

If a category produces nothing, you haven't looked hard enough —
not "there are no risks."

## Size each risk

Two axes, three buckets each (Low / Medium / High):

| Likelihood × Impact | L | M | H |
|---|---|---|---|
| **H** | watch | mitigate | mitigate hard |
| **M** | watch | mitigate | mitigate |
| **L** | accept | watch | mitigate |

Numbers are fake precision; the discipline is forcing yourself
to think about both axes. A H-impact / L-likelihood risk you
*accept* (and document); you don't ignore it.

## Per-risk fields

- **Description**: one sentence, plain language.
- **Likelihood / Impact**: as above.
- **Owner**: a person, not a team.
- **Mitigation**: what we'll do *now* to reduce likelihood or
  impact.
- **Trigger**: the observable signal that says "this risk is
  becoming real." Without a trigger, you'll only notice in
  retrospect.
- **Contingency**: the plan if it does happen.

Five fields. Anything less is decorative.

## Mitigation patterns

Common moves, in roughly increasing cost:

- **Reduce uncertainty** with a spike or prototype.
- **Add observability** so you'll see the risk early.
- **Decouple** so the risky piece can fail in isolation.
- **Provide a fallback path** if the risky piece breaks.
- **Insure** with redundancy, retries, alternate vendors.
- **Reshape the plan** to avoid the risk entirely.

Pick the cheapest mitigation that gets you below your acceptable
threshold. Over-mitigating is its own form of waste.

## Track them

A risk list is a living document, reviewed at standups or
weekly. For each:

- Has anything changed about likelihood / impact?
- Has the trigger fired?
- Is the owner still the right person?
- Should we accept it, escalate it, or close it?

Risks that never change for weeks are either resolved (close
them) or being ignored (escalate them).

## Pre-mortem

Once, before kickoff: imagine it's six months from now and the
project failed. Spend 30 minutes brainstorming why. The
imagination of failure surfaces risks the optimistic plan
doesn't. Add the survivors to the list.

## Communicating risk

Up to leadership, down to the team:

- **What it is** — plain language.
- **What we're doing** — mitigation in flight.
- **What we need** — decision, resource, or "nothing, just
  awareness."
- **When you'll know more.**

Risk reports without the "what we need" line are noise.

## Anti-patterns

- A risk list with no owners. Risks belong to people, not
  spreadsheets.
- Mitigation = "be careful." Be specific.
- Sizing every risk High because "everything matters." That's
  the same as sizing nothing.
- One-time risk assessment at kickoff, never revisited. Risks
  evolve; the document must too.
- Risks invisible to stakeholders until they fire. Surface
  early; nobody likes the "by the way" before a launch.
- Treating risk as pessimism. It's a planning input, not a mood.

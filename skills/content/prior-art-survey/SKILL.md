---
name: prior-art-survey
description: Survey existing solutions, libraries, papers, and prior projects before designing — what's been tried, what worked, what didn't, what's reusable. Use before any non-trivial new system, when a problem feels too generic to be original, or when choosing between build vs adopt.
category: workflow
priority: 24
---

# Prior-art survey

Most problems you'll face have been faced before, possibly badly,
possibly well. A short survey of what others did is a
disproportionately good investment — it surfaces failure modes
you'd otherwise rediscover and patterns you'd otherwise reinvent.

## When to do one

- Before starting any new system or significant component.
- When picking a library or framework.
- When stuck on a design — "how have others solved this?"
- When evaluating build vs adopt vs adapt.
- When a problem feels generic — "logging," "auth," "queue,"
  "rate limit" — others have absolutely solved it.

When *not*:

- For a 30-minute fix.
- When the problem is genuinely novel — rare; double-check.
- When the survey would take longer than just building the
  smallest working version.

## What to survey

- **Open-source libraries / frameworks** in your language and
  one neighbor.
- **Industry write-ups** from companies who solved this at scale
  — engineering blogs, conference talks, postmortems.
- **Standards / RFCs / specs** if the problem is in a
  standardized space (protocols, formats, crypto, auth).
- **Academic papers** when the problem is genuinely hard
  (consensus, scheduling, ML approaches). Don't skip; the field
  often has decades of prior thought.
- **Your own org's prior projects.** A project nobody
  remembers may have already shipped 60% of this.

## Triage the field

You don't need to read everything. Skim wide, read deep:

1. **List candidates** — 10–20 from search + docs + asking
   colleagues.
2. **One-line summary** of each — "what problem does it claim to
   solve, on what scale?"
3. **Cut to the top 3–5** based on fit with your constraints.
4. **Read the top 3–5 deeply** — README, recent issues / changelog,
   the actual interface, a couple of postmortems if any.

The cuts matter. A survey that "covers the field" without
prioritizing produces a long list with no decision attached.

## What to extract from each

- **Approach** in one paragraph.
- **Strengths** they emphasize.
- **Weaknesses** observed — issues, complaints, missing features,
  ops cost, license, maintenance status.
- **Lessons** that apply to your situation, whether you adopt or
  build.
- **Fit with your constraints** — language, scale, dependencies,
  team familiarity.

Build a comparison table when you have 3+ candidates. Same axes
for each so the comparison is honest.

## Build vs adopt vs adapt

For each viable candidate, decide:

- **Adopt** — use it as-is. Cheapest if fit is good.
- **Adapt** — fork or wrap. Pay the maintenance tax forever.
- **Build, learning from it** — implement your own with their
  lessons applied. Most expensive, justified only when fit is
  poor or vendor risk is high.

Bias toward adopting; teams systematically underestimate the
total cost of maintaining their own implementation.

## Health signals for a candidate

Especially for libraries:

- **Recent commits / releases.** Dead since 2022 is dead.
- **Issue triage.** Open issues stale for years, no responses?
  Bus factor of one.
- **Number of dependents** in the package registry. Popular ≠
  good but unpopular is a risk multiplier.
- **License compatibility** with your project. Check; don't
  assume.
- **Security history.** CVEs handled openly, with fixes? Or
  silently?
- **Documentation quality.** Bad docs cost real time.

A library that fails three of these is a foot-gun even if it
nominally fits.

## Output

A short doc that becomes input to the tech spec or ADR:

- Question you were answering.
- Candidates considered.
- Comparison.
- Recommendation with reasoning.
- What still needs validation (often a spike).

## Anti-patterns

- "Not invented here" — building from scratch without surveying.
- "Found here, must be good" — adopting the first popular
  result without checking fit.
- Skimming README and stopping. Read the issues; they tell the
  truth.
- Survey with no decision at the end. That's homework, not a
  survey.
- Treating a paper / blog as proof a thing works. It worked for
  them, at their scale, with their constraints. Verify yours.
- Choosing by GitHub stars. Stars correlate with hype, not
  fitness.

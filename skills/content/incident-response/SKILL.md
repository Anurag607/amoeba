---
name: incident-response
description: Triage and resolve a live incident with discipline — confirm, scope, mitigate, communicate, then investigate root cause. Use during production incidents, severe regressions, or any "something is broken right now" report.
category: workflow
priority: 12
---

# Incident response

During an incident, the temptation is to start typing fixes
immediately. The faster path is: spend the first five minutes
*understanding*, then act.

## First five minutes — triage

1. **Confirm it's real.** One alert is a hypothesis. Reproduce
   from a second signal — a synthetic check, a user report, a
   dashboard. False alarms eat hours.
2. **Scope it.** Who is affected? How many? Which region, which
   tenant, which version? Bound the blast radius before mitigation.
3. **Severity.** P0 (everyone broken) demands different actions
   than P2 (one customer slow). Don't auto-escalate or
   auto-downgrade — assign based on impact.
4. **Communicate.** Open the incident channel / page on-call /
   start the status doc. *Now*, not after you have an answer.
   Silence during an incident is the second-worst outcome.

## Mitigate before diagnosing

Recovery first, root cause second. Acceptable mitigations:

- Roll back the last deploy.
- Toggle the feature flag.
- Failover to standby.
- Drop traffic from the bad caller.
- Restart the affected service (acceptable as mitigation, *not*
  acceptable as the final answer).

Tag every mitigation as a *temporary* action. Don't let "we
restarted it" become the permanent fix.

## Investigate

Once mitigated, slow down:

- **Timeline.** Reconstruct what changed when. Deploys, config
  pushes, traffic shifts, upstream incidents.
- **Evidence first.** Logs, metrics, traces, stack samples. Don't
  reason from intuition when data is one query away.
- **One hypothesis at a time.** "It might be A or B or C" is
  three investigations — run them in parallel only with explicit
  owners.

## Communicating during the incident

- Post updates on a fixed cadence (e.g., every 15 min) even if the
  update is "still investigating." Predictability beats new
  information.
- State **what is known, what is being tried, what to expect
  next** — not "we're looking into it."
- Separate facts from speculation. "Latency p99 is 4× normal" is a
  fact. "Probably the new release" is a hypothesis.

## After

- **Mitigation ≠ fix.** File the follow-up to actually fix the
  underlying bug.
- **Blameless postmortem.** Focus on the system (alerts, gates,
  tests, rollbacks), not the person who pushed the commit.
- **Action items have owners and dates.** Anything else is a
  wishlist.

## Anti-patterns

- Typing fixes into prod while still trying to confirm the
  problem.
- Going silent because you "don't have an update yet."
- One person heads-down debugging; nobody else knows what's been
  tried. Designate a coordinator.
- Declaring resolved at the moment metrics recover, without
  watching for recurrence.
- Postmortem that blames an individual. You will get fewer
  postmortems next time.

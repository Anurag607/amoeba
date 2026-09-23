---
name: debugging-workflow
description: Find and fix bugs with a structured loop — read the error, reproduce, narrow, identify root cause, smallest fix, regression test — instead of guessing or shotgun-patching. Use whenever a bug report or failing test lands.
category: workflow
priority: 15
---

# Debugging workflow

The single biggest debugging failure is skipping straight to a fix.
You stop reading at "looks like a race condition," guess, change
three things, and now you have a different bug. The discipline below
is faster on the median bug, much faster on the hard ones.

## The loop

1. **Read the error fully.** All of it. Stack trace, log lines on
   either side, the exact request id, the timestamps. Don't
   reformulate — read.
2. **Reproduce.** A bug you can reproduce in <10 seconds is almost
   always solvable. If you can't reproduce yet, that is the first
   sub-problem. Don't move on without it.
3. **Narrow.** Which layer? Which file? Which line? Bisect commits
   when "it used to work." Bisect inputs (smaller, smaller) when
   the bug only fires sometimes.
4. **Identify root cause.** Why does the failing line fail *given
   the inputs you observed*? Stop one level above where you think
   you understand it: ask "and why is that?" once more.
5. **Smallest fix.** Change one thing. The fix should be smaller
   than the bug report. If it isn't, you're refactoring, which is a
   separate commit.
6. **Regression test.** If this bug could plausibly recur, add a
   test that fails before the fix and passes after.

## Fast techniques

- **Structured logging at state transitions.** Print the inputs
  and the chosen branch at every fork — far more useful than
  printf-debugging the final value.
- **Minimal reproduction.** Strip until the failure goes away, then
  add back the last thing. The result is a 10-line repro you can
  share.
- **Binary search through the call path.** Disable / mock half the
  code; if the bug persists, it's in the other half.
- **Diff the working vs broken.** Same input, different environment;
  capture both call traces and compare. Often the bug is in
  config, not code.

## When you're stuck

- **Stop and explain it.** Out loud, or in a chat draft. Rubber-duck
  debugging works because writing forces you to name the
  assumption you didn't realize you were making.
- **Question one assumption.** Pick the cheapest-to-verify
  assumption ("the database is reachable", "the config is what I
  think") and verify it directly. Most "impossible" bugs are a
  violated assumption.
- **Take a break.** Ten minutes off often beats two more hours
  staring.

## Anti-patterns

- "Restart and retry." Fine for production triage; useless for
  debugging — it destroys the evidence.
- Changing the test instead of the code.
- Catching the error and continuing without understanding why it
  fired. The bug is still there.
- Multiple changes in one commit so you can't tell which one fixed
  it.
- Declaring "fixed" without reproducing the original failure first.

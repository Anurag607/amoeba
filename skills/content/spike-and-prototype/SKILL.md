---
name: spike-and-prototype
description: De-risk uncertainty with a time-boxed, throwaway experiment whose deliverable is a decision, not code. Use when an estimate has a wide range, when a library's behavior is unknown, or when stakeholders disagree on feasibility.
category: workflow
priority: 22
---

# Spike and prototype

When you don't know enough to plan, planning more is the wrong
move. A spike — a small, time-boxed experiment with a specific
question to answer — beats another week of speculation.

## A spike is not implementation

The deliverable of a spike is a **decision** with evidence, not
production code. Conflating the two produces the worst of both:
throwaway-quality code that ships anyway.

Going in, write down:

- **The question** the spike will answer. Specific. "Can this
  library handle 10k WS connections on a 4-core box?" is a
  spike question. "Is this library good?" is not.
- **The time box.** Half a day, a day, three days. The box is
  the budget; when it's up, you have your answer or you decide
  to extend.
- **The success criteria** for the answer. What does "yes" look
  like? What does "no" look like? What looks like "we need a
  second spike"?
- **What you'll throw away.** Usually all the code. Knowing this
  up front frees you to cut corners that real code wouldn't
  allow.

## Cut every corner

Spike code:

- Hardcodes things real code parameterizes.
- Skips error handling beyond what's needed to observe the
  result.
- Has no tests beyond a manual "did the question get answered?"
- Doesn't follow style guides.
- Lives on a throwaway branch you may not even push.

The code is a means, not an output. Resist the temptation to
"clean it up and merge" — see anti-patterns below.

## What to record

Even though the code is throwaway, the *findings* are valuable.
Write a short writeup:

- The question.
- What you tried.
- What you observed (with concrete numbers / errors / outputs).
- The answer to the question.
- What you'd do differently in the real implementation.
- Open follow-ups.

This goes in the tech spec, ADR, or project doc — wherever
future readers will look for "why did we pick this?"

## Types of spike

- **Feasibility**: can this library do X at all? Smallest possible
  program that exercises X.
- **Performance**: does it work at our scale? Build a realistic
  load profile and measure.
- **Integration**: how does this dependency *actually* behave at
  the seam? Often surfaces undocumented behavior.
- **UX**: does this interaction feel right? Clickable mock,
  Figma flow, paper prototype — whichever is cheapest.
- **API exploration**: what shape do callers want? Sketch
  signatures, share with future users, get reactions.

Different shapes; same discipline.

## When to extend the time box

Sometimes the answer doesn't come within the box. Two valid
choices:

1. **Extend explicitly**, with a new box and a written reason
   ("we got 80% there, one more day").
2. **Stop and replan**, with what you learned, possibly
   substituting a different approach.

The wrong choice is silently extending — the box becomes the
project, the project becomes a deliverable, the throwaway code
ships.

## Sharing the result

Show, don't tell. The most useful spike outputs:

- Numbers (latency, memory, throughput) with the test setup
  beside them.
- Code snippets showing the awkward parts of the API the docs
  didn't.
- Screen recordings for UX spikes.
- A clear yes/no on the original question.

Five minutes of "here's what I found" beats a five-page report.

## Anti-patterns

- A spike with no question. Just "exploring." Two weeks later,
  no decision is closer.
- Promoting spike code to production "to save time." You
  inherit every shortcut.
- A spike that's actually the implementation in disguise. If
  you're not willing to throw it away, set expectations
  honestly: it's a prototype-to-production, with that timeline.
- A six-week spike. That's a project; plan it as one.
- No writeup. Whatever you learned dies when the branch is
  deleted.

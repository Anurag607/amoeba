---
name: requirements-gathering
description: Turn a vague request into a workable spec by asking the right clarifying questions — users, success, scope, constraints, edge cases — and surfacing assumptions before they become bugs. Use at the start of any feature, after a customer request, or whenever you catch yourself saying "I think they want…".
category: workflow
priority: 18
---

# Requirements gathering

The most expensive bugs come from misunderstood requirements, not
bad code. The fix is to spend an hour at the start asking
questions whose answers would otherwise cost a week.

## The questions, in order

Ask in this sequence. Skipping early ones makes the later ones
unanswerable.

### 1. Who and why

- Who is this for? Specific users, not "users."
- What are they trying to accomplish? In their words.
- What do they do today, badly? Workarounds reveal real needs.
- How often does this come up? Daily / weekly / quarterly
  changes how much polish it deserves.

### 2. Success

- How will *they* know it worked?
- How will *we* know it worked? Metric, dashboard, support
  ticket count, NPS — name it.
- What does "good enough for now" look like vs "really done"?

### 3. Scope

- What's in?
- What's deliberately out?
- What's a v2 thing we'd love but won't build now?
- What can we cut if we run out of time?

### 4. Constraints

- Deadline? Why that one — is it real or aspirational?
- Budget — tokens, dollars, headcount, ops surface.
- Compliance, security, legal — anything that has to be true?
- Existing systems we must not break or migrate?

### 5. Edges

- Empty state — first-time user with nothing.
- Failure state — upstream is down.
- Boundary state — max size, min size, ambiguous input.
- Adversarial state — someone abuses this. What's acceptable?

### 6. Non-functional

- Latency target? Under what load?
- Availability target? Maintenance windows OK?
- Audit / compliance logging?
- Internationalization, accessibility from day one or later?

## Surface assumptions

Every "obviously" is an assumption. Write them down explicitly:

- "We're assuming X is rare enough that we can show an error."
- "We're assuming the user has Y permission."
- "We're assuming the upstream returns Z."

Then check the ones that are cheap to check. The expensive ones
become risks to track.

## Clarifying questions are not weakness

Engineers worry that asking questions looks uncertain. Senior
engineers know the opposite — the engineer who asks five sharp
questions and ships the right thing is more valuable than the one
who didn't ask and shipped the wrong thing fast.

Frame questions concretely:

- Bad: "Can you tell me more about the feature?"
- Good: "If a user uploads a 2 GB file, do we want to reject
  immediately, accept and queue, or accept and stream-process?"

A concrete question forces a concrete answer.

## When the answer is "I don't know"

Common and fine. Convert each into:

- An **assumption** you're proceeding with.
- A **default** behavior chosen now.
- A **decision point** later when reality answers.

Document all three. Don't let "I don't know" turn into "I
guessed and we'll find out in production."

## Anti-patterns

- Going straight to design from a one-sentence request.
- Asking questions only by email and accepting silence as
  assent. Get on a call for the ambiguous ones.
- Asking 30 questions at once. Nobody answers all 30; you get
  the easy ones and the important ones get lost. Batch by topic.
- "We can iterate" as a substitute for understanding. Iteration
  costs more when you started in the wrong place.
- Treating the loudest stakeholder's view as the requirement.
  Talk to the actual users.

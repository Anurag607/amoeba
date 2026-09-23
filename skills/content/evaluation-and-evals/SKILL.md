---
name: evaluation-and-evals
description: Build a golden-set eval harness for an agent that catches regressions before users do — fixed inputs, scored outputs, tracked over time. Use when prompts/models/tools change frequently or before any production rollout of an agent.
category: agent
priority: 25
---

# Evaluation and evals

Without evals, every prompt or model change is a vibe. With them,
you change one knob, you see the delta, you ship or revert. Evals
are the single highest-leverage investment for an agentic system.

## A minimal eval

You need three things and nothing else to start:

1. **A frozen set of inputs.** 30–100 hand-picked examples
   covering critical paths, common cases, edge cases, and a few
   known-failure cases. Add to it over time; never silently
   change it.
2. **A scoring function.** What does "correct" mean for this
   task? Exact match, structural match, LLM-as-judge with a fixed
   rubric, or human review.
3. **A run command.** One command, deterministic seeds, runs the
   agent against the set and emits a score.

Track scores per commit. A red bar in CI on the eval is a
regression, treated like a test failure.

## Score types, picked by problem

- **Exact** / **regex** match when the output is structured and
  deterministic (classification, extraction). Cheapest, most
  reliable.
- **Structural match** — JSON parses, required fields present,
  enum values valid. Good for tool-call evaluation.
- **Reference-based** — BLEU/ROUGE/embedding similarity vs a gold
  answer. Useful for summarization, brittle for code.
- **LLM-as-judge** with a closed rubric and a strong model. Good
  for open-ended answers; budget it.
- **Human review** for the hard 10% that the above can't score.
  Sample, don't grade everything.

Combine when useful — a structural gate first (cheap), an
LLM-judge after (expensive) — short-circuit on structural failure.

## What to put in the set

- **Critical paths**: the things this agent is *for*. Most
  examples live here.
- **Known-hard cases** that previous versions failed on.
- **Edge cases**: empty input, very long input, ambiguous intent,
  user being adversarial.
- **Refusals**: cases where the right answer is "I can't help
  with that" — measured the same way.
- **Drift canaries**: a handful of trivially-correct cases that
  should *never* regress. If they do, something broke at infra,
  not prompts.

Tag each example so you can break the score down by category.

## Process

- **Per-PR eval** on the changed surface (cheap subset).
- **Nightly full eval** on the complete set.
- **Pre-release eval** with stricter thresholds before any
  rollout.

Display per-category scores, not just the aggregate. A 2% drop in
the aggregate can hide a 30% collapse on refusals.

## Calibrate LLM-as-judge

Judges drift, agree too easily, or favor verbose answers. To
trust one:

- **Pin the judge model and prompt.** Versioned.
- **Calibrate on a labeled subset** — run the judge against
  human-labeled examples and report agreement.
- **Use a closed rubric** (1–5 with explicit anchors), not "rate
  this answer."
- **Re-calibrate** when you change models or prompts.

## Anti-patterns

- Evaluating only the happy path. Production isn't.
- Adding examples until the score is green. That's training,
  not evaluating.
- Single aggregate number with no breakdown. Hides regressions.
- LLM-judge with no anchor: "is this answer good?" Useless.
- Running evals only locally, never blocking merges. You will
  regress and not notice.
- Evaluating on examples the prompt was tuned on. Hold out, or
  the eval is just memorization.

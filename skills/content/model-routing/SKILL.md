---
name: model-routing
description: Pick which model handles which sub-task — fast/cheap for routing and classification, strong for reasoning and synthesis — with a fallback chain and a cost ceiling. Use when designing a multi-model agent (MoE, planner-executor, router) or auditing latency/cost regressions.
category: agent
priority: 10
---

# Model routing

A single model handling every step is the easiest design and almost
never the best one. Routing splits work so cheap models do the
cheap parts and expensive models earn their cost on the parts that
matter. Done well, this is the highest-leverage optimization in an
agentic system.

## Two-tier baseline

Start here before anything fancier:

- **Fast tier.** Used for classification, intent extraction,
  routing decisions, short rewrites, and any step where the
  output is structured and verifiable. Latency budget: sub-second.
- **Strong tier.** Used for planning, synthesis, multi-step
  reasoning, code generation, and final answers. Latency budget:
  seconds, possibly with streaming.

Default: route → fast. Reason / write → strong. Resist the
temptation to send everything to the strong model "just to be
safe"; it costs 10–50× and rarely changes correctness on the easy
steps.

## Routing decisions

The router is itself a fast-tier call. Its job is to pick:

- **Which expert / sub-agent** handles this turn (code, math,
  search, summarize).
- **Which tier** that sub-agent should use.
- **Whether to short-circuit** — e.g., recognize a greeting and
  reply directly with no expert call.

Keep the router's output schema small and closed (enum of expert
ids + optional flags). Free-form router output is a fragility
source.

## Fallback chain

Every model call has at least three things that can go wrong:
provider 5xx, timeout, output that fails validation. Design for
all three:

1. **Same tier, different provider** if you have one.
2. **Stronger tier** as a last-resort retry (rare; expensive).
3. **Deterministic fallback** — a templated answer, a tool refusal,
   a "please rephrase" — never a silent failure.

Always cap total retries with a budget (count + wall clock).

## Cost ceiling

Per-task budget in tokens *and* wall clock, enforced in code:

- Track tokens spent in the loop; abort cleanly when the budget is
  exceeded.
- Distinguish the budget per *user turn* from the per-*session*
  cap; runaway loops are usually session-level.
- Log the budget burn alongside latency; cost regressions hide in
  averages.

## Avoid

- One giant model doing routing inline with its reasoning. Now you
  can't change the routing without retraining your prompt.
- Provider failover that silently changes model behavior — same
  prompt, different output shape, downstream parsing breaks.
- Routing decisions based on string matching when a 200ms classifier
  call would be both more accurate and easier to update.
- Streaming the fast-tier router's output to the user. Routing is
  internal; users see the chosen expert's output.

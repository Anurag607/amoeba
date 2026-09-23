---
name: structured-output-validation
description: Validate model output against a schema before acting on it, and design a recovery loop for invalid output instead of crashing. Use whenever an agent's output drives code execution, persistence, tool calls, or downstream prompts.
category: agent
priority: 8
---

# Structured output validation

Treat model output the same way you treat user input: untrusted,
possibly malformed, occasionally malicious. The model will
eventually return JSON with a trailing comma, an unexpected field,
or a missing required key. Plan for it.

## Why this matters

Every agent failure mode below comes from acting on unvalidated
output:

- Crashes when a field is the wrong type.
- Silent corruption when an enum value is misspelled (`"aproved"`).
- Code execution from a tool argument the model invented.
- Loop divergence when the planner emits free-form text where you
  expected a plan.

## Validate at the boundary

Right where the model output enters your code:

1. **Parse** — JSON.parse / equivalent, with try/catch. A parse
   failure is a known recoverable error, not a crash.
2. **Schema-validate** — closed schema (JSON Schema, zod, pydantic,
   Go struct + validator, `encoding/json` with `DisallowUnknownFields`).
3. **Semantic-validate** — values are in the allowed set, IDs
   exist, references resolve, sizes are bounded.
4. **Only then act.**

Skipping step 2 because "the prompt says to return JSON" is the
single most common production bug in agentic systems.

## Recovery loop

When validation fails, don't crash and don't silently fall back to
a stub. Re-prompt with a *specific* repair instruction:

```
Your previous reply did not match the required schema.
The error was: "status" must be one of [pending, approved, rejected];
got "aproved". Please re-emit only the JSON, with that field fixed.
```

Cap retries (2–3). Beyond that, return a clean error to the
caller — runaway repair loops burn budget and rarely converge.

## Schema design tips

- **Closed enums** for any categorical field. Open strings are
  validation-by-vibes.
- **Required vs optional is enforced**, not aspirational. If your
  schema marks a field required, reject calls without it.
- **Reject unknown fields** by default. They're either model
  hallucinations or signs the schema drifted.
- **Bound the sizes** — max array length, max string length, max
  nesting depth. Unbounded fields are denial-of-service vectors.

## Multiple competing strategies

In rough order of robustness:

1. **Provider-side structured output** (JSON mode, schema-grammar
   constrained decoding). Best when available.
2. **Tool / function calling** with a typed signature. Provider
   enforces the shape at decode time.
3. **Free-form text + parse + validate**. Works everywhere but
   fragile; always pair with the recovery loop.

## Anti-patterns

- `JSON.parse(reply)` with no try/catch.
- Trusting that "the model usually gets this right." Usually
  isn't a contract.
- Validating shape but not values — a `user_id` of `"admin' OR
  1=1"` is structurally fine and operationally lethal.
- Recovery loops with no retry cap.
- Different schemas for "what we ask for" and "what we accept."
  They drift.

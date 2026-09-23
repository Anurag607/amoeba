---
name: logging-and-observability
description: Emit logs and metrics that diagnose production problems — structured fields, the right level, redaction at the source, traces across boundaries. Use when adding logging to a new feature, debugging a production incident, or cleaning up a noisy log stream.
category: workflow
priority: 20
---

# Logging and observability

A log line is only useful if a future debugger can find it,
trust it, and act on it. Most logging is none of those things:
unstructured text nobody greps, fields that aren't actually fields,
secrets leaking in error messages, levels chosen by feel.

## Three signals, three jobs

- **Logs**: discrete events with rich context. Best for "what
  happened and why" on a per-request basis.
- **Metrics**: aggregated counters / gauges / histograms. Best for
  "is the system healthy?" and alerting.
- **Traces**: causality across services / async boundaries. Best
  for "where did the time go?" and "what called what?"

If you only have logs, you're guessing on health. If you only
have metrics, you can't debug. You need all three.

## Structured logs by default

Free-form strings are unsearchable at scale. Use the language's
structured logger and emit fields:

```
logger.Info("plan approved",
    "plan_id", planID,
    "user_id", userID,
    "duration_ms", dur.Milliseconds())
```

- One event per line.
- Field names are stable across the codebase (`user_id`, not
  sometimes `userId` and sometimes `uid`).
- Values are the raw types, not pre-formatted strings.

## Levels with meaning

A loose but workable convention:

- **ERROR**: something failed that the operator must look at —
  data loss, unhandled exception, exhausted retries. Wakes
  someone up via alerting.
- **WARN**: a degradation or retry — recoverable, but trending
  matters.
- **INFO**: significant business events — request received,
  decision made, job completed.
- **DEBUG**: detail useful when investigating; off in prod by
  default, toggleable per component.
- **TRACE** (if your logger has it): step-by-step internals.

Misusing levels (INFO for everything, WARN for noise) destroys
the on-call signal-to-noise ratio. The fix is editorial, not
technical.

## Redact at the source

The logger — not the call site — strips secrets, tokens, PII.
See the `secrets-handling` skill. Reasoning: every call site is
one missed redaction away from disaster; the logger is one place
to get right.

## Context that travels

Every log line that belongs to a request should carry:

- A **request id / trace id** generated at the edge, propagated
  through every internal call and async hop.
- A **user / tenant id** when authenticated.
- A **component / module** identifier so cross-cutting logs are
  filterable.

In Go, this is a logger pulled from `context.Context` (set at the
edge); other languages have equivalents. The point is that the
field is there without each call site re-typing it.

## Errors

When logging an error:

- Log it **once**, at the boundary you're handling it at — usually
  the HTTP / queue handler.
- Wrap with operation context as it bubbles
  (`fmt.Errorf("read config: %w", err)`); the logger sees the
  full chain.
- Don't log-and-rethrow at every layer. Each duplicate makes the
  signal worse.

## Metrics worth having

- **RED** for any service surface: Rate, Errors, Duration
  (typically as a histogram, watch p50 / p95 / p99).
- **USE** for resources: Utilization, Saturation, Errors.
- **Domain counters**: per-event counts that the product team
  cares about — distinct from operational metrics.

Cardinality matters. Don't put a user id in a metric label; you'll
explode the index and the bill.

## Traces

- Start a span at every boundary in / out (HTTP server, HTTP
  client, queue producer / consumer, DB call, model call).
- Attach the same request id used in logs.
- Tag spans with the few fields you'll filter by; not everything.

## Anti-patterns

- `fmt.Println` / `console.log` in server code. Unstructured,
  unfindable, unleveled.
- Logging the full request body to "see what came in." That's
  where the secrets and PII live.
- INFO on every iteration of a tight loop. Now your logging
  *causes* the incident.
- Adding a metric whose label is a user id, request id, or url
  path. Cardinality explosion.
- Alerts on raw counters instead of rates. Traffic doubles
  legitimately and you wake up.
- Trace ids generated independently per service. Now nothing
  joins.

---
name: rate-limiting-and-quotas
description: Protect a service with per-identity rate limits, abuse caps, and graceful 429s — token bucket as the default, sized to upstream capacity, never trusting the client. Use when exposing any endpoint a user (or model) can call, or auditing an endpoint for abuse risk.
category: security
priority: 8
---

# Rate limiting and quotas

A service without rate limits has uptime measured in user
patience. Limits aren't only for abuse; they're for the
well-meaning client running a `for` loop, the retry storm on a
flaky network, the buggy mobile app shipped last week. Plan for
all three.

## What to limit, where

- **At the edge** (load balancer / API gateway): coarse, by IP.
  Stops the loudest abuse.
- **At the auth boundary**: by authenticated identity — user,
  tenant, API key. Most useful production limit.
- **Per route**: cheap routes get high limits, expensive routes
  (model inference, batch export) get low ones.
- **Per resource**: writes to a specific record / channel /
  document — prevents thundering on one hot key.
- **Per backend connection**: connection-level caps on
  WebSocket / SSE messages.

Stacking is fine; cheaper layers stop the cheapest attacks.

## Token bucket as the default

Token bucket gives bursts plus a steady rate, which is what real
clients want:

- **Rate** (refill per second) and **burst** (bucket size).
- Refill continuously, not on a schedule edge.
- Reject (or queue, briefly) when empty.

A reasonable WebSocket per-connection default: ~20 rps with a
burst of 40. Per-route HTTP varies — start strict, loosen with
data.

## Identify the caller correctly

The limit is only as good as the identity it's keyed on:

- After auth, **always use the authenticated identity**, never a
  client-supplied header.
- For unauthenticated routes, IP plus a path component, with
  awareness that NAT collapses many real users into one IP.
- Account for proxies — `X-Forwarded-For` only if you trust the
  proxy that set it.

A limit keyed on a header the client controls is decorative.

## Respond clearly

When a request is rejected:

- **HTTP 429** with a `Retry-After` header (seconds or HTTP date).
- A small structured body: `{ "error": "rate_limited",
  "retry_after_ms": 1200 }` — clients can implement backoff.
- Log the rejection with the identity (after redaction) so you
  can spot abuse vs misbehaving client.

## Quotas vs rate limits

Different problems:

- **Rate limit**: requests per second / minute. Smooths bursts.
- **Quota**: requests / tokens / dollars per day / month. Caps
  total cost.

Both matter for agentic systems where a single user turn can cost
real money. Track:

- Per-user daily token budget for model calls.
- Per-user daily tool-invocation cap on expensive tools.
- Soft warning at 80%, hard cut at 100%, with a clear UI message.

## Size to upstream

If your limit is more permissive than what your downstream can
serve, you've moved the failure mode without changing it. Size
inbound limits to upstream capacity, not to "what feels nice."

## Anti-patterns

- "We'll add rate limits when we need them." You'll add them
  during the incident.
- Limits that count requests but not request *cost*. One
  expensive request can be worse than a thousand cheap ones.
- Limits that count successes only. The attacker generates
  errors and never gets stopped.
- 200 OK with a "you're being rate limited" body. Clients can't
  detect it; CDNs cache it.
- A single global limit. One noisy customer ruins the day for
  everyone else.
- Limits in-memory on a multi-instance service. Each instance
  has its own bucket; effective limit is N×. Use a shared store
  (Redis, etc.) or partition by client to a fixed instance.

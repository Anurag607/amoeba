---
name: caching-strategy
description: Cache deliberately — name the key, name the invalidation, name the staleness budget — and prefer not caching when in doubt. Use when adding a cache layer (prompt cache, retrieval cache, HTTP cache, DB read-through) or debugging stale-data bugs.
category: workflow
priority: 22
---

# Caching strategy

Caches are bug factories that occasionally improve performance.
The improvement is real; the bugs are not optional. A cache you
can't reason about — what's in it, how it gets there, how it
leaves — will eventually serve a stale or wrong value to the
worst possible user.

## Before adding a cache

Answer all four out loud:

1. **What is the cost** of the un-cached path, measured, not
   guessed?
2. **What is the hit rate** you expect, and how do you know?
3. **What is the invalidation rule?** Time-based, event-based,
   never?
4. **What is the worst case** of serving a stale value? If the
   answer is "a user sees the wrong thing," extra care.

If you can't answer #3, you don't have a cache; you have a leak.

## Key design

- The key includes **everything that affects the value**: inputs,
  user / tenant, version of the producing code, feature flags
  that change behavior.
- The key **excludes** things that don't affect the value
  (request id, timestamp, trace id).
- Key collisions across tenants are a security bug, not a perf
  bug. Tenant id is in the key, always.

## Invalidation patterns, in rough order of preference

1. **No invalidation needed** because the value is derived
   purely from the key (content-addressed: hash the inputs, that
   *is* the key). Best when feasible.
2. **TTL** with a budget the product can tolerate. Document the
   freshness contract: "menu items may be up to 60s stale."
3. **Event-driven** invalidation — writes to the source emit a
   message that invalidates affected keys. Powerful, fragile;
   needs an audit story for missed events.
4. **Read-through with revalidation** — serve cached, async
   refresh, swap on completion. Hides latency, keeps content
   reasonably fresh.
5. **Manual flush** for ops emergencies. Always available, never
   the primary mechanism.

## Stampede protection

When a hot key expires, naive caches let every concurrent reader
miss simultaneously, hammer the backend, and bring it down:

- **Single-flight** — coalesce concurrent misses for the same
  key into one upstream fetch; others wait for the result.
- **Early refresh** — refresh slightly before expiry while
  serving stale, so the expiry moment isn't a cliff.
- **Jittered TTLs** — adjacent keys don't all expire at once.

## Negative caching

Cache "this didn't exist" with a short TTL to avoid hammering the
source for misses. Keep the TTL short — a real value appearing
shouldn't take long to surface.

## Observability

A cache without metrics is unmaintainable. At minimum:

- Hit rate (per key class).
- Miss latency (the un-cached path you're trying to hide).
- Eviction rate (memory pressure?).
- Item count and total bytes.
- Stampede events.

## Prompt / LLM caching specifics

- **Prefix-stable ordering** of pinned content, tool catalog, and
  retrieved chunks lets the provider cache the prefix. Re-shuffling
  a map's iteration order on every call destroys the hit rate.
- **Cache key includes the model and version.** A model upgrade
  silently changing outputs is worse than a cache miss.
- **Don't cache personalized output across users.** It's a data
  leak and a policy violation in roughly that order.

## Anti-patterns

- A cache with no TTL, no invalidation, and no eviction. Now it's
  a memory leak with a friendly name.
- Caching at every layer because each layer's owner thought it
  would help. Compound staleness, no clear source of truth.
- A cache whose key omits something the value depends on. Wrong
  answers, sometimes.
- Logging cache hits and misses inline at INFO. Filling logs.
- Adding a cache to "fix" a slow query instead of fixing the
  query, when the query was easily fixable. Now you carry both.

---
name: performance-optimization
description: Profile, measure, and optimize systematically — never guess where the bottleneck is. Use when something is "slow", before any optimization PR, and when reviewing claims of speedup.
category: workflow
priority: 18
---

# Performance optimization

The single most common performance bug is optimizing the wrong
thing. The fix that's "obviously faster" turns out to save 0.2% on
wall time because the bottleneck was elsewhere. Always measure
first.

## The rule

> No optimization commit without a before-and-after measurement.

If you can't show the number going down, you didn't optimize — you
restructured.

## Measure before optimizing

- **Profile, don't guess.** Language profilers exist for a reason:
  Go pprof, Node `--inspect`, Python cProfile/`py-spy`,
  Java async-profiler, browser DevTools. Use them.
- **Reproducible benchmark.** Lock the input, lock the
  environment, run more than once, report median *and* p95. One-shot
  numbers lie.
- **Wall clock vs CPU vs memory vs I/O.** A "slow" program with low
  CPU is I/O-bound; nothing you do to the algorithm helps. Decide
  which dimension matters before changing code.

## Bottleneck categories

- **CPU-bound:** tight loops, expensive math, inefficient data
  structures. Fix with better algorithm or amortization.
- **I/O-bound:** database queries, HTTP calls, file system. Fix
  with batching, caching, async, or connection pooling.
- **Memory-bound:** large allocations, GC pressure, leaks. Fix with
  pooling, fewer allocations, or smaller working sets.
- **Concurrency:** lock contention, thread starvation, exhausted
  pools. Fix with lock-free reads, sharding, or larger pools — *not*
  by adding goroutines indiscriminately.

## Order of attack

1. The highest sample in the profile that you can plausibly change.
2. Re-profile. The new top sample is almost never what you predicted.
3. Stop when the change-to-improvement ratio drops below your
   threshold — usually well before "as fast as theoretically
   possible."

## Common high-leverage fixes

- **Batch the chatty thing.** N+1 queries → one IN-query.
- **Cache the expensive thing.** With a clear invalidation story; a
  cache without invalidation is a bug factory.
- **Move work off the hot path.** Background jobs, lazy
  computation, precomputed tables.
- **Allocate less.** Pre-size slices/maps, reuse buffers, avoid
  string concat in loops.

## Anti-patterns

- "Cleaning up" code and claiming a perf win because the code is
  shorter. Code length and runtime are uncorrelated.
- Microbenchmarks that don't reflect production access patterns.
- Adding goroutines/threads to fix a serial bottleneck. Now you
  have a *serial* bottleneck with contention overhead.
- Caching everything because it sometimes helps. Caches add
  invalidation cost, stale-read bugs, and memory pressure.
- Optimizing before profiling because you "know what's slow."

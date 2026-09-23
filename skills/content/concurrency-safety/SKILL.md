---
name: concurrency-safety
description: Write concurrent code that doesn't race — pass context for cancellation, give every goroutine/task an owner, avoid shared mutable state, prefer channels and immutability over locks. Use when adding goroutines, spawning tasks, or debugging "it works sometimes" bugs.
category: workflow
priority: 16
---

# Concurrency safety

Concurrent bugs are the most expensive bugs in the bug taxonomy:
intermittent, environment-dependent, often invisible in tests, and
catastrophic in production. Get the model right, not just the
syntax.

## Three rules

1. **Every concurrent task is cancellable.** It accepts a context
   (or equivalent cancellation signal) and exits promptly when
   cancelled.
2. **Every concurrent task has an owner.** Someone is responsible
   for starting it, waiting for it, and noticing if it died.
   Unowned goroutines are leaks waiting to happen.
3. **Shared mutable state is the exception, not the default.** If
   two tasks need to communicate, prefer passing values over
   sharing memory.

## Cancellation

- Pass `context.Context` (Go) / `CancellationToken` / equivalent
  through the call chain. It is never optional.
- Every blocking operation — I/O, channel send/recv, sleep — must
  observe the cancel signal.
- Cancellation propagates *down*: when a parent cancels, children
  cancel. Never the other way.
- Timeouts are cancellations with a deadline; same rules.

A "loop that doesn't check the cancel" is a leak. It will outlive
the request, the user session, and possibly the process restart
you scheduled to fix it.

## Ownership

For every spawn, answer:

- Who started this?
- Who joins / awaits it?
- If it panics, who notices?
- If the parent shuts down, who cancels it?

If you can't answer all four, you have a leak waiting to be
observed in production at 3am.

Patterns that make ownership explicit:

- A struct method that spawns goroutines also exposes `Wait()` or
  joins them in `Close()`.
- `errgroup` / structured concurrency primitives that join
  automatically.
- A supervisor that owns a fixed pool and restarts on death.

## Shared state — only as a last resort

Order of preference:

1. **Don't share.** Pass values, return values, copy where cheap.
2. **Immutability.** Build once, share read-only.
3. **Channels / queues.** One owner writes; consumers read.
4. **Single-writer with a mutex.** Mutex guards a small, well-named
   region. Document the invariant the mutex protects.
5. **Read-write lock.** Only when you measured that #4 is a
   bottleneck, not because it sounds faster.
6. **Atomics.** For counters and flags. Easy to get wrong for
   anything more complex.

Each step down adds a class of bug.

## Single-writer for external resources

Things that don't tolerate concurrent writers:

- WebSocket connections (writes from two goroutines corrupt the
  frame).
- File handles being appended to.
- Database transactions.
- Most stdio.

Funnel writes through one owning goroutine with an inbound
channel. Readers can be many; writers are one.

## Detection

- Run with the race detector in CI (`go test -race`, equivalent
  in your toolchain). It catches bugs your tests never trigger.
- Stress tests: run the contested path under N parallel callers
  for M seconds.
- Long-running soak tests catch leaks the race detector won't.

## Anti-patterns

- Spawning a goroutine in a request handler that outlives the
  request, with no cancel.
- "Adding a mutex" to a struct without naming what invariant it
  protects.
- Locking around a slow I/O call. Now all your concurrency is
  serial *and* contended.
- `sync.Once` to paper over an initialization race instead of
  fixing the ordering.
- Logging "shouldn't happen" inside a goroutine and continuing.
  It did happen, you just made it invisible.

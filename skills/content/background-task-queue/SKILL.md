---
name: background-task-queue
description: Run long agentic tasks in detached goroutines with bounded contexts, persisted status, and completion notifications — so HTTP requests never block on multi-minute work. Use when adding async execution to a synchronous endpoint.
category: agent
priority: 20
---

# Background task queue

Some agent tasks (plans with many tool calls, scheduled rules, model
fine-tunes) take minutes. Holding an HTTP request open for the whole
duration is wrong: connections drop, proxies time out, and the
client can't ever retry safely. Move the work to a detached
goroutine.

## Contract

The async endpoint returns **202 Accepted** with:

```json
{"job_id": "...", "status": "queued", "summary": "what was kicked off"}
```

The client polls a status endpoint (or watches a websocket / desktop
notification) for completion. The job's lifecycle is persisted, so
restarts don't lose state.

## Required ingredients

1. **Detached context.** The request context dies when the client
   disconnects. The background work needs its own:

   ```go
   ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
   defer cancel()
   ```

   The timeout caps worst-case resource use. Pick an explicit number;
   never run unbounded.

2. **Single-transition state machine.** Backed by storage:

   ```text
   pending -> approved -> running -> (completed | failed)
                                  \-> cancelled
   ```

   The pending → running step must be atomic on the store, so a
   double POST can't double-run the job. (A `findAndModify` or
   conditional UPDATE works.)

3. **Persisted before responding.** Persist `status: queued` *inside*
   the request handler, before returning 202. The goroutine then
   transitions to `running` itself.

4. **Guardrails carried over.** Permissions, audit, rate-limiting —
   anything the synchronous version did — must be re-applied inside
   the goroutine. The auth identity from the request must be captured
   (string, not `*gin.Context`) and passed in.

5. **Notification on terminal state.** Desktop notification, webhook,
   or websocket push. Always log the outcome, including the failure
   reason for `failed`.

6. **Panic recovery.** Wrap the goroutine body in `recover()` and
   log; one bad job must not crash the process.

## Status visibility

Expose at least these queries:

- `GET /jobs/:id` — full state.
- `GET /jobs?status=running` — what's in flight.
- (Optional) `POST /jobs/:id/cancel` — flips the persisted state and
  the goroutine's context (via a tracked `cancel()`).

## Anti-patterns

- Reusing the request context — silent cancellation on client
  disconnect.
- Holding the cancel func in a process-global map keyed by job id
  with no cleanup on completion — leaks one entry per job.
- Letting jobs run without a hard timeout. "It usually finishes in a
  minute" is not a timeout.
- Trusting in-memory job state — survives crashes only if you
  persist before responding.

---
name: streaming-output
description: Stream model output to the user safely — token-by-token rendering, mid-stream cancellation, partial parsing for tool calls, final-fallback so a dropped connection doesn't lose the reply. Use when adding streaming to an agent UI or fixing "the answer was cut off / shown twice / never finalized" bugs.
category: agent
priority: 22
---

# Streaming output

Streaming makes latency feel half what it is and is mandatory for
anything conversational. It also introduces a fresh class of bugs:
half-rendered output, double-rendered output, cancelled streams
that keep billing, tool calls fired from incomplete JSON. The
discipline below avoids them.

## Two streams, one truth

A stream has two consumers:

- **The UI**, which wants tokens as they arrive.
- **The persistence layer**, which wants one final, complete,
  validated message.

Build the pipeline so the final message is *also* derived from
the same byte sequence the UI saw — not a second non-streaming
request "to get the real answer." Otherwise the two diverge and
the user notices.

## Cancellation must work

Every stream is cancellable:

- The UI's stop button calls a cancel that propagates to the
  provider call (close the connection / abort the request).
- The provider call respects the cancel and exits its read loop.
- Cost accounting still records partial usage; many providers
  bill on tokens generated, not on completion.

A "stop" button that keeps tokens flowing in the background is
worse than no stop button.

## Partial parsing for tool calls

When a tool call is being streamed as JSON:

- **Do not act** on the partial JSON. Wait for the closing brace
  and a successful parse.
- It's fine to *show* a "calling tool X..." indicator from the
  first key, but the call itself fires once, after validation.
- If the parse fails at the end, treat it as a normal validation
  failure with the recovery loop (`structured-output-validation`).

## Buffering and chunking

- The wire chunks aren't the rendering chunks. Buffer wire bytes
  and emit on **safe** boundaries (newline, sentence, every N ms)
  to avoid jittery half-words.
- For code blocks and Markdown, accumulate until the block can be
  rendered consistently; partial code blocks with no closing
  fence look broken.
- Cap buffered size so a slow consumer doesn't grow memory
  unbounded.

## Final-message fallback

Connections drop. The model's last token is often the one the
user needed most. Defenses:

- The harness writes the in-progress message to durable storage
  on every flush (or every N tokens). On reconnect, the client
  fetches the persisted state.
- If the provider stream errors after generating useful tokens,
  do not discard them — finalize with what you have and a
  truncation marker.
- A "fallback" non-streaming request is acceptable as the last
  resort, never the default; it doubles cost.

## Backpressure

If the UI consumer is slow, push back to the producer; don't grow
an unbounded queue. WebSocket and SSE both need an explicit
high-water mark and a drop / pause policy.

## Token deltas vs full snapshots

Two valid wire designs:

- **Deltas** (most common): "+= these tokens". Cheap, fragile to
  packet loss / reordering. Needs an explicit sequence id.
- **Snapshots**: "the whole message so far". Idempotent on
  resend, larger payloads. Often better for non-text streams
  (UI block updates, tool progress).

Pick one *per channel* and document it. Mixing is where bugs live.

## Anti-patterns

- Streaming token deltas as channel messages with no sequence
  number, then resending on reconnect. The client glues them
  twice.
- Acting on a tool call before the JSON is fully parsed.
- Cancelling the UI render but letting the provider call run to
  completion. Cost still climbs.
- A non-streaming "final" request fired alongside the stream.
  Doubled cost; sometimes a different answer.
- Buffering the full reply server-side and then "streaming" it
  out instantly. The latency win comes from the model, not the
  wire.

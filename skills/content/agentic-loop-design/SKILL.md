---
name: agentic-loop-design
description: Design an agentic tool-calling loop with explicit iteration caps, tool-call caps, and a separate synthesis turn. Use when implementing a new agent loop, debugging a runaway loop, or deciding when to terminate.
category: agent
priority: 10
---

# Agentic loop design

An agentic loop drives an LLM through repeated tool calls until it
produces a final answer. Without explicit caps, loops can burn tokens
indefinitely or oscillate between two tools. This skill defines the
minimum structure every loop needs.

## Required caps

Every loop **must** enforce all three of these before sending a message
to the model:

- `MaxIterations` — total tool-call rounds (e.g. 8). The loop returns
  the model's last assistant message and stops once exceeded.
- `MaxToolCalls` — total individual tool invocations across all
  rounds (e.g. 12). Parallel tool calls in one round each count once.
- `MinToolCallsBeforeFinish` — block the model from terminating with
  zero tool calls when the request clearly required tool use (e.g. 1).

The caps are enforced by the loop, never by the model. The model can
"want" to keep going; the loop says no.

## Tool turn vs synthesis turn

Use two models in a single loop:

- **Tool model** (balanced or fast tier) — generates tool calls. Most
  rounds.
- **Synthesis model** (strong tier) — runs the final round where the
  model summarizes results. One round, one call, optionally a longer
  context window.

Pick the tier per turn based on `LoopOptions.ToolCallModel` vs
`LoopOptions.SynthesisModel`. Don't waste a strong model on routine
tool calls and don't ship a weak model the synthesis turn.

## Stop conditions

Terminate as soon as **any** of these are true:

1. Model returned a text-only assistant message (no tool calls).
2. `MaxIterations` reached.
3. `MaxToolCalls` reached.
4. Context cancelled (user disconnected, shutdown, timeout).
5. A tool returned an unrecoverable error and the policy is
   stop-on-error.

## Tool result handling

Every tool result must be:

- Captured into the message history with the matching `tool_call_id`.
- Wrapped via the `prompt-injection-defense` skill before being shown
  to the model.
- Truncated to a sane size before being sent (per-tool cap, e.g.
  16 KiB) so a single noisy tool can't blow the context budget.

## Anti-patterns

- Letting the model decide when to stop without iteration caps.
- Reusing the same context for tool turns and synthesis (the synthesis
  prompt should explicitly ask for the final answer).
- Re-injecting full prior tool results on every turn instead of trimming.
- Calling the LLM from inside a tool (causes recursive blowups).

## Reference: minimum loop

```text
for i := 0; i < MaxIterations; i++ {
    resp := llm.Call(toolModel, history, tools)
    history.append(resp.assistant)
    if len(resp.toolCalls) == 0 {
        return resp.text
    }
    if totalToolCalls >= MaxToolCalls { break }
    for _, call := range resp.toolCalls {
        out := dispatch(call)
        history.append(toolMessage(call.id, guardrail.Wrap(out)))
        totalToolCalls++
    }
}
// Final synthesis turn
return llm.Call(synthesisModel, history, nil).text
```

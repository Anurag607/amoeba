---
name: context-window-management
description: Fit useful prompts into bounded context by budgeting per section, truncating oldest-first with summaries, and pruning redundant tool output. Use when building or debugging an agent loop that hits context limits, slows down at long sessions, or loses early instructions.
category: agent
priority: 12
---

# Context window management

Every model has a hard context ceiling, and *useful* context — the
part where attention still works well — is smaller than the
documented limit. Treat context as a finite budget you allocate
deliberately, not a bag you keep tossing things into.

## Budget by section

Pre-allocate, in tokens, roughly:

- **System / instructions** — fixed, small, near the top.
- **Pinned facts** — user preferences, project conventions, active
  goals. Small and stable.
- **Tool catalog** — only tools relevant to the current step.
  Trim aggressively; unused tool definitions are pure overhead.
- **Working memory** — the running plan, recent decisions.
- **Conversation tail** — the most recent N turns verbatim.
- **Conversation head** — summarized into 1–3 paragraphs.
- **Retrieved chunks** — bounded count, ranked, deduplicated.
- **Reserved for output** — enough room for the model's reply.

If the total exceeds budget, the section that gets squeezed should
be explicit, not whichever happens to be longest.

## Truncate oldest, but keep a summary

Sliding-window-only loses the "why we're doing this." Better:

1. Keep the last N turns verbatim.
2. Summarize everything older into a rolling digest stored as a
   single message.
3. When the digest gets too long, summarize the summary.

The digest should preserve *decisions*, *user intent*, and
*persistent state* — not the chit-chat that led there.

## Prune tool output

Tool calls are often the biggest context hog:

- Replace huge tool outputs with a short structured summary
  (counts, top-K, errors) once the model is done with them.
- Never re-paste the same retrieval result on every turn — store a
  reference and only include the body when actively in use.
- Set a hard cap on per-tool output size (truncate with an ellipsis
  marker so the model knows truncation happened).

## Pinned vs ephemeral

- **Pinned** content stays at fixed positions and never gets
  evicted. Keep it tiny.
- **Ephemeral** content (tool output, retrieval, scratch
  reasoning) lives in the working window and is eligible for
  truncation or summarization.

Confusing the two — pinning everything — is the most common cause
of premature context exhaustion.

## Observability

Log per turn:

- Tokens in / out / reserved.
- Tokens per section.
- Did we summarize or truncate? Which section?

Without these numbers, context bugs look like model regressions.

## Anti-patterns

- Including the full tool catalog on every turn regardless of what
  step you're in.
- Keeping the entire transcript verbatim "in case the model needs
  it." It doesn't; it needs the *summary*.
- Letting the model decide what to forget by writing it into the
  prompt. Truncation is the harness's job, not the model's.
- Counting characters instead of tokens. Off by 3–4× and it's
  always the budget you actually cared about.

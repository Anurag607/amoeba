---
name: tool-design-for-agents
description: Design tools that agents actually use correctly — narrow schemas, validated inputs, idempotent or clearly-non-idempotent semantics, structured error shapes. Use when adding a new tool to an agent or auditing existing tools for misuse.
category: agent
priority: 13
---

# Tool design for agents

A poorly-designed tool is a footgun for the model: it'll find
creative wrong ways to call it. A well-designed tool is one the
model can only use one way — the right way.

## Schema design

- **Narrow types, closed enums.** Prefer `status: "pending" |
  "approved" | "rejected"` over `status: string`. The model picks
  from your closed set instead of inventing values.
- **Required vs optional is a contract.** Mark a field required
  only if you actually validate it and refuse the call without it.
  Optional fields with silent defaults are a debugging nightmare.
- **One tool, one job.** A tool with five mutually-exclusive
  modes (`action: "read" | "write" | "delete" | ...`) is five
  tools wearing a trench coat. Split them.
- **Avoid free-form blobs.** `data: object` lets the model emit
  anything; you parse anything; nobody is happy. Use a typed
  sub-schema or split the tool.

## Names and descriptions

The tool name and description are part of the prompt the model
sees. Treat them as documentation:

- **Verb-noun names**: `search_docs`, `create_pin`, `cancel_job`.
- **Description says what + when**: "List approved plans. Use
  before approving a new one to check for duplicates."
- **Argument descriptions say units and constraints**: not "the
  count", but "max items to return; 1–100; default 20."

## Validate inputs server-side

The model's schema adherence is good, not perfect. Always validate
on the server before executing:

- Type and required-field checks (the cheap part).
- Semantic checks (file path inside allowed root, URL passes SSRF
  rules, regex argument is `QuoteMeta`'d).
- Authorization checks against the *authenticated* identity, not
  any identity the model passed.

Reject with a structured error the model can recover from (see
below) instead of crashing.

## Idempotency

Be explicit:

- **Idempotent tools** (reads, lookups, search) can be retried
  freely. Document this.
- **Non-idempotent tools** (create, send, charge, delete) should
  accept an `idempotency_key` argument or carry one through.
  Document the side effect prominently in the description so the
  model treats it carefully.

## Error shape

Errors the model can act on:

```
{ "ok": false, "error": "invalid_argument",
  "field": "max_items", "reason": "must be 1-100" }
```

Errors the model cannot act on:

```
{ "error": "something went wrong" }
```

Use a closed set of error codes; the model can learn to retry on
some and surface to the user on others.

## Output shape

- Stable keys, stable types. If `items` is a list, it's always a
  list (empty, never absent).
- Truncate large fields with an explicit marker (`"truncated":
  true`) so the model knows the output was cut.
- For long-running operations, return a handle and a status
  endpoint, not a blocking call.

## Anti-patterns

- A `run_command` tool that takes a free-form shell string. The
  agent *will* find a way to misuse it; auditing is impossible.
- Tools that mutate state silently on read calls.
- Tools whose success path returns a string and whose failure
  path returns an object. The model has to guess which it got.
- Adding tools the model rarely needs to every prompt; bloats the
  catalog and makes correct tool choice harder.
- Wrapping every external API as a tool 1:1. Agents want
  *task*-shaped tools, not REST-shaped ones.

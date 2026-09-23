---
name: error-messages
description: Write error messages that help the recipient act — specific, scoped to their layer, and free of internals. Distinguish messages shown to users, to operators, to clients, and to models. Use when adding error paths, designing an API's error shape, or surfacing failures in a UI.
category: workflow
priority: 19
---

# Error messages

A good error message is a tool for the recipient. The recipient
varies — end user, on-call engineer, automated client, language
model — and so does the right message. Writing all of them the
same is why "Something went wrong" exists.

## Four audiences, four shapes

| Audience | What they need |
|---|---|
| End user | What happened in their terms, what they can do, no internals. |
| Operator (logs) | Full chain: operation, inputs (redacted), wrapped causes. |
| API client | Stable error code, machine-readable fields, human-readable hint. |
| LLM (tool error) | Closed-set code, the field that failed, what would satisfy validation. |

The same failure should produce all four. They're derived from
one root error, not authored four times.

## For users

- **Say what happened in their world.** Not "ECONNREFUSED" — "We
  couldn't reach the payment service. Your card has not been
  charged."
- **Say what they can do**, even if it's only "try again in a
  minute" or "contact support with this code: ABC-123."
- **Don't blame the user** for things the system could have
  validated earlier. "Please enter a valid email" is fine after
  they did something wrong; not fine after a backend hiccup.
- **No stack traces, no internal hostnames, no SQL.**

## For operators (logs)

- Wrap with the operation that failed, preserving the cause:
  `fmt.Errorf("approve plan %s: %w", planID, err)` or your
  language's equivalent.
- Include the structured fields that let someone find this in
  observability — request id, user id (redacted appropriately),
  resource id.
- Log at the boundary that handles the error, once. Don't
  log-and-rethrow at every layer.
- The full chain ends up readable: `approve plan abc123: write
  plan: mongo: connection refused`. That's a triagable error.

## For API clients

A predictable error shape:

```json
{
  "error": "invalid_argument",
  "message": "max_items must be 1-100",
  "field": "max_items",
  "request_id": "req_abc123"
}
```

- **Closed set** of `error` codes — clients switch on them.
- **`message`** is human-readable but stable enough to grep in
  logs.
- **`field`** when it's a validation failure.
- **`request_id`** for support correlation.
- Status code matches the category (400 / 401 / 403 / 404 / 409 /
  429 / 5xx) and is *consistent* — same error, same code, every
  time.

## For LLMs (tool errors)

See `tool-design-for-agents`. The model can recover from:

```json
{ "ok": false, "error": "invalid_argument",
  "field": "path", "reason": "must be under /allowed/root" }
```

It cannot recover from:

```json
{ "error": "request failed" }
```

Closed-set codes, the specific field, the constraint that was
violated.

## What never to include

- Secrets, tokens, full API keys.
- PII beyond what the recipient already knows.
- Internal IPs, hostnames, file paths from the server.
- Stack traces in user-facing surfaces (logs only).
- Free-form text the recipient can't reliably parse.

## Don't lie

Two common lies, both production hazards:

- Returning 200 with `{ "success": false }`. Clients and CDNs
  treat 200 as success.
- Returning a fake success because the real operation is async.
  Be explicit: 202 Accepted with a status handle.

## Anti-patterns

- "Something went wrong." Always wrong.
- Different error shapes per route. Clients give up parsing.
- Leaking the upstream provider's error verbatim. Now your
  client switches on a third-party's error codes.
- Catching everything to a single "internal error" with no
  request id. Triage cost: hours.
- Translating user errors before logging them. The operator now
  sees the localized French message; debugging fun.

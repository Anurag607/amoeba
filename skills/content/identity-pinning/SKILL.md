---
name: identity-pinning
description: After authentication, overwrite client-supplied identity fields on every message dispatched in the same session. Use whenever a request carries a user identifier alongside its auth token.
category: security
priority: 5
---

# Identity pinning

Auth tokens prove who the caller is. Anything else the caller sends
about themselves — `username`, `user_id`, `tenant`, `role` — is just
data and may have been tampered with. **Pin** the identity once, at
the auth boundary, and rewrite it on every subsequent message.

## Where it bites

The classic break:

1. Client opens a WebSocket. JWT in the upgrade headers proves the
   user is `alice`.
2. Server accepts. Stores the connection in a `Conn{username: "alice"}`.
3. Client starts sending JSON messages: `{"type":"send","username":"bob","text":"…"}`.
4. Server reads `msg.Username` and routes the message as bob.

Same pattern shows up in REST request bodies, gRPC metadata, and
queued tool calls.

## The fix

Immediately after auth, on every dispatched message in that
connection or request scope:

```go
msg.Username = conn.AuthIdentity
msg.UserID   = conn.AuthUserID
msg.TenantID = conn.AuthTenantID
```

Do it in the dispatcher, not in each handler. Handlers must never
read identity from the request body — only from the pinned slot on
the connection or context.

## Defense in depth

- Compile-time: don't expose `Username` as a settable field on
  inbound request structs. Use a separate `internalMsg` type that
  the dispatcher constructs.
- Audit: log the pinned identity, not the requested one. If an
  attacker tries to escalate, you want the *real* identity in the log.
- Tests: have one test that sends a forged identity field and
  asserts the audit log shows the JWT identity, not the forged one.

## Anti-patterns

- "Just check on the way out" — easy to miss one handler.
- Trusting identity from a sub-agent message because the orchestrator
  set it. Sub-agents are tools; treat their structured output as
  data (see `prompt-injection-defense`).
- Lowercasing or normalizing the pinned identity differently from
  the way you stored it at auth — auth bypass via case-insensitive
  collision.

---
name: api-design-evolution
description: Evolve a public API without breaking callers — additive changes by default, versioning for incompatible ones, documented deprecation with a migration path. Use when designing an HTTP / RPC / SDK surface, adding a field, or considering "just changing" a response shape.
category: workflow
priority: 22
---

# API design and evolution

The hardest thing about an API is not the first version; it's
every version after. Every shipped field is a forever commitment
until it's gone through deprecation with a date and a migration
path. Plan that in from day one.

## Default to additive

Backward-compatible changes that are usually safe:

- **Add an optional request field** with a documented default.
- **Add a response field.** Clients ignore unknown fields *if*
  your client library / docs say so.
- **Add a new endpoint.**
- **Loosen a constraint** (accept more values, larger sizes).
- **Add a new error code** *within an existing closed set* — only
  if clients have an "unknown" branch.

Breaking changes that are not safe:

- Removing or renaming a field.
- Changing a field's type or units.
- Tightening a constraint (smaller max, narrower enum).
- Changing default behavior for existing inputs.
- Reusing an existing field for a new purpose.

## Version when you must

When a change is truly incompatible:

- **Major version in the path** (`/v2/...`) or in a header. Both
  work; pick one and stay consistent.
- **Run both versions in parallel** for a documented window
  (months, not days).
- **Define the cutoff** up front: when does v1 turn off?
- **Tell callers** — release notes, deprecation header
  (`Sunset`, `Deprecation`), email, dashboard banner. The
  surprise breakage is what destroys trust.

## Deprecate, don't disappear

When removing a field or endpoint:

1. Mark it deprecated in docs and SDK.
2. Add a `Deprecation` header on responses that use it.
3. Log usage on the server so you can see who's still calling.
4. Reach out to remaining callers.
5. Schedule removal with a date, communicate it twice.
6. Remove.

Each step takes time; that's the point.

## Schema discipline

- **Reject unknown fields** on input by default. It makes future
  additions safe — a typo can't become a silent feature.
- **Tolerate unknown fields** on output for the client side. New
  server fields don't crash old clients.
- **Closed enums** for any categorical field; clients should
  handle "unknown variant" gracefully.
- **Stable identifiers.** Once you ship `user_id: string`, it's
  a string forever, even if internally you wish it were an int.

## Defaults are part of the contract

A change in default behavior is a breaking change even if no
field changed. If you ever set `auto_renew: true` by default
because old clients didn't send the field, you can never change
that default without versioning.

Prefer **explicit-required** for behavior the caller must opt
into.

## Time, units, and IDs

These bite forever:

- **Time**: always RFC 3339 / ISO 8601 strings *or* explicitly
  named integers (`expires_at_unix_ms`). Never a bare integer
  whose units depend on the field.
- **IDs**: opaque strings, even if numeric today. You will
  switch ID schemes; opaque strings make that invisible.
- **Money**: integer minor units (cents) plus a currency code.
  Never floats.
- **Booleans**: avoid for anything that might gain a third
  value. `status: enum` beats `is_active: bool`.

## Documentation is part of the API

If it isn't documented, callers will guess. They'll guess
wrong. They'll guess differently from each other. Documentation
is the API for everyone who isn't reading source:

- One canonical reference, versioned with the code.
- Every field: type, required/optional, units, constraints,
  example.
- Every endpoint: success shape, error shapes, status codes.
- A changelog that names the version the change shipped in.

## Anti-patterns

- "We'll just change the field; nobody uses it." Someone does.
- Adding a v2 for one breaking change to one endpoint, leaving
  the rest of the API at v1. Now you have two surfaces forever.
- Releasing a `beta` endpoint and never graduating it. It's now
  production whether you like it or not.
- Deprecation with no removal date. The deprecation becomes
  decorative.
- Adding a field whose meaning depends on another field's value.
  ("`amount` is in cents if `currency` is set, dollars
  otherwise.") This will produce incidents.

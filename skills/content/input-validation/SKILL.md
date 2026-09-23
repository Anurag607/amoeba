---
name: input-validation
description: Validate all untrusted input at the boundary — allow-lists over deny-lists, normalize then validate, bound sizes, fail closed. Use whenever data enters the process from a user, network, file upload, or model output.
category: security
priority: 6
---

# Input validation

Every input that crosses a trust boundary is hostile until proven
otherwise. The proof is validation: type, shape, range, and intent.
Skip it, and you ship whichever bug the attacker finds first.

## Validate at the boundary

The boundary is the *first* point inside your trust zone the data
hits — the HTTP handler, the queue consumer, the file reader, the
model-output parser. Validate once, deeply, there. Downstream code
gets to assume the values are sane.

The opposite pattern — "I'll check it where I use it" — fails the
moment a new caller is added.

## Allow-lists over deny-lists

- **Allow-list:** "these characters / values / paths are
  permitted." Anything else is rejected.
- **Deny-list:** "these are forbidden." Everything else is
  permitted, including the one you forgot.

For anything security-sensitive (paths, URLs, identifiers, shell
arguments), the answer is always allow-list. Deny-lists are for
spam classifiers, not security gates.

## Normalize, *then* validate

Order matters:

1. Decode (URL decode, base64, unzip).
2. Normalize (Unicode NFC, path `filepath.Clean`, case-fold where
   appropriate, strip BOMs).
3. *Then* check against the allow-list.

Otherwise an attacker bypasses the allow-list by sending
`%2e%2e/etc/passwd` and laughing at your `..` check that ran
before decoding.

## Bound every size

Unbounded input is a denial-of-service primitive:

- HTTP bodies: `io.LimitReader` (or your language's equivalent) on
  the read side. **Never trust `Content-Length`** as the cap; the
  client controls it.
- Strings inside a payload: per-field max length.
- Arrays / collections: max element count.
- Recursion / nesting depth: explicit cap on JSON / XML parsers.
- File uploads: enforce max size *during* read, not after.

## Type, shape, range

Walk the order:

1. **Parse** to the strongest type the language gives you. A
   string `"-1"` should become an `int` here and fail if it
   doesn't.
2. **Shape**: required fields present, no unknown fields (when
   the schema is closed), arrays where expected.
3. **Range**: numeric bounds, enum membership, regex on bounded
   string shapes.
4. **Semantic**: cross-field invariants ("`end >= start`"),
   referenced ids exist, the caller is authorized for them.

## Fail closed

A validator that returns `nil, nil` on an unknown branch is a
backdoor:

- Default to rejection.
- Log the rejection with enough context to debug, never with the
  full unsanitized payload.
- Return a generic error to the caller (don't leak which rule
  fired — that's a discovery aid for attackers).

## Special cases worth a sentence each

- **Paths:** allow-list the root, `Clean` the result, then check
  the cleaned path is still under the root via prefix check on
  the *absolute* path.
- **URLs:** scheme allow-list (`http`, `https` only), reject
  userinfo, resolve DNS once and validate the IP against your
  rules; see the `ssrf-safe-http` skill.
- **Regex from users:** wrap in `QuoteMeta` / equivalent; never
  pass through raw.
- **SQL / Mongo / shell:** parameterize. Don't compose query
  strings out of user input under any circumstances.
- **Filenames in uploads:** strip directory components, allow-list
  characters, generate a server-side ID for storage.

## Anti-patterns

- Trusting "internal" callers. Any input that started as user
  data is still user data ten hops in.
- Reading the whole body to memory to check its size.
- "Sanitize" functions that try to fix bad input. Reject; don't
  guess what the caller meant.
- Validators that share regex across "username", "path", and
  "url". Each has different rules; one regex won't satisfy any of
  them correctly.

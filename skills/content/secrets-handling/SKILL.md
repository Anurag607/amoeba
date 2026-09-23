---
name: secrets-handling
description: Keep credentials, API keys, and tokens out of source, logs, error messages, and model context — load at the boundary, redact in transit, rotate on exposure. Use any time code touches an API key, database credential, JWT, or signing secret.
category: security
priority: 4
---

# Secrets handling

A leaked secret is one of the few bugs that can ruin a Sunday for
people who didn't write the bug. The defenses are well-known; the
trick is applying all of them, every time.

## Never in source

- Not in committed files.
- Not in test fixtures (use clearly-fake values like `test-key-DO-NOT-USE`).
- Not in example configs (`API_KEY=changeme` is fine; a real key
  is not).
- Not in commit messages, screenshots, or README copy-pastes.

Even in private repos. Repos go public, get forked, get cloned to
laptops that get stolen.

## Load at the boundary

Secrets enter the process at one well-defined place:

- Environment variables, or
- A secret manager (Vault, AWS Secrets Manager, GCP Secret
  Manager, ...), or
- A mounted file with restrictive permissions.

From there they flow into a typed config struct. The rest of the
code uses the config, not the env var directly. This makes it easy
to *find* every use of a secret with a single grep, and to *swap*
the source without touching call sites.

## Redact in logs

The logger must strip secrets before emitting. A redacting hook
that recognizes:

- Authorization headers (`Bearer ...`, `Basic ...`).
- Known credential keys in URLs (`mongodb://user:pass@host`,
  `https://x:y@host`).
- API key formats your providers use (sk-..., ghp_..., etc.).
- Anything matching a configured allowlist of "this field is a
  secret."

Replace with `<redacted>` plus the *length* if useful — never the
prefix; prefixes leak provider and sometimes tenant.

## Never echo to the user

- Error messages: "auth failed" not "auth failed with key
  sk-abc123".
- Stack traces shown to the user: scrub or wrap.
- Diagnostic dumps: redact before serializing.

## Never put in model context

This is specific to agentic systems and frequently missed:

- Secrets *into* the prompt are a prompt-injection target — a
  hostile retrieved doc can ask the model to repeat them.
- Secrets *in* tool arguments returned from the model are an
  exfiltration path if the tool logs its args.

If the model needs to call an authenticated tool, the tool reads
the secret on the server side from config; the model never sees
or names it.

## Rotation and exposure

- **On suspected exposure:** rotate first, investigate second.
  The cost of an unnecessary rotation is small; the cost of a
  delayed one can be unbounded.
- **Scheduled rotation:** for long-lived secrets, at a documented
  cadence; the rotation tooling should be runnable in <5
  minutes.
- **Granularity:** one secret per integration. Sharing a key
  across services means you rotate everything together.

## Anti-patterns

- Hard-coded "temporary" keys ("I'll fix it later").
- Logging the request body unredacted to debug.
- Telling the model the API key in the system prompt so it can
  "call the API itself."
- Long-lived static keys when short-lived tokens are available
  (OIDC, IAM role assumption, service accounts).
- "It's only a dev key" — dev keys reach prod data more often than
  anyone admits.

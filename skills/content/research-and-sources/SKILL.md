---
name: research-and-sources
description: Run effective technical research — specific queries, source prioritization (official → primary → reputable → community), cross-reference before trusting, check dates. Use when investigating an unfamiliar library, debugging a "no one else has this error" problem, or gathering evidence for a design decision.
category: workflow
priority: 25
---

# Research and source evaluation

Good research isn't about more searches; it's about specific
queries against the right sources, then verifying before you
commit to an answer. Bad research wastes hours on outdated blog
posts and AI summaries of summaries.

## Better queries

- **Include versions and exact error text.** `urllib3 2.0
  ConnectionResetError SSL` beats `python ssl error`.
- **Use `site:` to restrict to authoritative sources** when you
  know one exists: `site:docs.python.org`, `site:github.com
  org/repo`, `site:rfc-editor.org`.
- **Quote unusual phrases** so the search engine can't "helpfully"
  reinterpret them.
- **Search the changelog, not the README.** Behavior questions are
  almost always answered in CHANGELOG / release notes / git log.

## Source hierarchy

When sources disagree — and they will — trust in this order:

1. **Official docs of the exact version** you're using. Not
   "latest", not "stable" — the version your code imports.
2. **Source code and its tests.** Reading the test that exercises
   the API beats reading prose about the API.
3. **Primary specifications.** RFCs, language specs, W3C standards,
   formal protocol docs.
4. **Reputable engineering blogs** from the project maintainers,
   the language's core team, or named experts.
5. **Curated community knowledge** — top-voted Stack Overflow
   answers, *if* current and with high-quality comments.
6. **Random blog posts, forum threads, AI-generated tutorials.**
   Treat as hypotheses to verify, never as truth.

## Evaluate before trusting

A source passes if it answers all of:

- **Is it current?** Date of writing, last edit, target version.
  Two-year-old advice about a young library is often wrong.
- **Is it authoritative?** Author, organization, link to source.
  Anonymous + unsourced + confident = treat as fiction.
- **Does it cite primary sources?** A blog that links to the docs
  and the source is checkable; one that doesn't is folklore.
- **Does it match a second independent source?** One source is a
  hypothesis. Two independent ones (not one quoting the other) is
  evidence.

## Cross-reference checklist

For anything you're about to act on:

1. Confirm in the official docs of your version.
2. Look at the actual source/test if it's a behavior question.
3. If still ambiguous, write a 10-line repro and observe.
4. *Then* trust it.

## On AI summaries

- Useful for *orientation* — "what is this library for?"
- Dangerous for *specifics* — "what does flag X do in version Y?"
  Language models confabulate plausible-sounding API surfaces.
- Always verify quoted API names, flag names, and version numbers
  against the actual source.

## Anti-patterns

- Skimming the first Stack Overflow answer without checking the
  date or the version it targets.
- Trusting a tutorial because it ranks #1 on Google. Ranking is
  about SEO, not correctness.
- "It works on my machine" research — one example, no
  cross-reference, ship it.
- Quoting an AI answer as if it were a citation. Quote what *it*
  was citing.
- Reading ten sources and using none of them because none agreed.
  Pick the most authoritative, note the disagreement, move.

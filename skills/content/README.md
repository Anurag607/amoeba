# Curated agentic-moe skills

A library of generic, framework-level skills distilled from production
agentic codebases (Anthropic Agent Skills, OpenClaw, Warp, Omni,
opencode, claude-code). Each skill is a self-contained directory with
a `SKILL.md` that starts with the Anthropic-standard frontmatter:

```markdown
---
name: skill-id
description: One sentence on what the skill is and when to use it.
category: optional grouping (defaults to "general")
priority: optional integer (lower = injected earlier)
---
```

Load them via `skills.LoadFS(reg, fsys, "content")` after embedding
this directory into your binary:

```go
//go:embed all:content
var skillFS embed.FS

reg := skills.NewRegistry()
_ = skills.LoadFS(reg, skillFS, "content")
```

Skills shipped here are intentionally generic. Repo-specific
playbooks belong in the consumer repo, not in the framework.

## Index

| Slug | What it teaches |
|---|---|
| `secrets-handling` | Load at boundary, redact in logs, never echo to user or model context. |
| `prompt-injection-defense` | Wrap untrusted tool output in sentinels, defang known injection markers. |
| `ssrf-safe-http` | http(s)-only allowlist, reject loopback / private / link-local IPs at dial time. |
| `identity-pinning` | Never trust client-supplied identity past the auth boundary. |
| `mongo-injection-safety` | Always `regexp.QuoteMeta` user input in `$regex`; prefer typed filters. |
| `input-validation` | Allow-lists, normalize-then-validate, bound every size, fail closed. |
| `rate-limiting-and-quotas` | Per-identity token-bucket limits, per-route caps, daily quotas, clear 429s. |
| `structured-output-validation` | Schema-validate model output before acting; bounded recovery loop. |
| `agentic-loop-design` | Iteration caps, tool-call caps, tool-turn vs synthesis model split, stop conditions. |
| `model-routing` | Fast tier for routing/classification, strong tier for reasoning; fallback chain + cost ceiling. |
| `context-window-management` | Budget by section, summarize the head, prune tool output, pin sparingly. |
| `retrieval-augmented-generation` | Structural chunking, two-stage retrieval, dedupe, citations, bounded injection. |
| `tool-design-for-agents` | Narrow schemas, closed enums, validated inputs, idempotency, structured errors. |
| `background-task-queue` | Detached context, status transitions, completion notification. |
| `streaming-output` | Token streaming, mid-stream cancellation, partial-JSON safety, final-fallback. |
| `evaluation-and-evals` | Frozen golden set, scoring per category, per-PR + nightly + pre-release gates. |
| `concurrency-safety` | Cancellable contexts, owned tasks, prefer channels/immutability over locks. |
| `retries-and-backoff` | Retry only retryable errors; exponential backoff with jitter; idempotency keys. |
| `caching-strategy` | Name the key, invalidation, and staleness budget; stampede protection; cache observability. |
| `error-messages` | Different shapes per audience (user / operator / API client / LLM); closed-set codes. |
| `api-design-evolution` | Additive by default, version when breaking, deprecate with a date and migration path. |
| `logging-and-observability` | Structured fields, redaction at source, trace ids across boundaries, RED metrics. |
| `code-review` | Priority order (correctness → security → design → readability → tests); blocker vs nit. |
| `debugging-workflow` | Read error, reproduce, narrow, root cause, smallest fix, regression test. |
| `testing-discipline` | Test critical paths and failure paths; deterministic/isolated/readable/fast. |
| `performance-optimization` | Profile before optimizing; categorize CPU vs I/O vs memory vs concurrency. |
| `requirements-gathering` | Clarifying questions for users/success/scope/constraints/edges; surface assumptions. |
| `product-spec` | PRD structure: summary, users, scenarios, behavior, acceptance criteria, non-goals. |
| `tech-spec` | Design doc: context, constraints, options, decision rationale, rollout, observability, risks. |
| `architecture-decision-records` | Short, dated, immutable ADRs; supersede instead of edit; record the "no"s too. |
| `prior-art-survey` | Survey existing solutions before building; triage candidates; build vs adopt vs adapt. |
| `spike-and-prototype` | Time-boxed throwaway experiments answering a specific question, not implementation. |
| `estimation-and-scoping` | Ranges not points; include unfun parts; name what would invalidate the estimate. |
| `risk-assessment` | Likelihood × impact, owner, mitigation, trigger; tracked, not filed-and-forgotten. |
| `planning-breakdown` | Outcome → MVP slice → phases with verification points; replan when reality disagrees. |
| `implementation-patterns` | Naming, flat control flow, contained side effects, minimal mutable state. |
| `incident-response` | Triage (confirm/scope/severity/comm), mitigate first, then investigate; blameless postmortem. |
| `research-and-sources` | Specific queries, source priority (official → primary → reputable → community), cross-reference. |
| `commit-discipline` | Small, scoped, conventional commits; group related changes. |
| `pr-triage` | List-first, hydrate-few, bounded JSON queries; no unsolicited reviews. |
| `dependency-upstream-verification` | Read upstream docs/source/types before assuming any API. |

## Compatibility

The loader accepts both `SKILL.md` (Anthropic spec) and `skill.md`
(legacy). New skills should use `SKILL.md`.

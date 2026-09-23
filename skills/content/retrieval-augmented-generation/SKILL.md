---
name: retrieval-augmented-generation
description: Build a retrieval pipeline that actually helps the model — chunk with structure, embed and rank, dedupe, cite, cap injected context. Use when adding RAG to an agent or debugging hallucinations from a "we already retrieved that" system.
category: agent
priority: 12
---

# Retrieval-augmented generation

RAG fails for a small set of reproducible reasons: bad chunking,
no re-ranking, too much injected context, no provenance. The fix
is not a bigger model.

## The pipeline

1. **Source curation.** Decide what corpus is in scope. RAG over
   "everything" is worse than RAG over a small, trusted set.
2. **Chunking** that respects structure — sections, functions,
   paragraphs — not fixed byte windows that slice mid-sentence.
3. **Embedding** of chunks with a model whose dimensionality and
   distance metric match the index.
4. **Retrieval** — top-K candidates by similarity (often K=20–50,
   not 3; you'll rerank).
5. **Re-ranking** with a cross-encoder or a stronger model to pick
   the actual top-N (often N=3–8).
6. **Deduplication** so two near-identical chunks don't both eat
   budget.
7. **Injection** into the prompt with clear delimiters and
   citation handles.
8. **Citation** in the answer — the model points back at chunk ids
   the user can verify.

Skipping any step costs measurable quality.

## Chunking that works

- Chunk on **semantic boundaries** (heading, function, paragraph)
  with a max size cap as a safety net, not as the primary rule.
- Keep **structural metadata** (file path, heading path, line
  range) on the chunk so citation is automatic.
- **Overlap** small (10–20%) to avoid cutting cross-references.
- Store **the original** alongside the embedding; never re-derive
  from the embedding.

## Two-stage retrieval

A single similarity search returns plausible-but-wrong neighbors.
The cure is a cheap recall stage feeding a precise rank stage:

- Stage 1: vector top-K (cheap, broad).
- Stage 2: cross-encoder or LLM re-rank of those K.

The cost of stage 2 on 20 candidates is small; the quality jump
is large.

## Deduplicate and diversify

If three of your top-5 chunks are paraphrases of the same
paragraph, you've wasted 60% of injected context:

- Cluster by similarity, keep one per cluster.
- Optionally diversify by source so one document doesn't dominate.

## Injection budget

See `context-window-management`:

- Hard cap on total retrieved tokens.
- Per-chunk size cap with a truncation marker.
- Order matters: most-relevant near the user's question, not at
  the top of a wall of text.

## Citations

Every retrieved chunk gets a stable handle (`[#a3]`, `[doc:42]`).
The prompt instructs: "Cite handles for any fact taken from the
retrieved context." Then either:

- The harness verifies cited handles exist (cheap defense against
  hallucinated citations), or
- The UI renders citations as hover-to-preview links so the user
  can verify.

Uncited claims should be flagged in the UI, not hidden.

## Anti-patterns

- Re-embedding the corpus on every query.
- Using the chat model itself as the embedder. Different models,
  different jobs.
- Retrieving from a stale index without a freshness signal.
- Injecting the full retrieved document because "context is
  cheap." Attention degrades long before the context limit does.
- Letting the model rewrite the user's query before retrieval
  with no observability into what was actually queried.
- Trusting cosine similarity > 0.7 as "definitely relevant."
  Thresholds drift with the embedding model.

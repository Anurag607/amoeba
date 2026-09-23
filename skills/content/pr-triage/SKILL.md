---
name: pr-triage
description: Triage GitHub PRs and issues efficiently — list-first with bounded JSON, hydrate few, no unsolicited reviews. Use when sweeping a backlog, deduping, or deciding which PRs deserve a deeper look.
category: workflow
priority: 30
---

# PR triage

Triaging a backlog is a search problem, not a reading problem. The
mistake is fetching full bodies for every item up front. Cost grows
linearly with the list; you'll waste tokens on noise.

## List-first, hydrate few

Pattern:

1. **Cheap list call.** Project only the columns you need.

   ```bash
   gh pr list --state open --limit 40 \
       --json number,title,author,updatedAt,labels,isDraft,reviewDecision
   ```

   Plus `gh search issues` when you need cross-repo or full-text
   queries.

2. **Score and bucket.** From the projection alone, decide:
   - Likely duplicate → check against the open-set; if matched, close
     with a comment linking the canonical issue.
   - Stale (no activity > N days, no review requested) → label and
     skip.
   - Belongs to a maintainer → leave alone unless explicitly asked.
   - Needs a real look → continue.

3. **Hydrate the survivors.** Only now fetch the body, comments,
   files-changed, check status:

   ```bash
   gh pr view 1234 --json body,files,statusCheckRollup,reviewDecision,closingIssuesReferences
   ```

## Dedupe before commenting

Before opening a new issue or PR, run a bounded search for the same
symptoms:

```bash
gh search issues 'repo:OWNER/REPO is:open <keywords>' \
    --json number,title,state,updatedAt --limit 20
```

Boolean `OR` operators in GitHub search are unreliable; if a combined
query returns empty, split it into individual exact-term searches
across title, body, and comments before concluding there's no match.

## What not to do

- **No unsolicited reviews.** A summary in chat is fine. Posting a
  review on a PR you weren't asked to review is noise.
- **No unprompted close/reopen on someone else's PR.** Comment a
  reason and link if you must close; never silently.
- **No mass label/retitle/rebase** without owner approval. Bulk
  actions across more than ~5 items should be confirmed first.
- **Don't poll CI continuously.** Fetch jobs/logs only after a
  failure or when concrete information is needed. The 30-60s polling
  pattern with `--jq` projections is the right rhythm.

## Anti-patterns

- Fetching every comment on every PR before deciding which to act on.
- Closing duplicates without a comment linking the canonical thread —
  reporter sees a silent close and refiles.
- Using approximate string matching ("looks similar") on titles
  alone to flag duplicates. Read the body first, even if briefly.
- Acting on PRs that the maintainer has clearly claimed (assigned to
  themselves, drafted PR open) without checking first.

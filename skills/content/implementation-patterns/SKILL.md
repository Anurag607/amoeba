---
name: implementation-patterns
description: Default coding patterns for clarity and correctness — single responsibility, early returns, contained side effects, minimal mutable state. Use as a checklist while writing or reviewing code.
category: workflow
priority: 18
---

# Implementation patterns

Code is read far more often than it's written. The patterns below
optimize for the reader six months from now — which is usually you.

## Write code that communicates

- **Name things for what they represent**, not how they're
  implemented. `subscribers` not `userMap`. `pending` not
  `theSliceOfJobsThatAreNotDoneYet`.
- **One responsibility per function.** If you need the word "and"
  to describe what a function does, split it.
- **Code structure mirrors the problem structure.** A pipeline
  problem reads as a pipeline. A state-machine problem reads as a
  state machine. Match the shape.
- **Comments explain *why*, not *what*.** If the code needs a
  comment to say what it does, simplify the code first.

## Manage complexity

- **Inline simple logic; extract for reuse or readability.** A
  helper used once usually adds noise. A 12-line block named with
  a verb often doesn't.
- **Flat control flow with early returns** beats nested if/else.
  Validate, then early-return on failure; the happy path stays on
  the left margin.
- **Contain side effects at the boundaries.** I/O, randomness,
  time, global state — keep them at the edges. Core logic should
  be pure where practical because pure code tests itself.
- **Limit mutable and shared state.** Every shared mutable cell
  is a race condition waiting to happen. Prefer immutable values
  passed in, mutable values owned by one goroutine/thread.

## Error handling

- Decide at the boundary whether to wrap, log, or return. Don't
  do all three at every layer.
- Wrap errors with context (`fmt.Errorf("read config: %w", err)`
  in Go; equivalent in your language) — bare errors lose where
  they came from.
- Never silently swallow an error. If you really do want to
  ignore it, write `_ = thing()` and a comment saying why.
- "Unrecoverable" is a design decision. A panic on bad startup
  config is fine; a panic on bad user input is a bug.

## API surface

- Accept the broadest input type that makes sense; return the
  most specific output type that's useful. (Go: accept
  interfaces, return structs.)
- Functions that need three booleans usually want an options
  struct.
- A new exported symbol is a forever commitment unless versioned.
  Keep it small.

## Anti-patterns

- DRY-ing things that only happen to look alike. Coincidental
  duplication is fine; premature abstraction is not.
- Deep inheritance chains. Composition almost always wins.
- "Helper" packages that grow until they're a dumping ground.
- Boolean flags on a function that change its behavior radically
  — split into two functions.
- Catching every error and logging-then-returning-nil. Now the
  caller doesn't know the operation failed and the logs are noise.

---
name: testing-discipline
description: Write tests that catch real bugs without over-testing trivial code — deterministic, isolated, readable, fast — and cover boundaries, integrations, and the failure path. Use when adding tests for new behavior or auditing a test suite that has grown noisy.
category: workflow
priority: 15
---

# Testing discipline

A test suite has two failure modes: too few tests (real bugs ship)
and too many tests (every refactor breaks fifty tests, so nobody
refactors). Both are real. The middle path is to test *observable
behavior at the boundaries that matter*, and skip the rest.

## Test what matters

- **Critical paths first.** Auth, payments, data persistence,
  anything the business cares about.
- **Boundaries and edge cases.** Off-by-one, empty input, nil,
  negative numbers, max-size payloads. This is where most
  reproducible bugs live.
- **Integration points.** Where your code meets a database, an
  HTTP client, a queue, another service.
- **Skip:** trivial getters, setters, simple constructors,
  generated code, things whose behavior is fully captured by the
  type system.

## Good tests are…

- **Deterministic.** Same inputs, same result, every run. No
  `time.Now()` baked in, no random seeds unfixed, no order-dependent
  iteration over a map.
- **Isolated.** One test's failure does not cascade. No shared
  mutable state across tests; build the fixture inside the test or
  in a per-test setup.
- **Readable.** The test name says what scenario is exercised, the
  body shows the inputs and the expected outcome. If you need a
  comment to explain the test, rename.
- **Fast.** Unit tests in milliseconds, integration tests in
  seconds. Slow tests get skipped or commented out, which is the
  same as not having them.

## Failure-path coverage

Most production incidents are in the failure path. For every
externally-visible operation, at least one test should exercise:

- The dependency returning an error.
- A timeout or cancellation.
- Invalid input rejected at the boundary.
- A partial result (some items succeed, some fail).

## Mocking sparingly

- Prefer real implementations of cheap dependencies (in-memory
  store, fake clock) over mocks.
- Mock at the *narrowest* seam — the HTTP client, not the wrapping
  business logic.
- A test that mocks the function under test is testing the mock,
  not the code.

## What to assert

Behavior, not implementation:

- Assert on outputs, persisted state, emitted events.
- Don't assert that an internal helper was called N times.
- Snapshot tests are fine for stable serialized output; never for
  internal data structures.

## Anti-patterns

- One giant test that exercises five behaviors. When it fails you
  don't know which one broke.
- `sleep(100)` to wait for an async result. Use the test's
  synchronization primitives or a fake clock.
- Tests that pass only when run in the right order.
- Coverage chasing — adding tests for trivial code to push the
  number up. Coverage is a *floor* check, not a goal.
- Modifying production code to make it testable in ways that hurt
  the design (exposing fields, adding "test only" branches).

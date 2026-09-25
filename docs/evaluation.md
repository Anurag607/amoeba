# Capability evaluation

`agentic-moe-eval` is an opt-in live evaluator for the planner as consumed by a
real model host. It is separate from `make verify`: normal tests remain offline
and never invoke or download a model.

List the frozen cases without model access:

```sh
go run ./cmd/agentic-moe-eval --list
```

Run the complete suite against an already-installed local Ollama model:

```sh
go run ./cmd/agentic-moe-eval \
  --model llama3.1:8b \
  --repeats 2 \
  --concurrency 1 \
  --context-window 8192
```

The default comparison runs every case twice through a neutral single-expert
baseline and twice through the expert selected by `runtimekit`. Both modes use
the same model, seed schedule, token ceiling, and case text. This isolates the
effect of routing and admitted expert context from model-size differences.
`--context-window` defaults to `ollama.context_window` and is sent to Ollama as
`num_ctx` for every trial, including the cancellation probe.

Use `--domains coding,research,safety` for a subset, `--mode moe` for the routed
path only, and `--json-report path.json` for an explicit report location. The
default report path is `.eval/<timestamp>-<model>.json`; `.eval/` is ignored by
Git. Existing report files are never replaced.

## What is measured

The frozen suite covers coding, reasoning, supplied-source research, concise
writing, planning, fail-closed tool admission, cross-domain routing, long
context, ambiguity, and prompt-injection/secret handling. Each case uses
objective assertions such as exact answers, required concepts, valid JSON,
forbidden text, or word limits. Reports include:

- answer score, routing score, and pass rate by mode and domain;
- selected expert and tier, latency, and provider token counts;
- response SHA-256 and a bounded redacted preview, never the case prompt;
- provider cancellation behavior and observed evaluator concurrency;
- classified planning, timeout, cancellation, and provider failures;
- separate factual, format, instruction-following, and safety assertion
  failures. Strict assertions are not repaired or relaxed after generation.

Report schema version 2 adds `context_window`, per-trial `failure_classes`, and
aggregate `failures`. Older frozen version-1 reports remain valid historical
artifacts and are not rewritten.

The normal offline suite also runs a separate routing corpus with development
and holdout splits. It covers every default expert, punctuation and Unicode
boundaries, false substrings, ambiguity, routing context, and genuine
cross-domain synthesis. Both splits must remain at 100%; live evaluator cases
remain frozen independently so router tuning cannot hide model regressions.

The command fails its gate when the routed path is below either
`--min-answer-score` (default `0.70`) or `--min-routing-score` (default `0.90`).
The JSON report is written before that non-zero exit so failed runs remain
inspectable.

## Safety and interpretation

Ollama defaults to loopback. Remote endpoints require `--allow-remote` or the
framework's explicit `ollama.allow_remote` setting. Redirects, oversized
responses, unbounded calls, silent report replacement, and prompt/output logs
are rejected. A bounded model-manifest preflight fails before the matrix when a
listed model has a stale or missing local blob. Each call has an independent
timeout and generated-token bound, and extended model thinking is disabled so
the scorer evaluates the answer channel.
Interrupting the process cancels active calls and still writes completed
results when possible.

Scores are regression signals, not proof of universal model quality. They use
deterministic rules rather than a model judge, and should be reviewed alongside
the sanitized failed-case previews. The framework owns planning and admission,
not a concrete host tool loop. Accordingly, the tools-domain case verifies that
the evaluated consumer refuses to claim a destructive action when no tool or
approval was admitted; real tool execution remains a host integration eval.

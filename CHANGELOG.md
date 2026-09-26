# Changelog

All notable changes are documented here. The project follows Semantic
Versioning and keeps this file in the Keep a Changelog format.

## [Unreleased]

### Added

- An opt-in `agentic-moe-eval` binary with a frozen multi-domain suite,
  deterministic scoring, baseline/MoE comparison, bounded local Ollama calls,
  cancellation and concurrency probes, and redacted JSON scorecards.
- A frozen offline routing corpus with separate development and holdout splits,
  router fuzz coverage, and allocation-aware microbenchmarks.
- Provider-neutral JSON Schema output contracts with Ollama constrained
  decoding and strict local validation for machine-readable evaluation cases.

### Changed

- Routing now uses Unicode token and phrase boundaries, a general
  low-confidence fallback, and distinct-domain evidence for synthesis instead
  of substring matches and confidence-only escalation.
- Default experts include broader explicit vocabulary and concise operating
  prompts for exact formatting, evidence handling, safe side effects, rollback,
  and verification.
- Evaluation report schema 3 records constrained-output use. Structured
  contracts are applied equally to baseline and routed trials without output
  repair or format retries; existing deterministic assertions remain strict.

## [0.1.0] - 2026-09-24

### Added

- Host-neutral Go orchestration packages for routing, admission, continuation,
  context, durable execution, workspaces, extensions, skills, and research.
- A production composition facade, strict versioned configuration, and
  read-only Ollama discovery.
- MCP stdio and stateless Streamable HTTP transports with a CLI, generated host
  configuration, and a typed TypeScript client and binary launcher.
- Cross-platform release archives, checksums, SBOM generation, Homebrew
  packaging, npm provenance, and local certification scripts.

[Unreleased]: https://github.com/Anurag607/amoeba/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/Anurag607/amoeba/releases/tag/v0.1.0

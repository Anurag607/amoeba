# Integrations

## MCP

Start a local stdio server:

```sh
agentic-moe mcp stdio --config agentic-moe.yaml
```

Start stateless Streamable HTTP on loopback:

```sh
agentic-moe mcp http --address 127.0.0.1:8080
```

Legacy HTTP+SSE is not supported. Non-loopback binds require a bearer token
resolved through `http.bearer_token_env`. Request bodies, concurrency, request
duration, hosts, origins, and graceful shutdown are bounded. Wildcard binds
also require an explicit `allowed_hosts` list.

The server publishes `plan_task`, `plan_for_expert`, and `validate_config`;
manifest, expert, provider, and health resources; and the `orchestrate_task`
prompt. All tools are read-only and side effects are disabled.

Generate host configuration explicitly:

```sh
agentic-moe init --target generic
agentic-moe init --target codex --output agentic-moe.codex.toml
agentic-moe init --target vscode
agentic-moe init --target ollama --output agentic-moe.yaml
```

Generation never overwrites an existing file unless `--force` is supplied.
The Codex fragment can be merged into `~/.codex/config.toml`; VS Code uses
`.vscode/mcp.json`; the generic target writes portable `.mcp.json`.
Generate into an isolated path first when an existing host configuration is
present. The command refuses existing files and symlinks unless replacement of
a regular file is explicitly requested with `--force`.

## Ollama

Ollama discovery defaults to `http://127.0.0.1:11434`. Remote endpoints are rejected
unless `allow_remote` is explicit. Discovery calls `/api/version` and
`/api/tags`, limits response size and duration, and maps installed model size
to fast, balanced, or strong routing tiers. The integration never pulls or
runs a model. HTTP redirects are rejected so a trusted loopback endpoint cannot
redirect discovery to a different network authority.

The separate, explicitly invoked `agentic-moe-eval` binary can run an installed
model through Ollama's bounded non-streaming chat API. It is not started by the
runtime, MCP server, package installation, tests, or CI. See
[capability evaluation](evaluation.md). Evaluation cases that declare a JSON
Schema send it through Ollama's `format` field in both baseline and routed
modes. The response is also validated locally, without repair or retry.

## Streamable HTTP

The HTTP endpoint is stateless and intended for hosts that cannot launch an MCP
stdio child. Loopback is the safe default. A non-loopback listener requires a
bearer token referenced by environment-variable name, and wildcard listeners
require explicit allowed hosts. Configure exact browser origins only when a
browser-based MCP client is expected. Requests are bounded by size, deadline,
and concurrent admission; overload returns HTTP 429.

The repository's package smoke gate exercises both transports as real child
processes through the published TypeScript surface, in addition to protocol
tests against the official Go MCP client.

## Routing integration guidance

Start with `runtimekit.DefaultConfig` and replace expert vocabulary only when
your host has a narrower taxonomy. Use whole words and short phrases, enumerate
intended variants, and avoid generic stems. Pass code, build, issue, trace, and
topic context as supporting evidence; do not encode the entire prompt again as
topics. Keep one general fallback and one synthesis expert. Before shipping a
custom configuration, add provider-free routing cases modeled on
`evaluation.RoutingCases`, keep a holdout split, run the router benchmark, then
use `agentic-moe-eval` against at least one small and one stronger installed
model. Model answer quality and deterministic routing are separate gates.

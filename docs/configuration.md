# Configuration

agentic-moe accepts strict YAML or JSON. Unknown fields, duplicate expert IDs,
invalid tiers, unsupported schema versions, negative per-expert limits, and
non-positive global limits fail at startup. Expert IDs must begin with a
lowercase ASCII letter and contain at most 64 lowercase letters, digits,
hyphens, or underscores. Precedence is defaults, file, `AGENTIC_MOE_*`
environment values, then explicit CLI flags.

```yaml
version: 1
models:
  fast: qwen2.5:3b
  balanced: qwen2.5:7b
  strong: qwen2.5:14b
ollama:
  enabled: true
  base_url: http://127.0.0.1:11434
http:
  address: 127.0.0.1:8080
  bearer_token_env: AGENTIC_MOE_HTTP_TOKEN
  allowed_hosts: [localhost, 127.0.0.1]
```

Run `agentic-moe schema` for JSON Schema and
`agentic-moe config validate path/to/config.yaml` for strict validation.
Versionless files are migrated in memory to version 1; files are never
rewritten implicitly.

Expert `keywords` are matched as case-folded Unicode tokens or contiguous
phrases. Punctuation is a boundary, so `release-note` matches the phrase
`release note`; substrings do not match, so `explanation` does not imply
`plan`. List intended grammatical variants explicitly instead of relying on
stems. Keep the `general` expert as the low-confidence fallback and reserve
`can_synthesize` for one expert: automatic synthesis escalation requires
at least two positive lexical signals from each of two specialist domains, not
context boosts or two generic words. Expert `prompt` values should be short operating constraints
that reinforce output format, authority, rollback, and evidence handling. Leave
an expert prompt empty when the host's secure baseline already owns those rules;
redundant system instructions can reduce adherence on smaller models.

The supported environment values are:

- `AGENTIC_MOE_HTTP_ADDRESS`
- `AGENTIC_MOE_OLLAMA_URL`
- `AGENTIC_MOE_OLLAMA_ENABLED`

`bearer_token_env` stores an environment variable name, not a token. Model and
provider credentials are intentionally absent from the framework schema.
Wildcard HTTP binds require an explicit allowed-host list. Non-loopback binds
also require `bearer_token_env`. Allowed origins are exact `http` or `https`
origins without credentials, paths, queries, or fragments; subdomains are not
implicitly trusted.

The MCP `validate_config` tool returns `{valid:false}` with a generic error for
invalid input rather than exposing parser details across the transport. Local
CLI validation retains detailed diagnostics for the operator.

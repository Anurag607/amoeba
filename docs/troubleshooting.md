# Troubleshooting

Run `agentic-moe doctor --json` first. It validates configuration and reports
each configured provider independently.

- **stdio client disconnects:** ensure no wrapper writes human logs to stdout.
  Protocol messages exclusively own stdout; diagnostics use stderr.
- **HTTP rejects a bind:** non-loopback addresses require
  `http.bearer_token_env` and a non-empty environment value.
- **HTTP returns 403:** add the exact browser origin to `allowed_origins`, or
  use a non-browser MCP client without an Origin header.
- **Ollama is degraded:** verify the configured URL and run `ollama list`.
  Discovery does not install missing models.
- **npm launcher cannot find a binary:** set `AGENTIC_MOE_BINARY`, install the
  Homebrew/npm release on PATH, or permit a verified release download.
- **config reports an unknown field:** the schema is intentionally strict;
  inspect `agentic-moe schema` and remove misspelled or obsolete keys.

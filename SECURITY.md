# Security policy

## Supported versions

Security fixes are provided for the latest minor release. Before 1.0, minor
releases may include breaking changes described in the release notes.

## Reporting a vulnerability

Use GitHub private vulnerability reporting for this repository. Do not open a
public issue with exploit details, credentials, prompts, or tool output. Include
the affected version, impact, reproduction steps, and any suggested mitigation.

## Trust boundary

agentic-moe plans work but does not own credentials or execute host side
effects. Hosts remain responsible for authenticating users, pinning identity,
authorizing every model and tool invocation, sandboxing processes, and storing
secrets. MCP HTTP binds outside loopback require bearer authentication. Tokens
are referenced by environment-variable name and must never be placed in config.
Wildcard listeners require explicit host authority; browser origins are exact,
and HTTP request size, time, concurrency, and shutdown are bounded. Responses
disable caching and MIME sniffing.

Ollama support performs bounded version and installed-model discovery only. It
does not pull, delete, run, or switch models. It rejects redirects so loopback
discovery cannot be used as a redirect-based SSRF trampoline.

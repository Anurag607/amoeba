# agentic-moe TypeScript SDK

This package is a typed client and launcher for the canonical Go runtime. It
does not reimplement routing or execution policy.

```ts
import { AgenticMOEClient } from "agentic-moe";

const client = await AgenticMOEClient.stdio();
const plan = await client.plan("Review this API design", {
  has_code_context: true,
});
await client.close();
```

Set `AGENTIC_MOE_BINARY` to an explicit trusted binary, install
`agentic-moe` on `PATH`, or allow the launcher to download the matching
checksummed GitHub release into the user cache.

The client validates structured MCP responses at runtime before returning typed
values. The launcher resolves an explicit path, `AGENTIC_MOE_BINARY`, or `PATH`
before downloading the matching GitHub release. Downloads have a deadline and
size cap, require the archive's SHA-256 entry from `checksums.txt`, extract only
the expected binary, and cache only a regular executable file in a
platform-and-architecture-specific directory.

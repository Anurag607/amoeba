import assert from "node:assert/strict";
import test from "node:test";

import { AgenticMOEClient } from "./client.js";

test("HTTP client rejects non-HTTP URLs before connecting", async () => {
  await assert.rejects(AgenticMOEClient.http({ url: "file:///tmp/mcp" }), /http or https/);
});

test("HTTP client rejects credentials embedded in URLs", async () => {
  await assert.rejects(AgenticMOEClient.http({ url: "https://user:secret@example.com/mcp" }), /credentials/);
});

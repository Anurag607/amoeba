import { spawn } from "node:child_process";
import { createServer } from "node:net";

import { AgenticMOEClient } from "../sdk/typescript/dist/index.js";

const binary = process.argv[2];
if (binary === undefined) throw new Error("usage: node scripts/http_smoke.mjs /path/to/agentic-moe");

const port = await reservePort();
const address = `127.0.0.1:${port}`;
const child = spawn(binary, ["mcp", "http", "--address", address], {
  env: process.env,
  stdio: ["ignore", "ignore", "pipe"],
});
let diagnostics = "";
child.stderr.setEncoding("utf8");
child.stderr.on("data", (chunk) => {
  diagnostics = (diagnostics + chunk).slice(-8192);
});

try {
  const client = await connectWithRetry(`http://${address}`);
  try {
    const plan = await client.plan("compare runtime options");
    const manifest = await client.manifest();
    if (plan.expert.id === "" || !manifest.transports.includes("streamable-http")) {
      throw new Error("HTTP MCP returned an incomplete contract");
    }
  } finally {
    await client.close();
  }
} catch (error) {
  throw new Error(`${error instanceof Error ? error.message : String(error)}\n${diagnostics}`.trim());
} finally {
  await stopChild(child);
}

async function reservePort() {
  const server = createServer();
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address();
  await new Promise((resolve) => server.close(resolve));
  if (address === null || typeof address === "string") throw new Error("failed to reserve a TCP port");
  return address.port;
}

async function connectWithRetry(url) {
  let lastError;
  for (let attempt = 0; attempt < 40; attempt += 1) {
    if (child.exitCode !== null) throw new Error(`HTTP MCP exited with ${child.exitCode}`);
    try {
      return await AgenticMOEClient.http({ url });
    } catch (error) {
      lastError = error;
      await new Promise((resolve) => setTimeout(resolve, 50));
    }
  }
  throw lastError ?? new Error("HTTP MCP did not become ready");
}

async function stopChild(process) {
  if (process.exitCode !== null) return;
  process.kill("SIGTERM");
  const exited = new Promise((resolve) => process.once("exit", resolve));
  const timeout = new Promise((resolve) => setTimeout(resolve, 3000, "timeout"));
  if (await Promise.race([exited, timeout]) === "timeout") {
    process.kill("SIGKILL");
    await exited;
  }
}

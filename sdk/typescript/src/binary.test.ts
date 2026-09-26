import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import { resolveBinary, verifyFileSHA256 } from "./binary.js";

test("resolveBinary honors an explicit executable", async () => {
  const directory = await mkdtemp(join(tmpdir(), "agentic-moe-sdk-"));
  const path = process.platform === "win32" ? process.execPath : join(directory, "agentic-moe");
  try {
    if (process.platform !== "win32") {
      await writeFile(path, "#!/bin/sh\nexit 0\n", { mode: 0o755 });
    }
    assert.equal(await resolveBinary({ binaryPath: path, download: false }), path);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("resolveBinary rejects a directory as an executable", async () => {
  const directory = await mkdtemp(join(tmpdir(), "agentic-moe-sdk-"));
  try {
    await assert.rejects(resolveBinary({ binaryPath: directory, download: false }), /not a file/);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("verifyFileSHA256 compares the full digest", async () => {
  const directory = await mkdtemp(join(tmpdir(), "agentic-moe-sdk-"));
  const path = join(directory, "fixture");
  try {
    const body = Buffer.from("agentic-moe");
    await writeFile(path, body);
    const digest = createHash("sha256").update(body).digest("hex");
    assert.equal(await verifyFileSHA256(path, digest), true);
    assert.equal(await verifyFileSHA256(path, "0".repeat(64)), false);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("resolveBinary validates download timeout before network access", async () => {
  const directory = await mkdtemp(join(tmpdir(), "agentic-moe-sdk-"));
  const originalPath = process.env.PATH;
  const originalBinary = process.env.AGENTIC_MOE_BINARY;
  try {
    process.env.PATH = "";
    delete process.env.AGENTIC_MOE_BINARY;
    await assert.rejects(resolveBinary({ version: "0.1.0", cacheDir: directory, downloadTimeoutMs: 999 }), /downloadTimeoutMs/);
  } finally {
    if (originalPath === undefined) delete process.env.PATH; else process.env.PATH = originalPath;
    if (originalBinary === undefined) delete process.env.AGENTIC_MOE_BINARY; else process.env.AGENTIC_MOE_BINARY = originalBinary;
    await rm(directory, { recursive: true, force: true });
  }
});

test("resolveBinary refuses a release with a mismatched checksum", async () => {
  const directory = await mkdtemp(join(tmpdir(), "agentic-moe-sdk-"));
  const originalPath = process.env.PATH;
  const originalBinary = process.env.AGENTIC_MOE_BINARY;
  const originalFetch = globalThis.fetch;
  const requestedURLs: string[] = [];
  try {
    process.env.PATH = "";
    delete process.env.AGENTIC_MOE_BINARY;
    globalThis.fetch = async (input) => {
      const url = String(input);
      requestedURLs.push(url);
      if (url.endsWith("checksums.txt")) {
        const platform = process.platform === "win32" ? "windows" : process.platform;
        const arch = process.arch === "x64" ? "amd64" : process.arch;
        const extension = platform === "windows" ? "zip" : "tar.gz";
        return new Response(`${"0".repeat(64)}  agentic-moe_0.1.0_${platform}_${arch}.${extension}\n`);
      }
      return new Response("not an archive");
    };
    await assert.rejects(resolveBinary({ version: "0.1.0", cacheDir: directory }), /checksum mismatch/);
    const platform = process.platform === "win32" ? "windows" : process.platform;
    const arch = process.arch === "x64" ? "amd64" : process.arch;
    const extension = platform === "windows" ? "zip" : "tar.gz";
    const releaseURL = "https://github.com/Anurag607/amoeba/releases/download/v0.1.0";
    assert.deepEqual(requestedURLs, [
      `${releaseURL}/agentic-moe_0.1.0_${platform}_${arch}.${extension}`,
      `${releaseURL}/checksums.txt`,
    ]);
  } finally {
    globalThis.fetch = originalFetch;
    if (originalPath === undefined) delete process.env.PATH; else process.env.PATH = originalPath;
    if (originalBinary === undefined) delete process.env.AGENTIC_MOE_BINARY; else process.env.AGENTIC_MOE_BINARY = originalBinary;
    await rm(directory, { recursive: true, force: true });
  }
});

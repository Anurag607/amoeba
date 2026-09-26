import { createHash } from "node:crypto";
import { access, chmod, lstat, mkdir, mkdtemp, readFile, realpath, rename, rm, stat, writeFile } from "node:fs/promises";
import { constants } from "node:fs";
import { homedir } from "node:os";
import { delimiter, join } from "node:path";
import { spawn } from "node:child_process";

import { VERSION } from "./version.js";

const repository = "https://github.com/Anurag607/amoeba";
const maxArchiveBytes = 128 * 1024 * 1024;
const maxChecksumBytes = 1024 * 1024;

export interface BinaryOptions {
  binaryPath?: string;
  version?: string;
  cacheDir?: string;
  download?: boolean;
  downloadTimeoutMs?: number;
}

/** Resolve the Go binary by explicit path, environment, PATH, then verified release download. */
export async function resolveBinary(options: BinaryOptions = {}): Promise<string> {
  const explicit = options.binaryPath ?? process.env.AGENTIC_MOE_BINARY;
  if (explicit !== undefined) {
    await ensureExecutable(explicit);
    return explicit;
  }
  const fromPath = await findOnPath(binaryName());
  if (fromPath !== undefined) {
    return fromPath;
  }
  if (options.download === false) {
    throw new Error("agentic-moe binary was not found; set AGENTIC_MOE_BINARY or install it on PATH");
  }
  return downloadBinary(options.version ?? VERSION, options.cacheDir, options.downloadTimeoutMs ?? 30_000);
}

async function downloadBinary(version: string, cacheDir: string | undefined, timeoutMs: number): Promise<string> {
  if (!/^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/.test(version)) {
    throw new Error(`refusing invalid agentic-moe version: ${version}`);
  }
  if (!Number.isSafeInteger(timeoutMs) || timeoutMs < 1_000 || timeoutMs > 300_000) {
    throw new Error(`downloadTimeoutMs must be an integer between 1000 and 300000`);
  }
  const platform = releasePlatform();
  const arch = releaseArch();
  const ext = platform === "windows" ? "zip" : "tar.gz";
  const archive = `agentic-moe_${version}_${platform}_${arch}.${ext}`;
  const root = cacheDir ?? join(homedir(), ".cache", "agentic-moe", version, `${platform}-${arch}`);
  const destination = join(root, binaryName());
  try {
    await ensureRegularExecutable(destination);
    return destination;
  } catch {
    // Continue to an authenticated-by-checksum release download.
  }
  await mkdir(root, { recursive: true });
  const temporary = await mkdtemp(join(root, ".download-"));
  try {
    const releaseURL = `${repository}/releases/download/v${version}`;
    const signal = AbortSignal.timeout(timeoutMs);
    const [archiveResponse, checksumResponse] = await Promise.all([
      fetch(`${releaseURL}/${archive}`, { signal }),
      fetch(`${releaseURL}/checksums.txt`, { signal }),
    ]);
    if (!archiveResponse.ok || !checksumResponse.ok) {
      throw new Error(`release download failed (${archiveResponse.status}, ${checksumResponse.status})`);
    }
    const archiveBody = await readBounded(archiveResponse, maxArchiveBytes, "release archive");
    const checksums = new TextDecoder().decode(await readBounded(checksumResponse, maxChecksumBytes, "checksum file"));
    const expected = checksumFor(checksums, archive);
    const actual = createHash("sha256").update(archiveBody).digest("hex");
    if (actual !== expected) {
      throw new Error("agentic-moe release checksum mismatch");
    }
    const archivePath = join(temporary, archive);
    await writeFile(archivePath, archiveBody, { mode: 0o600 });
    await run("tar", [platform === "windows" ? "-xf" : "-xzf", archivePath, "-C", temporary, binaryName()]);
    const extracted = join(temporary, binaryName());
    if (platform !== "windows") {
      await chmod(extracted, 0o755);
    }
    await ensureRegularExecutable(extracted);
    try {
      await rename(extracted, destination);
    } catch (error) {
      // Another process may have completed the same verified download first.
      try {
        await ensureRegularExecutable(destination);
      } catch {
        throw error;
      }
    }
    return destination;
  } finally {
    await rm(temporary, { recursive: true, force: true });
  }
}

async function readBounded(response: Response, limit: number, name: string): Promise<Uint8Array> {
  if (response.body === null) throw new Error(`${name} returned an empty body`);
  const length = response.headers.get("content-length");
  if (length !== null && Number(length) > limit) throw new Error(`${name} exceeds ${limit} bytes`);
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let size = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    size += value.byteLength;
    if (size > limit) {
      await reader.cancel();
      throw new Error(`${name} exceeds ${limit} bytes`);
    }
    chunks.push(value);
  }
  const body = new Uint8Array(size);
  let offset = 0;
  for (const chunk of chunks) {
    body.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return body;
}

function checksumFor(file: string, archive: string): string {
  for (const line of file.split(/\r?\n/)) {
    const [digest, name] = line.trim().split(/\s+/, 2);
    if (name === archive && digest !== undefined && /^[a-f0-9]{64}$/i.test(digest)) {
      return digest.toLowerCase();
    }
  }
  throw new Error(`release checksum is missing ${archive}`);
}

async function findOnPath(name: string): Promise<string | undefined> {
  for (const directory of (process.env.PATH ?? "").split(delimiter)) {
    if (directory === "") continue;
    const candidate = join(directory, name);
    try {
      await ensureExecutable(candidate);
      if (await resolvesToCurrentScript(candidate)) continue;
      return candidate;
    } catch {
      // Keep searching.
    }
  }
  return undefined;
}

async function resolvesToCurrentScript(candidate: string): Promise<boolean> {
  if (process.argv[1] === undefined) return false;
  try {
    return await realpath(candidate) === await realpath(process.argv[1]);
  } catch {
    return false;
  }
}

async function ensureExecutable(path: string): Promise<void> {
  const info = await stat(path);
  if (!info.isFile()) throw new Error(`${path} is not a file`);
  await access(path, process.platform === "win32" ? constants.F_OK : constants.X_OK);
}

async function ensureRegularExecutable(path: string): Promise<void> {
  const info = await lstat(path);
  if (!info.isFile()) throw new Error(`${path} is not a regular file`);
  await access(path, process.platform === "win32" ? constants.F_OK : constants.X_OK);
}

function binaryName(): string {
  return process.platform === "win32" ? "agentic-moe.exe" : "agentic-moe";
}

function releasePlatform(): string {
  const platforms: Readonly<Record<string, string>> = { darwin: "darwin", linux: "linux", win32: "windows" };
  const value = platforms[process.platform];
  if (value === undefined) throw new Error(`unsupported platform: ${process.platform}`);
  return value;
}

function releaseArch(): string {
  const architectures: Readonly<Record<string, string>> = { x64: "amd64", arm64: "arm64" };
  const value = architectures[process.arch];
  if (value === undefined) throw new Error(`unsupported architecture: ${process.arch}`);
  return value;
}

async function run(command: string, args: string[]): Promise<void> {
  await new Promise<void>((resolve, reject) => {
    const child = spawn(command, args, { stdio: "ignore" });
    child.once("error", reject);
    child.once("exit", (code) => code === 0 ? resolve() : reject(new Error(`${command} exited with ${code ?? "signal"}`)));
  });
}

// Kept exported for offline package tests without network access.
export async function verifyFileSHA256(path: string, expected: string): Promise<boolean> {
  const body = await readFile(path);
  return createHash("sha256").update(body).digest("hex") === expected.toLowerCase();
}

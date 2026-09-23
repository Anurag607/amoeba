#!/usr/bin/env node
import { spawn } from "node:child_process";

import { resolveBinary } from "./binary.js";

const binary = await resolveBinary();
const child = spawn(binary, process.argv.slice(2), { stdio: "inherit" });
child.once("error", (error) => {
  console.error(`agentic-moe: ${error.message}`);
  process.exitCode = 1;
});
child.once("exit", (code, signal) => {
  if (signal !== null) {
    process.kill(process.pid, signal);
    return;
  }
  process.exitCode = code ?? 1;
});

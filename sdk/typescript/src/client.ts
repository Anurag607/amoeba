import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StdioClientTransport } from "@modelcontextprotocol/sdk/client/stdio.js";
import { StreamableHTTPClientTransport } from "@modelcontextprotocol/sdk/client/streamableHttp.js";

import type { Manifest, PlanView, RoutingInput, ValidationResult } from "./types.js";
import { parseManifest, parsePlan, parseValidation } from "./validate.js";
import { VERSION } from "./version.js";

export interface StdioOptions {
  command?: string;
  args?: string[];
  env?: Record<string, string>;
}

export interface HTTPOptions {
  url: string | URL;
  bearerToken?: string;
}

/** AgenticMOEClient delegates all planning to the canonical Go MCP server. */
export class AgenticMOEClient {
  readonly #client: Client;

  private constructor(client: Client) {
    this.#client = client;
  }

  static async stdio(options: StdioOptions = {}): Promise<AgenticMOEClient> {
    const client = new Client({ name: "agentic-moe-typescript", version: VERSION });
    const transport = new StdioClientTransport({
      command: options.command ?? "agentic-moe",
      args: options.args ?? ["mcp", "stdio"],
      ...(options.env === undefined ? {} : { env: options.env }),
      stderr: "inherit",
    });
    await client.connect(transport);
    return new AgenticMOEClient(client);
  }

  static async http(options: HTTPOptions): Promise<AgenticMOEClient> {
    const url = new URL(options.url);
    if (url.protocol !== "http:" && url.protocol !== "https:") throw new TypeError("MCP URL must use http or https");
    if (url.username !== "" || url.password !== "") throw new TypeError("MCP URL must not contain credentials");
    const client = new Client({ name: "agentic-moe-typescript", version: VERSION });
    const headers = options.bearerToken === undefined ? undefined : { Authorization: `Bearer ${options.bearerToken}` };
    const transport = new StreamableHTTPClientTransport(url, {
      ...(headers === undefined ? {} : { requestInit: { headers } }),
    });
    await client.connect(transport);
    return new AgenticMOEClient(client);
  }

  async plan(query: string, routing: RoutingInput = {}): Promise<PlanView> {
    if (query.trim() === "") throw new TypeError("query is required");
    return parsePlan(await this.call("plan_task", { query, routing }));
  }

  async planForExpert(expertId: string): Promise<PlanView> {
    if (expertId.trim() === "") throw new TypeError("expertId is required");
    return parsePlan(await this.call("plan_for_expert", { expert_id: expertId }));
  }

  async validateConfig(config: string): Promise<ValidationResult> {
    if (config.trim() === "") throw new TypeError("config is required");
    return parseValidation(await this.call("validate_config", { config }));
  }

  async manifest(): Promise<Manifest> {
    const result = await this.#client.readResource({ uri: "agentic-moe://manifest" });
    const text = result.contents.find((content) => "text" in content)?.text;
    if (typeof text !== "string") {
      throw new Error("agentic-moe manifest resource did not return text");
    }
    return parseManifest(JSON.parse(text) as unknown);
  }

  async close(): Promise<void> {
    await this.#client.close();
  }

  private async call(name: string, args: Record<string, unknown>): Promise<unknown> {
    const result = await this.#client.callTool({ name, arguments: args });
    if (result.isError) {
      const content = Array.isArray(result.content) ? result.content : [];
      const message = content.map(textFromContent).filter((text) => text !== undefined).join("; ");
      throw new Error(message || `${name} failed`);
    }
    if (result.structuredContent === undefined) {
      throw new Error(`${name} returned no structured content`);
    }
    return result.structuredContent;
  }
}

function textFromContent(item: unknown): string | undefined {
  if (typeof item !== "object" || item === null || !("type" in item) || !("text" in item)) return undefined;
  const value = item as { type: unknown; text: unknown };
  return value.type === "text" && typeof value.text === "string" ? value.text : undefined;
}

import type { Manifest, PlanView, ValidationResult } from "./types.js";

export function parsePlan(value: unknown): PlanView {
  const object = asObject(value, "plan");
  const expert = asObject(object.expert, "plan.expert");
  const selection = asObject(object.selection, "plan.selection");
  const options = asObject(object.options, "plan.options");
  const tier = asObject(object.tier, "plan.tier");
  requireString(expert.id, "plan.expert.id");
  requireString(expert.name, "plan.expert.name");
  requireString(expert.default_tier, "plan.expert.default_tier");
  requireString(selection.primary, "plan.selection.primary");
  requireString(selection.reasoning, "plan.selection.reasoning");
  requireNumberRecord(selection.confidences, "plan.selection.confidences");
  optionalStringArray(selection.secondary, "plan.selection.secondary");
  requireNonNegativeInteger(options.response_token_budget, "plan.options.response_token_budget");
  requireNonNegativeInteger(options.max_iterations, "plan.options.max_iterations");
  requireNonNegativeInteger(options.max_tool_calls, "plan.options.max_tool_calls");
  if (typeof options.enable_chunking !== "boolean") throw new TypeError("plan.options.enable_chunking must be a boolean");
  requireString(tier.tool_call, "plan.tier.tool_call");
  requireString(tier.synthesis, "plan.tier.synthesis");
  requireString(tier.reasoning, "plan.tier.reasoning");
  optionalStringArray(object.tool_names, "plan.tool_names");
  optionalStringArray(object.loaded_skills, "plan.loaded_skills");
  optionalStringArray(object.omitted_skills, "plan.omitted_skills");
  return object as unknown as PlanView;
}

export function parseManifest(value: unknown): Manifest {
  const object = asObject(value, "manifest");
  requireNonNegativeInteger(object.schema_version, "manifest.schema_version");
  if (!Array.isArray(object.experts)) throw new TypeError("manifest.experts must be an array");
  if (!Array.isArray(object.transports) || !object.transports.every((item) => typeof item === "string")) {
    throw new TypeError("manifest.transports must be a string array");
  }
  if (typeof object.side_effects_enabled !== "boolean") {
    throw new TypeError("manifest.side_effects_enabled must be a boolean");
  }
  optionalStringArray(object.providers, "manifest.providers");
  for (const [index, expertValue] of object.experts.entries()) {
    const expert = asObject(expertValue, `manifest.experts[${index}]`);
    requireString(expert.id, `manifest.experts[${index}].id`);
    requireString(expert.name, `manifest.experts[${index}].name`);
  }
  return object as unknown as Manifest;
}

export function parseValidation(value: unknown): ValidationResult {
  const object = asObject(value, "validation result");
  if (typeof object.valid !== "boolean") throw new TypeError("validation result.valid must be a boolean");
  requireNonNegativeInteger(object.version, "validation result.version");
  if (object.error !== undefined) requireString(object.error, "validation result.error");
  return object as unknown as ValidationResult;
}

function asObject(value: unknown, name: string): Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new TypeError(`${name} must be an object`);
  }
  return value as Record<string, unknown>;
}

function requireString(value: unknown, name: string): asserts value is string {
  if (typeof value !== "string" || value.length === 0) throw new TypeError(`${name} must be a non-empty string`);
}

function requireNonNegativeInteger(value: unknown, name: string): asserts value is number {
  if (!Number.isSafeInteger(value) || (value as number) < 0) throw new TypeError(`${name} must be a non-negative integer`);
}

function optionalStringArray(value: unknown, name: string): void {
  if (value !== undefined && (!Array.isArray(value) || !value.every((item) => typeof item === "string"))) {
    throw new TypeError(`${name} must be a string array`);
  }
}

function requireNumberRecord(value: unknown, name: string): void {
  const object = asObject(value, name);
  if (!Object.values(object).every((item) => typeof item === "number" && Number.isFinite(item))) {
    throw new TypeError(`${name} must contain only finite numbers`);
  }
}

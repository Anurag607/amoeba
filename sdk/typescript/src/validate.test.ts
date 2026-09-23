import assert from "node:assert/strict";
import test from "node:test";

import { parseManifest, parsePlan, parseValidation } from "./validate.js";

const validPlan = {
  expert: { id: "coding", name: "Coding", default_tier: "balanced" },
  selection: { primary: "coding", confidences: { coding: 1 }, reasoning: "matched" },
  options: { response_token_budget: 1024, enable_chunking: false, max_iterations: 4, max_tool_calls: 6 },
  tier: { tool_call: "balanced", synthesis: "strong", reasoning: "complexity" },
};

test("parsePlan accepts the complete transport contract", () => {
  assert.equal(parsePlan(validPlan).expert.id, "coding");
});

test("parsePlan rejects malformed nested fields", () => {
  assert.throws(() => parsePlan({ ...validPlan, selection: { ...validPlan.selection, confidences: { coding: Number.NaN } } }), /confidences/);
  assert.throws(() => parsePlan({ ...validPlan, options: { ...validPlan.options, enable_chunking: "false" } }), /enable_chunking/);
  assert.throws(() => parsePlan({ ...validPlan, tool_names: ["read", 2] }), /tool_names/);
});

test("parseManifest validates providers and experts", () => {
  const manifest = {
    schema_version: 1,
    experts: [{ id: "coding", name: "Coding" }],
    providers: ["ollama"],
    transports: ["stdio"],
    side_effects_enabled: false,
  };
  assert.equal(parseManifest(manifest).providers?.[0], "ollama");
  assert.throws(() => parseManifest({ ...manifest, providers: [1] }), /providers/);
  assert.throws(() => parseManifest({ ...manifest, schema_version: 1.5 }), /schema_version/);
});

test("parseValidation rejects non-integer versions", () => {
  assert.deepEqual(parseValidation({ valid: true, version: 1 }), { valid: true, version: 1 });
  assert.equal(parseValidation({ valid: false, version: 1, error: "configuration is invalid" }).valid, false);
  assert.throws(() => parseValidation({ valid: true, version: Number.POSITIVE_INFINITY }), /version/);
});

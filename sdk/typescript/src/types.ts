export interface RoutingInput {
  topics?: string[];
  has_trace_context?: boolean;
  has_build_context?: boolean;
  has_issue_context?: boolean;
  has_code_context?: boolean;
  history_depth?: number;
}

export interface ExpertView {
  id: string;
  name: string;
  description?: string;
  default_tier: "fast" | "balanced" | "strong" | "unspecified";
  can_synthesize?: boolean;
}

export interface SelectionView {
  primary: string;
  secondary?: string[];
  confidences: Record<string, number>;
  reasoning: string;
}

export interface PlanView {
  expert: ExpertView;
  selection: SelectionView;
  system_prompt_augmentation?: string;
  tool_names?: string[];
  loaded_skills?: string[];
  omitted_skills?: string[];
  options: {
    tool_call_model?: string;
    synthesis_model?: string;
    response_token_budget: number;
    enable_chunking: boolean;
    max_iterations: number;
    max_tool_calls: number;
  };
  tier: {
    tool_call: string;
    synthesis: string;
    reasoning: string;
  };
  policy_version?: string;
  catalog_version?: string;
  context_digest?: string;
}

export interface Manifest {
  schema_version: number;
  experts: ExpertView[];
  providers?: string[];
  side_effects_enabled: boolean;
  transports: string[];
}

export interface ValidationResult {
  valid: boolean;
  version: number;
  error?: string;
}

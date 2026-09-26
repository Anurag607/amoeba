# agentic-moe

> Production foundation: v0.1.x

A reusable Mixture-of-Experts (MoE) orchestration framework for Go LLM agents
built on [langchaingo](https://github.com/tmc/langchaingo).

The framework owns the domain-agnostic control plane: expert routing,
versioned policy and tool admission, immutable identity, bounded delegation,
durable execution attempts and trajectories, revisioned context, workspace
leases, extension lifecycle, adaptive planning, and skill-based knowledge.
Concrete model clients, databases, transports, approval UIs, sandboxes, and
host side effects remain application responsibilities.

agentic-moe is a framework and package, not an application or hosted control
plane. The Go API is canonical. The CLI, MCP server, and TypeScript SDK are
thin adapters over the same runtime and never duplicate routing logic.

## Install and start

Build the CLI from source:

```sh
go install github.com/Anurag607/amoeba/cmd/agentic-moe@latest
go install github.com/Anurag607/amoeba/cmd/agentic-moe-eval@latest
agentic-moe doctor
agentic-moe mcp stdio
```

After published releases are available, the same binary is installable with
Homebrew and the typed launcher/client with npm or pnpm:

```sh
brew install --cask Anurag607/tap/agentic-moe
npm install agentic-moe
# or: pnpm add agentic-moe
```

Generate a host config without overwriting existing files:

```sh
agentic-moe init --target generic    # .mcp.json
agentic-moe init --target vscode     # .vscode/mcp.json
agentic-moe init --target codex      # mergeable TOML fragment
agentic-moe init --target ollama     # framework config with discovery enabled
```

MCP uses stdio or stateless Streamable HTTP. The exposed operations plan and
inspect only; model execution, tool execution, credentials, approvals, and
other side effects stay with the host. See [integrations](docs/integrations.md),
[configuration](docs/configuration.md), and the [security policy](SECURITY.md).

### Go SDK

```go
cfg := runtimekit.DefaultConfig()
runtime, err := runtimekit.New(cfg, runtimekit.Options{})
if err != nil { log.Fatal(err) }
defer runtime.Close()

plan, err := runtime.Plan(context.Background(), "Review this API", runtimekit.RoutingInput{
    HasCodeContext: true,
})
```

### TypeScript SDK

```ts
import { AgenticMOEClient } from "agentic-moe";

const client = await AgenticMOEClient.stdio();
const plan = await client.plan("Compare these deployment options");
await client.close();
```

## Packages

| Package | Purpose |
|---|---|
| [`log`](log) | Swappable logger interface (no-op by default). Call `log.Set(yourLogger)` from init. |
| [`policy`](policy) | Layered allow/deny/require-approval rules. Immutable, versioned snapshots fail closed and filter discovery as well as execution. |
| [`execution`](execution) | Immutable identity, safe-boundary input admission, session coordination, root-run projection, deferred versioned tool discovery, canonical operation envelopes, owner-scoped CAS attempts/approvals/tool rounds, revisioned plans/tasks, task mailboxes, and child runs. |
| [`continuation`](continuation) | Content-addressed parked checkpoints, authoritative resume proofs, leased resume claims, effect-unknown reconciliation, and leased at-least-once transition outboxes with corruption recovery. |
| [`contextengine`](contextengine) | Stable source keys, revisions, epochs, durable context heads, explicit turn reconciliation/commit/rebaseline, centrally bounded spill-to-store output, and provenance-bearing retrieval adapters. |
| [`trajectory`](trajectory) | Sanitized append-before-delivery events with monotonic sequences, idempotent IDs, ACK cursors, bounded retention, degraded-state reporting, and authoritative-refresh signals. |
| [`workspace`](workspace) | Canonical-root owner leases, managed worktrees, generation-pinned terminal ownership, reproducible environments, and fail-closed sandbox-driver admission. |
| [`extension`](extension) | Topological activation, cycle rejection, rollback, health quarantine, transactional update/unregister, cross-registry transactions, trust metadata, and stable active snapshots. |
| [`adapter`](adapter) | Allowlisted MCP and named external-harness lifecycle management, plus manifest-pinned resume capabilities, concurrency bounds, and event provenance checks. |
| [`skills`](skills) | Scoped, precedence-aware `Skill` registry with schema/digest/trust metadata, bounded deterministic prompt assembly, and optional malformed-entry quarantine. |
| [`guardrail`](guardrail) | `WrapToolOutput` — wraps untrusted tool results in sentinels and defangs known prompt-injection markers. |
| [`agent`](agent) | Tool registration builder and sealed `AdmittedAgent`; execution is impossible until policy, catalog, identity, invocation, ledger, and approval boundaries are installed. |
| [`moetools`](moetools) | The `load_skill` and `load_reference` tool definitions + handlers. |
| [`moe`](moe) | Experts, routing, tier decisions, policy-filtered child admission, adaptive learning, multi-objective provider ranking, resource/health profiles, durable delegation leases, typed artifacts, and cross-expert tools. |
| [`conformance`](conformance) | Reusable owner-isolation, exact-retry, CAS, cursor, and close/reopen probes for host-supplied durable stores. |
| [`workflow`](workflow) | Optional deterministic DAG execution with durable node checkpoints, dependency failure propagation, and explicit indeterminate-node reconciliation. |
| [`research`](research) | Optional immutable source snapshots, evidence, claims, claim-evidence edges, citation verification, gap detection, and contradiction assessment. |
| [`runtimekit`](runtimekit) | Stable production composition facade, strict versioned config, serializable plans, manifest, and health. |
| [`provider/ollama`](provider/ollama) | Bounded read-only Ollama health and installed-model discovery. |
| [`mcpserver`](mcpserver) | Official MCP SDK server over stdio and stateless Streamable HTTP. |
| [`configgen`](configgen) | Explicit config generation for generic MCP, Codex, VS Code, and Ollama users. |
| [`evaluation`](evaluation) | Offline routing holdouts plus opt-in live capability cases, provider-neutral structured-output contracts, categorized deterministic scoring, bounded Ollama generation, and redacted reports. |

## Operational contract

- Supported config schema: version 1, with strict unknown-field rejection and
  in-memory migration of versionless files.
- HTTP defaults to loopback. Non-loopback binds require bearer authentication;
  exact origins, host authority, request size, duration, concurrency, headers,
  and shutdown are validated or bounded. Responses disable caching and MIME
  sniffing.
- Ollama discovery defaults to loopback and never downloads or invokes models.
  Redirects are rejected, including when callers supply an HTTP client, and
  the configured deadline and response-size ceiling always apply.
- Release archives target macOS and Linux on amd64/arm64 plus Windows amd64,
  and include checksums, embedded build metadata, archive SBOMs, and GitHub
  build-provenance attestations. CI actions are pinned by commit digest.
- Versioning follows SemVer. Before 1.0, minor releases may change public
  contracts; patch releases remain backward compatible.

See [release policy](docs/releasing.md), [troubleshooting](docs/troubleshooting.md),
the [capability evaluator](docs/evaluation.md), and [contributing](CONTRIBUTING.md).

For maintainers, `make verify` is the complete source gate and includes
race detection, TypeScript packaging, version consistency, layout limits, and
clean npm/pnpm installation plus a reproducible binary comparison. `make
certify` adds static analysis,
dependency vulnerability checks, and npm audit; `make release-check` builds all
release archives and SBOMs twice, verifies byte reproducibility and packaging,
and does not publish.
Release builds inject version, commit, and date metadata. Module-aware source
installs also derive available version-control metadata from Go build info.

## Design

**Planner-style orchestrator.** `moe.Orchestrator` does not run your agentic
loop. It returns a `Plan` describing:

- the selected expert (and routing rationale),
- pre-rendered skill context to append to your system prompt,
- the `[]llms.Tool` to expose to the model,
- the per-step model tier decision (mapped via your `TierMapping`),
- the loop caps (`MaxIterations`, `MaxToolCalls`, …).

You compose messages, call your model, and dispatch tool calls — the
framework just tells you _which expert_, _which skills_, _which tools_,
_which model tier_.

**Evidence-based routing.** Router keywords match case-folded Unicode tokens or
contiguous multi-token phrases, never arbitrary substrings. Configure explicit
variants such as `deploy` and `deployment`; the router intentionally does not
apply broad stemming. Query evidence and ambient context are scored separately.
Sparse or unknown requests fall back to the `general` expert, while synthesis
escalation requires at least two lexical signals from each of two distinct specialist
domains. The synthesis expert may still win directly when the user explicitly
uses its configured planning or architecture vocabulary. This keeps words such
as `explanation` from matching `plan` and `diagnostic` from matching `api`.

**Cross-expert delegation.** The central agent gets two tools out of the box:

- `consult_expert` — advisory: ask a specialist a focused question. No tool
  execution; just the expert's domain knowledge speaking.
- `delegate_to_expert` — hands a bounded sub-task to another expert which runs
  its own loop inside an admitted model/tool envelope and returns a typed,
  provenance-validated artifact.

Both are wired in via consumer-supplied callbacks (`ConsultFn`, `DelegateFn`)
so the framework never has to know about your loop or model client. Consult
and delegate results are untrusted model output and are wrapped by default.

**One root-run lease.** Install one `DelegationLease` in the root execution
context. Every child shares depth, node, child, reasoning-child, parallelism,
work-unit, delegated-token, request, aggregate-artifact, and deadline limits.
Delegated children cannot spend the root-finalization token reserve. When a
run pauses, checkpoint `lease.Snapshot()`; durable hosts should write it
through a `DelegationLeaseStore` compare-and-swap revision so concurrent
continuations cannot spend the same budget. Restore it before resuming;
constructing a fresh lease would reset cumulative limits.

**Separated trust domains.** `DelegateRequest.Task` is the trusted task.
`SelectedContext` contains opaque references only. A host `ContextResolver`
turns them into canonical `ContextFrames`, which must enter the consumer's
context compiler as data frames—never through a system prompt or by string
concatenation with the task. The model cannot submit context content, and the
legacy free-form `context` argument is rejected by strict JSON decoding.

**Model-tier routing.** `ComplexityClassifier` picks `ModelTier{Fast,
Balanced, Strong}` per step from cheap heuristics (query length, history
depth, multi-domain routing confidence, synthesis vs not). You map tiers to
concrete model names via `TierMapping`. Experts can also pin a
`DefaultTier`.

**Admission and execution are separate checks.** Build a versioned
`policy.Snapshot` and `execution.CatalogSnapshot` at run admission. Filter
model-visible tools with that policy and an exact `CapabilityCeiling`, then
dispatch each call through `execution.Dispatcher`. The dispatcher resolves
the exact tool version, validates and canonicalizes arguments, derives the
resource and any nested expert/skill capabilities, seals one operation
envelope, and only then evaluates policy or exposes an approval. Approval,
intent, dispatch, settlement, and reconciliation all consume that same
argument/target/schema/policy/catalog-bound envelope. Exact retries join the
existing attempt; conflicting reuse is rejected.

**Inputs are admitted before scheduling.** `execution.AdmissionStore` records
only owner identity and a request digest. `SessionCoordinator` distinguishes
queue from steer and persists an explicit `now`, `next`, or `later` safe
boundary, serializes each session, applies a global active-run bound, coalesces
wakes, supports exact retry/join and interrupt, and reconstructs its
process-local scheduler state from durable pending records. `RunStore` keeps
execution and delivery state separate so a completed run can still have a
pending or failed delivery. Before provider inference,
`AdmissionEnvelopeBuilder` projects unresolved or indeterminate effects into
the canonical admission digest; `ReadyForProvider` remains false until those
effects are reconciled.

**A tool round must converge, not merely stop.** `execution.ToolRoundStore`
records proposed calls in order and advances them under a round-wide CAS
revision. Required work converges only after success or an explicit host-marked
safe omission; pending approvals, live calls, unsafe omissions, and
indeterminate effects remain blockers. This record complements the exact
per-operation `AttemptLedger` rather than replacing it.

**A continuation is a coupled proof, not a resume token.** The
`continuation` package seals the owner, runtime/harness manifest and selected
resume capability, model,
environment, policy/catalog, context head, exact approved tool operation,
delegation lease, trajectory cursor, and hashed resume token into one immutable
checkpoint. A tool result becomes resumable only after a receipt binds its
semantic and observable hashes to the exact durable result event and a newer
context projection proves incorporation. `RecoveryCoordinator` reconstructs
that proof from a host-owned `EvidenceReader` immediately before the store
leases one resume claim under CAS; callers do not supply their own evidence.
Unknown effects and expired claims enter reconciliation rather than being
retried automatically. Every mutation and its metadata-only outbox event share
one storage transaction, closing the crash gap between authoritative state and
replayable notification. Delivery workers lease those events, retry with
bounded backoff, and leave delivered tombstones. Sinks must deduplicate by the
stable event ID because delivery is intentionally at-least-once. A hash mismatch
is emitted only as a safe reconciliation-required event, never as the corrupted
terminal claim.

**Plans and teams are durable but narrow.** `execution.PlanStore` and
`TaskStore` provide revisioned plan steps, immutable task ancestry, claims,
terminal state, and plan-staleness detection. `TaskMailbox` carries only
payload digests through monotonic per-recipient cursors. The optional
`workflow` package adds deterministic DAG execution without making every agent
run a workflow.

**Context is a revisioned projection.** `contextengine.Engine` reconciles
stable-keyed sources in deterministic order. A temporarily unavailable source
retains the last admitted frames and emits an explicit update; an unavailable
initial source fails. Workspace changes and destructive compaction start new
epochs. Oversized frames require a `SpillStore`, preserving the complete body
behind an opaque reference while only a bounded preview enters the prompt.

**Events are durable facts, not UI state.** `trajectory.Emitter` redacts an
event before append. Stores assign monotonic sequence numbers and replay
strictly after a cursor. ACKs are monotonic, retention never reuses sequence
numbers, an expired cursor requires an authoritative state refresh, and
persistence failure is exposed as degraded reliability. Event payloads should
contain operational metadata, never prompts, credentials, or raw tool output.

## Minimal usage sketch

```go
// 1. Skills (typically embedded into your binary)
//go:embed all:skills
var skillFS embed.FS

skillReg := skills.NewRegistry()
_ = skills.LoadFS(skillReg, skillFS, "skills")

// 2. Experts
expertReg := moe.NewRegistry()
expertReg.Register(&moe.ExpertDefinition{
    ID: "code", Name: "Code Expert",
    SkillIDs: []string{"code_review_strategy"},
    ToolSetName: "code",
    Keywords: []string{"diff", "review", "commit"},
    DefaultTier: moe.ModelTierBalanced,
})
// … register more experts (including one with CanSynthesize=true) …

// 3. Immutable policy + execution catalog
policySnapshot, _ := policy.NewSnapshot("policy-v1", admittedRules...)
catalogSnapshot, _ := toolCatalog.Snapshot("catalog-v1")

// Model schemas and dispatch now come from this same snapshot.
capabilities := moe.CapabilityResolverFromCatalog(catalogSnapshot, policySnapshot)

// 4. Orchestrator
orch := moe.New(moe.Config{
    Experts: expertReg,
    Skills:  skillReg,
    Policy: &policySnapshot,
    CatalogVersion: catalogSnapshot.Version(),
    CapabilityResolver: capabilities,
    Tiers: moe.TierMapping{
        moe.ModelTierFast:     "gpt-4o-mini",
        moe.ModelTierBalanced: "gpt-4o",
        moe.ModelTierStrong:   "gpt-4o-2024-11-20",
    },
    DefaultOptions: moe.LoopOptions{
        MaxIterations: 8, MaxToolCalls: 12, MinToolCallsBeforeFinish: 1,
    },
    // Zero value denies all delegated tools. Admit exact read-only names.
    DelegatedToolCeiling: moe.NewCapabilityCeiling("read_file", "search"),
    MaxInjectedSkillTokens: 4096,
})

// 5. Plan + run your own loop
plan, _ := orch.Plan(userQuery, moe.RoutingContext{HasCodeContext: true})
// plan.SystemPromptAugmentation → append to your system prompt
// plan.Tools                    → expose to the model
// plan.Options.ToolCallModel    → which model to call for tool turns
// plan.Options.SynthesisModel   → which model for the final synthesis turn
```

For the central coordinator path, call `orch.PlanCentral()` and register
`moe.RegisterConsult(...)` / `moe.RegisterDelegate(...)` on an `agent.BaseAgent`.
Seal that builder with the exact admitted policy/catalog and ledger before it
can execute. Every call also requires immutable identity and invocation
metadata in context. Before running the root loop, install its lease:

```go
lease := moe.NewDelegationLease(runID, moe.DefaultDelegationLimits())
ctx = moe.WithDelegationLease(ctx, lease)

err := moe.RegisterDelegateWithOptions(&central, expertReg,
    func(ctx context.Context, req moe.DelegateRequest, expert *moe.ExpertDefinition) (*moe.DelegateResult, error) {
        plan, err := orch.PlanForExpertWithAdmission(expert.ID, moe.DelegationAdmission{
            ToolCeiling:    moe.NewCapabilityCeiling("read_file", "search"),
            Policy:         &admittedPolicy,
            ToolCallModel:  admittedModel,
            SynthesisModel: admittedModel,
            ChildProfile: moe.ChildProfile{
                WorkspaceLease: childLeaseID,
                ContextSources: []string{"workspace", "selected-artifacts"},
                OutputTokenBudget: moe.DelegationTokenGrantFromContext(ctx),
            },
        })
        if err != nil { return nil, err }

        // Give req.Task to the child as its request. Give req.ContextFrames
        // to the context compiler as untrusted data. Enforce both
        // plan.Options and moe.DelegationTokenGrantFromContext(ctx).
        artifact, usage, err := runChild(ctx, plan, req)
        if err != nil { return nil, err }
        return &moe.DelegateResult{Artifact: artifact, Usage: usage}, nil
    }, moe.DelegateOptions{
        ResolveContext: func(ctx context.Context, ref string) (moe.ContextFragment, error) {
            return authorizedContextStore.Resolve(ctx, ownerID, ref)
        },
})

admittedCentral, _ := central.Seal(agent.Admission{
    CatalogVersion: "catalog-v1", Policy: policySnapshot,
    Ledger: attemptStore, Approvals: approvalStore,
})
ctx, _ = execution.WithIdentity(ctx, admittedIdentity)
ctx, _ = agent.WithToolInvocation(ctx, agent.ToolInvocation{
    AttemptID: callID, RequestID: requestID, IdempotencyKey: idempotencyKey,
})
out, err := admittedCentral.ExecuteTool(ctx, "delegate_to_expert", arguments)
```

`PlanForExpert` applies the configured delegated ceiling. Its zero value is
fail-closed. `PlanForExpertWithAdmission` additionally pins the admitted
model, policy snapshot, and child profile. The profile's zero value inherits
no transcript, memory, persistence, mutation, approvals, or network access.
`PlanForExpertUnrestricted` is intentionally explicit and should only be used
for trusted/manual runs.

Successful child artifacts require a summary and coverage. When selected
context or prior artifacts are supplied, returned source references must be a
subset of those server-selected references. Prior artifacts can only be
reused unchanged from the same lease.
When a child relies on one, it cites `moe.DelegationArtifactRef(artifact)` in
the returned artifact's source references.

### Delegation API migration

The hardened delegation contract intentionally replaces the original
free-form callback:

- `DelegateRequest.Context string` is replaced by opaque
  `SelectedContext []string` references and server-resolved `ContextFrames`;
- `DelegateFn` returns `*DelegateResult` rather than a raw string;
- `DelegateResult.Artifact` must satisfy provenance and coverage validation;
- execution requires a run-level lease in the context;
- `PlanForExpert` is now fail-closed for tools unless a delegated ceiling is
  configured.

These are security-boundary changes rather than compatibility aliases. A
legacy raw-context adapter would recreate the trust-promotion flaw the new API
is intended to remove.

### Continuation and harness recovery migration

Harness adapters that can resume should implement
`adapter.HarnessCapabilityReporter`. The resulting manifest pins token,
transcript, and exact-checkpoint support independently. Every resume request
must select `RequiredResume` and supply that mode's evidence; an exact-checkpoint
resume supplies both `ContinuationID` and `CheckpointHash`. A harness that does
not report a capability is not assumed to have it. Parked checkpoints must set
`RuntimeBinding.ResumeCapability` to the exact selected mode. A paused
`HarnessResult` likewise declares `ResumeMode` and returns only that mode's
required token, context digest, or continuation/checkpoint binding.

Durable `continuation.Store` implementations now provide `LeaseOutbox`,
`RetryOutbox`, and `AckLeasedOutbox`. Persist status, attempt count, retry time,
lease, content hash, expected continuation state/revision, operation key, and
delivered tombstone with the event. An expired lease must be reclaimable after
restart, while stale lease IDs and attempt numbers fail closed. The older
`PendingOutbox` and `AckOutbox` methods remain inspection/manual-administration
surfaces; concurrent delivery should use `DrainOutbox` and leased ACKs.

## Guardrail

`execution.Dispatcher` automatically wraps output unless the catalog's
immutable `ToolClass.TrustedOutput` marks it author-controlled. The built-in
skill/reference registrations are trusted; consult and delegation results are
not. Hosts should not apply a second execution path or a second trust list.

## Skill prompt budgets

Skills are ordered deterministically by priority and then ID. The orchestrator
injects them up to `MaxInjectedSkillTokens` and reports `Plan.LoadedSkills` and
`Plan.OmittedSkills`. Omitted skills can still be loaded on demand through
`load_skill`. Frontmatter may provide `token_estimate`, `scope`, `source`,
`trust`, `schema_version`, and `precedence`; the loader computes a stable
digest over the body and sorted references. `LoadFSWithOptions` adds hard
per-skill/aggregate byte budgets and can quarantine malformed entries with
diagnostics while continuing to load healthy skills. Plain `LoadFS` remains
strict.

## Durable and host-owned boundaries

The in-memory stores are reference implementations, not a claim of process
durability. Production hosts supply durable implementations of
`AdmissionStore`, `RunStore`, `AttemptLedger`, `ApprovalStore`, `ChildRunStore`,
`contextengine.SnapshotStore`, `trajectory.ReplayStore`, workspace/worktree
stores, spill storage, `DelegationLeaseStore`, and `continuation.Store`. A
pending durable approval pauses before side effects and can be settled out of
band; the decision must echo the sealed envelope digest, and an exact retry
resumes only that request.
External harness requests carry an immutable name/version/config/health and
resume-capability manifest so resumption cannot silently switch runtimes or
downgrade recovery semantics. Continuation store
implementations must update the checkpoint record and append its outbox event
atomically; implementing those operations as independent writes violates the
contract even when both writes are individually durable. Outbox delivery is a
separate leased phase: a sink success followed by an ACK crash can replay, so
consumers deduplicate on event ID rather than treating transport success as a
new authoritative transition.

`workspace.Environment` is a digest-bound reproducibility manifest covering
image, pinned repositories, setup, runtime, network, resource, secret-binding,
and cleanup configuration. It describes placement and restrictions but does not
turn a local process into a sandbox. The host must enforce filesystem roots,
network policy, resource limits, secret bindings, and cancellation in the
chosen local/container/remote runtime. Likewise, `adapter.MCPServer` and
`adapter.AgentHarness` normalize lifecycle and events without implementing a
transport, OAuth flow, credential store, or generic plugin runtime.
`HarnessSupervisor` supplies manifest verification, active-run bounds,
cancellation, event identity checks, and terminal-result validation around a
named harness. `workspace.SandboxRuntime` accepts a backend only when it
declares that it can enforce every requested placement, network, and resource
restriction; concrete container/WASM/process drivers remain host modules.

Host stores should run `conformance.ProbeCore` before adoption and
`conformance.ProbeRestart` against their real close/reopen path. The restart
probe preserves running input, paused run, indeterminate attempt, pending
approval, open tool round, plan/task, context head, and trajectory event across
the boundary. It also expires and reclaims a leased continuation outbox event
after the close/reopen boundary, then verifies its delivered tombstone
suppresses replay, so a host cannot pass conformance with state-only recovery.

Learning is storage-neutral and opt-out. `moe.EWMAStore` and
`DefaultAdaptivePlanner` provide minimum-sample gating, effective-context fit,
tool-aware chunking, and continuation-based tier escalation. Hosts can replace
either through `moe.StatsLookup`, `moe.StatsRecorder`, and `AdaptivePlanner`.
`RecordOutcome` automatically records expert- and MoE-strategy-level samples. With
`LearningConfig.Disabled`, planning performs no learning reads or writes.
`ProviderCandidate` carries explicit tier, context-window, concurrency,
tool/structured-output, placement, readiness, account binding, memory/cache,
warmup/cold-start, platform, bundle-digest, and health-freshness metadata.
`RankProviders` combines quality, reliability, latency, cost, cold-start, and
uncertainty without treating an unmeasured route as proven. The host still owns
credentials and provider health checks.

## Deliberate host-owned and deferred surfaces

UI projections, approval presentation, provider credentials, encrypted
persistence, retrieval indexes, source acquisition, and concrete process,
container, or WASM isolation remain host-owned. The framework now defines
terminal handoff, sandbox enforcement, retrieval, workflow, and research
evidence boundaries, but does not claim that an interface enforces the host
system. Arbitrary plugin SDKs, semantic-index implementations, scheduled
automation, and transcript branching/revert remain deferred until a concrete
host requires them.

## Repository organization

[`AGENTS.md`](AGENTS.md) is the mandatory contributor contract for automated
coding agents. Source and test files have a 700-line hard ceiling, new files
normally stay at 500–650 lines or fewer, and each directory may contain at most
19 immediate production source files and 19 immediate test files. Package tests
remain co-located so the standard Go toolchain can exercise private contracts;
repository-wide invariant tests live under `test/`.

`TestRepositoryLayout` enforces the line ceiling, separate production/test
directory-density limits, and exclusion of `.DS_Store` metadata. When a package
reaches its density limit, carve out a cohesive child package before adding a
new responsibility.

## Verification

```bash
make verify
```

This formats Go source and runs the layout assertion, tests (including the
frozen development/holdout routing corpus), race detector, coverage, vet, and
full module build. Router microbenchmarks are available with `go test ./moe
-run '^$' -bench BenchmarkRouterRoute -benchmem`.

## Module info

- Go 1.25+
- Depends on `github.com/tmc/langchaingo` and `gopkg.in/yaml.v3`.
- Security, delegation, capability, and deterministic skill-budget behavior
  are covered by offline unit tests.

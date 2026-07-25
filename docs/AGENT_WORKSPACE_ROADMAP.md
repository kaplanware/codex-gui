# Agent Workspace Product Roadmap

## Product direction

The product evolves from a Linux Codex client into:

> The open-source command center for coding agents.

Its durable advantage is provider-independent orchestration combined with safety,
verifiable outcomes, and project knowledge. Codex remains a first-class provider,
not the product boundary.

## Product principles

1. A provider is replaceable; task history and evidence belong to the workspace.
2. “Done” is a verification-policy result, not an agent sentence.
3. Every material action is attributable, reviewable, and recoverable.
4. Plans, diffs, test reports, recordings, and rollback points are artifacts.
5. Cloud quota, cost, privacy, risk, and historical success inform routing.
6. Production access and destructive actions require stronger policy gates.

## Target architecture

### Provider adapter

```go
type AgentProvider interface {
    Name() string
    Detect(context.Context) InstallationStatus
    Authenticate(context.Context) error
    StartSession(context.Context, SessionOptions) (Session, error)
    Send(context.Context, string, Input) error
    Interrupt(context.Context, string) error
    Events() <-chan AgentEvent
}
```

Initial adapters:

- Codex
- Claude Code
- Antigravity
- Gemini
- OpenCode
- Ollama and OpenAI-compatible local endpoints

Provider-specific protocol events are normalized into sessions, turns, items,
approvals, usage, artifacts, commands, file changes, and verification evidence.

### Four product layers

| Layer | Responsibilities |
|---|---|
| Universal agent client | Providers, sessions, normalized events, authentication |
| Safety | Risk engine, snapshots, worktrees, sandbox, production guard, rollback |
| Verification | Build, test, lint, browser QA, security and evidence policies |
| Orchestration | Routing, jury, workflows, scheduling, cost and quota optimization |

## Core domain model

- **Task:** provider-independent unit of requested work.
- **Run:** one provider attempt for a task.
- **Agent:** a named role such as planner, implementer, reviewer, or browser QA.
- **Artifact:** versioned reviewable output with lifecycle state.
- **Evidence:** machine-produced proof for a verification requirement.
- **Capsule:** replayable audit record containing prompt, environment, commands,
  approvals, diffs, results, and Git boundaries.
- **Policy:** project or organization rule controlling risk and completion.
- **Memory:** proposed, approved, expired, or conflicting project knowledge.

Artifact lifecycle:

```text
Draft → Needs Review → Approved → Implemented → Verified → Archived
```

## Delivery modes

### Fast

Direct execution for scoped, low-risk work. Policy can still force planning or
verification when critical paths are touched.

### Planned

Read-only repository analysis produces a plan artifact containing assumptions,
affected files, ordered steps, risks, tests, and rollback. Implementation cannot
start until the approval gate is satisfied.

### Verified

Uses the planned gate and a project verification policy. Completion requires
machine evidence for configured build, test, lint, migration, browser, diff, and
log checks.

## Phased delivery

### Alpha — trustworthy Codex workspace

- Codex provider and normalized event foundation
- Projects, threads, approvals, unified diff, model and quota controls
- Structured `@` file context and context visibility
- Git worktree execution
- Task timeline and basic artifacts
- Pre-task snapshot, task capsule, selective rollback
- Plan approval and verification-policy skeleton

Exit criterion: a Codex task can be planned, approved, executed, verified, and
rolled back with a complete local audit trail.

### Beta — provider independence

- Extract current Codex integration behind `AgentProvider`
- Claude Code provider
- Provider capability matrix and graceful feature degradation
- Mission Control for running, waiting, failed, and completed agents
- Test/build artifact viewers
- Context Inspector and context-budget estimator
- Unified cost and quota panel
- Rule-based Smart Router

Exit criterion: the same task contract can run through Codex or Claude while the
UI, safety policy, artifacts, and audit model remain provider-independent.

### V1 — differentiated control plane

- Agent Jury with cost-aware policies
- Antigravity/browser provider integration or Playwright browser worker
- Visual QA recordings, screenshots, console/network evidence
- Production Guard and command Risk Engine
- Living Project Memory with human approval
- Architecture Guardian and requirement traceability
- Documentation drift detection
- Reusable workflow templates

Exit criterion: critical changes can require independent implementation,
review, browser verification, and policy consensus before merge.

### V2 — Agent OS

- Visual workflow builder
- Benchmark arena using isolated worktrees
- Multi-agent debate and solution merging
- Docker, Podman, Distrobox, Nix, and remote dev capsules
- Team policy engine and shared workflow marketplace
- Repository-specific failure library and adaptive preflight checks

## Priority capabilities

### Mission Control

Shows role, provider, task, status, elapsed time, quota/cost, files changed,
commands, approvals, blockers, and branch/worktree for every active agent.

### Safety and Time Machine

Before a run, capture Git SHA, dirty files, selected configuration, environment
fingerprint, database metadata, and services. Support code-only, selected-file,
database, or complete-task rollback.

### Risk Engine

Resolve command paths before approval and classify read-only, low, medium, high,
or critical risk. Detect destructive Git, filesystem, database, container,
Kubernetes, Terraform, privilege, secret, and production operations.

### Smart Router

Start with auditable YAML rules based on task type, repository stack, browser
needs, privacy, quota, cost, and user preference. Add historical success metrics
only after enough verified task data exists.

### Agent Jury

Modes: off, critical files, before commit, before pull request, always. Reviews
remain independent until a consensus report highlights agreements, conflicts,
severity, evidence, and recommended action.

### Context and memory

Expose included, excluded, pinned, ignored, and sensitive context with reasons.
Memory changes enter a proposal queue and never silently become project truth.

### Verification

Policies map requirements to commands and evidence. A run is complete only when
required evidence is fresh, attributable to the final diff, and successful.

## Near-term engineering sequence

1. Define normalized provider capability and event contracts.
2. Move Codex process/RPC code into `CodexProvider`.
3. Add task, run, artifact, evidence, policy, and capsule tables.
4. Build worktree/snapshot service and rollback UI.
5. Convert the current plan review into persisted artifacts.
6. Add verification runner and completion gates.
7. Build Mission Control on normalized runs.
8. Implement Claude provider as the adapter architecture proof.
9. Add context inspector, quota/cost model, and rule-based router.
10. Add browser worker, Jury, Production Guard, and workflow templates.

## Explicit non-goals for the next milestone

- A drag-and-drop workflow builder before the execution model is stable
- Learned routing before verified historical data exists
- Automatic production mutation
- Silent permanent project memory
- Pretending provider quota can be manually reset when its API does not allow it


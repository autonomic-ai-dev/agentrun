# Feature Contract: Project X Integration with AgentRun

Integration of the Project X declarative execution protocol (pre-execution AST policy validation, concurrent DAG orchestration, cryptographic receipts) with AgentRun's isolation and provenance pipeline.

---

## Sub-features

### `projectx-preflight-validation`
- **Intent**: Validate declarative execution plans (`ExecutionPlan` YAML/JSON) using `project-x` CLI before provisioning containers or git worktrees.
- **Behavior**: Executes `project-x validate <plan>`, parses DAG topological waves and digest, and maps validation rejections to standard AgentRun 4-line errors.
- **Outcome**: Immediate (<10ms) rejection of cyclic dependencies or schema violations with zero host or container resource allocation.

### `projectx-receipt-inspection`
- **Intent**: Ingest and inspect Project X cryptographic execution receipts (`receipt.json`).
- **Behavior**: Decodes step execution statuses, durations, and cryptographic input/output hashes, with support for both in-process schema parsing and CLI `project-x inspect`.
- **Outcome**: Deterministic step verification and audit records for each executed plan.

### `projectx-provenance-record`
- **Intent**: Bind the Plan Digest to AgentRun's immutable provenance record.
- **Behavior**: Extends `record.Provenance` to store and validate `plan_digest`.
- **Outcome**: Immutable provenance linking host policy, runtime environment, and cryptographic plan digest.

### `projectx-evidence-reporting`
- **Intent**: Display Project X verification breakdown in `agentrun-dev evidence`.
- **Behavior**: Reads `receipt.json` alongside `outcome.json` in task directories and renders step-by-step verification outcomes.
- **Outcome**: Unified end-to-end evidence reporting.

---

## How to get to it (user POV)

- **Go Package API**:
  ```go
  runner := projectx.NewRunner("/path/to/project-x")
  val, err := runner.ValidatePlan(ctx, "plan.yaml")
  receipt, err := runner.InspectReceipt(ctx, "receipt.json")
  ```
- **CLI Subcommand**:
  ```bash
  agentrun-dev evidence <run-dir>
  ```
- **Provenance Encoding**:
  ```go
  prov, _ := record.NewProvenance(mode.Lite, "local", policyDigest)
  prov, _ = prov.WithPlanDigest("2e291eee7f8d565316bec0b0ad815aa9e4e95b8521d1bba42c10288b4778be6a")
  ```

---

## Verification Suites (Targeted Test Commands)

- **Unit Command**: `go test -race -count=1 ./internal/projectx/... ./internal/record/... ./cmd/agentrun-dev/...`
- **Functional / Integration Command**: `go test -race -count=1 -v ./test/integration/...`
- **Allowlisted Command**: `sh .verify/run.sh all`

### Step 1: Valid Plan Pre-flight Validation
- **Request:** `runner.ValidatePlan(ctx, "valid-multi-step.yaml")`
- **Why:** Verifies that a well-formed DAG plan validates successfully and extracts plan name, API version, digest, and topological waves.
- **Expected:** `err == nil`, `val.PlanName == "valid-multi-step"`, `val.StepsCount == 2`, `val.WavesCount == 2`.
- **Impact:** Plan is authorized for container execution.

### Step 2: Cyclic Dependency Plan Rejection
- **Request:** `runner.ValidatePlan(ctx, "13-cycle-dependency-error.yaml")`
- **Why:** Proves that an invalid cyclic dependency DAG is rejected pre-flight before container launch.
- **Expected:** Non-nil error matching `errors.Error` 4-line standard format, containing "Cyclic dependency detected".
- **Impact:** Zero containers spawned, immediate task exit.

### Step 3: Receipt Ingestion and Step Status Verification
- **Request:** `runner.InspectReceipt(ctx, "sample-receipt.json")`
- **Why:** Validates that completed execution receipts correctly reflect step execution states and durations.
- **Expected:** `receipt.Status == "failed"`, `receipt.PlanID == "valid-multi-step"`, step "deployment" status "failed", step "logs" status "skipped".
- **Impact:** Audit records accurately trace tool executions.

### Step 4: Provenance Record with Plan Digest
- **Request:** `prov.WithPlanDigest(sampleDigest)` and JSON roundtrip
- **Why:** Ensures `plan_digest` is strictly validated (64 lowercase hex chars) and preserved across JSON serialization.
- **Expected:** `unmarshaled.PlanDigest == prov.PlanDigest`.
- **Impact:** Run provenance cryptographically references the exact plan executed.

### Step 5: Evidence CLI Output with Receipt Breakdown
- **Request:** `agentrun-dev evidence <testdir>` where `<testdir>` contains `outcome.json` and `receipt.json`.
- **Why:** Proves that operator can inspect both test harness outcomes and Project X step-by-step verification.
- **Expected:** Exit code 0, output contains "status: passed" and "project-x receipt: valid-multi-step" with step statuses.
- **Impact:** Full visibility into agent verification evidence.

---

## Gotchas

- **Executable Resolution**: If `project-x` is not in `$PATH`, `NewRunner` allows explicit binary path injection, with fallback resolution checking `/Volumes/Seagate/developer/personal/autonomic-ai-dev/project-x/target/release/project-x` or standard system paths.
- **Digest Format Compatibility**: Project X digests are 64 lowercase hex characters; AgentRun internal policy digests prefix `sha256:`. `WithPlanDigest` normalizes bare 64-hex hashes to prefix `sha256:` while preserving exact match semantics on unmarshal.

# Feature Contract: Run Engine

The run engine stores each run as a directory under the data dir, executes its stages (worktree, agent, verify, commit), and exposes status, result, and cancel. The agent adapter maps an agent type to its command line, tool allowlist, and credentials.

---

## Sub-features

### `run-store`
- **Intent**: Create, read, list, and update runs on disk.
- **Behavior**: `CreateRun(SubmitConfig)` writes a run directory; `GetRun`, `ListRuns`, `GetStatus`, `SetStatus` read and update it. `ValidID` and `GenerateID` constrain run IDs.
- **Outcome**: Each run has one directory whose status is readable without the process that created it.

### `run-execution`
- **Intent**: Run the verify loop for one run, then commit and diff the result.
- **Behavior**: `Execute(ctx, run, ExecutionOptions)` runs `verify.Loop` in the run's worktree, saves `outcome.json` via `verify.SaveOutcome`, and if verification passed commits the changes and records a diff against the base commit.
- **Outcome**: The run ends `succeeded` or `failed` with a recorded reason.

### `run-cancel-and-result`
- **Intent**: Cancel a run and read its result.
- **Behavior**: `Cancel(dataDir, id)` marks the run cancelled; `GetResult(dataDir, id)` returns the stored `Result`.

### `agent-adapter`
- **Intent**: Describe how each supported agent is launched and what it may touch.
- **Behavior**: `CommandFor(agent, prompt)` builds the argv (the prompt is passed as an argument value, not interpolated into a shell); `AllowlistFor` returns the tool allowlist; `ProvisionCredentials` collects only the environment variables that agent needs.

---

## How to get to it (user POV)

- **Store**: `engine.CreateRun(engine.SubmitConfig{...})`, `engine.GetRun(dataDir, id)`
- **Execute**: `engine.Execute(ctx, run, engine.ExecutionOptions{...})`
- **Cancel / result**: `engine.Cancel(dataDir, id)`, `engine.GetResult(dataDir, id)`
- **Agent command**: `agent.CommandFor(agent.AgentType, prompt)`
- **Agent image**: `images/agent-claude/Dockerfile` (Node 22, Go 1.27.2, gcc for `-race` checks, `@anthropic-ai/claude-code` 2.1.299); the same image runs the agent and the checks.
- **Credentials**: the CLI passes `agent.ProvisionCredentials(...)` into `ContainerOptions.Env`; the engine merges `HTTPS_PROXY` into a copy of it for each container.
- **CLI**: `agentrun submit "<task>" ...`, `agentrun status|result|logs|cancel <id> [-data dir]` (`cmd/agentrun`, wraps the functions above)

---

## Verification Suites (Targeted Test Commands)

- **Unit Command**: `go test -race -count=1 -cover ./internal/engine/... ./internal/agent/...`
- **Allowlisted Command**: `sh .verify/run.sh all`

---

## Code

- `internal/engine/`
- `internal/agent/`
- `cmd/agentrun/`
- `images/agent-claude/`

---

## Gotchas

- The run engine is uncommitted work in progress; the README still says the engine is not built.
- The prompt text is never interpolated into a shell command. Keep it that way when adding agent types.
- `Cancel` and run start can race; cancel only after a run shows `running` with an agent PID.

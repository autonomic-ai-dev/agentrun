# Decision: Phase 0 gates

Status: accepted. The owner delegated these calls to the agent. Each one can be reversed by a later decision note.

Context: `docs/implementation-plan.md` lists three decisions for a human. This note records them.

## Decisions

1. **Steps 1–4 are approved.** They do not change any frozen artifact. `prototype/` is untouched by the Step 1–4 commits (`da3c2f0`, `7e298ed`, `43037ea`, `6a275c2`, `3f296c4`).

2. **The dev CLI stays a separate binary.** `cmd/agentrun-dev` is for developing this repository and is not the product. Folding it into the future `agentrun` binary would ship developer tooling to users. Revisit when `agentrun` exists.

3. **Step 5 starts now, with its scope limited to the attached backend.** Step 5 needs the P0 isolation items to be resolved, and they are not all resolved:
   - Detached exit codes: resolved by decision. Runs are attached and the supervisor records the exit code. `TestDetachedExitStatusIsUnavailableFromInspect` confirms the limit. See `docs/decisions/apple-container-backend.md`.
   - Egress allowlist: decided (dual-homed sidecar, `internal/egress`). Not yet proven with an authenticated prompt, which needs `GEMINI_API_KEY` or `CURSOR_API_KEY`.
   - Host sleep and service restart: **open.** No test in `test/capability/` covers it, and no decision record covers it.

   Step 5 may therefore start, and its done-when check (capability suite plus a submit-to-patch run) must pass. The sleep and restart item stays open. It blocks unattended use and Step 6 (parallel runs), not Step 5.

## Why

- Step 5 is the first point where the engine runs real agents. Holding it for every P0 item would stall the work, and the sleep and restart gap only matters once runs are unattended.
- Starting Step 5 without the sleep and restart test would be an unmeasured risk. Making it a blocker for unattended use puts the risk where it bites.

## Consequences

- Step 6 (parallel runs) waits for the sleep and restart item to have a test and a decision.
- Before any unattended run, add a capability test that suspends the host or restarts the container service during a run, and record what the run does.

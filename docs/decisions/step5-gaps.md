# Step 5 gaps

Status: working list. Each gap says whether it is fixed, prepared, or waiting for a decision.

## Fixed

- **Cancel did not stop a container.** Container runs record their name in `container.name` in the run directory. `Cancel` runs `container stop` on that name. Test: `TestCancelStopsRunningContainer` (live).
- **Default `git status` check does not work inside a container.** The worktree's `.git` file points at a host path. `Execute` now refuses to start a container run that has no explicit checks and no `.verify/run.sh`, with `ErrNoContainerChecks`. Nothing is started and the status stays `queued`. Test: `TestExecuteInContainerRequiresChecksWhenRepoHasNoVerifyScript`.

## Known limit that is not fixed

- **A cancel during the start window is lost.** The name is written before `container run` starts the container. If `container stop` runs before the container reaches `running`, the stop does nothing, and the agent runs to the end of its work. Reproduced by hand: a stop issued just after the name was written left the attached client waiting. The live test waits for `running` before it cancels, so it does not cover this window. Fix options: (a) poll `container list` in `Cancel` until the container is running or gone, then stop it; (b) have the engine check the run status while waiting on the attached client and stop the container itself. Recommendation: (b), because it does not depend on the timing of the caller.

## Prepared, not built

- **Submit command.** The Go engine has no CLI. `prototype/agentrun-proto submit` is still the only submit path. Proposed: `agentrun submit "<task>" --repo PATH [--include-dirty] --image REF --network NAME`, calling `CreateRun` then `Execute` with a `ContainerOptions` built from the flags. It is blocked on the egress decision below, because a submit that runs agents needs a network choice.
- **Egress sidecar launcher.** `internal/egress` and `cmd/agentrun-egress` exist, but nothing starts them for a run. The decision in `docs/decisions/apple-container-backend.md` needs: an internal network for agents, a dual-homed sidecar container from `images/egress`, the sidecar's internal address read from `container inspect`, and `HTTPS_PROXY` set in `ContainerOptions`. `Spec.Networks` already supports attaching one container to two networks. Not built yet.
- **Default network is a risk.** `ContainerOptions.Network` is required, but `"default"` gives an agent full outbound access. Until the sidecar launcher exists, do not run untrusted agents with `"default"`.
- **Host sleep and service restart test.** No test in `test/capability/` covers either. A test cannot put the host to sleep. Proposed: a manual procedure (suspend the host during a run, then check that `container stop` on the run's `container.name` leaves no container and that the run's status is consistent), plus a capability test that restarts the container service during a run (`container system stop` then `container system start`) and checks that the run ends with a recorded status and no orphan container. Both are needed before unattended runs (Step 6).

## Decision needed

- **Exit code and checks.** Today a non-zero agent exit does not fail a run if the checks pass. The exit code is recorded and the agent error is kept in `outcome.json`. Options:
  - A. Keep it: checks decide the outcome. Simple, matches the host path.
  - B. Fail the run on any non-zero agent exit, even when checks pass. Safer: an agent that crashes midway cannot produce a successful run.

  Recommendation: B. It changes the host path too, so it needs its own test update. Not changed yet.

# Step 5 gaps

Status: the gaps listed in the earlier version of this note are closed, except where a line below says otherwise.

## Done

- **Cancel did not stop a container.** Container runs record their name in `container.name`. `Cancel` runs `container stop` on it. Test: `TestCancelStopsRunningContainer`.
- **A cancel during container start was lost.** Each container run has a watcher that retries `container stop` while the run is cancelled, until the run returns. Test: `TestCancelBeforeContainerRunningStopsContainer`.
- **Container runs without checks.** A container run with no checks and no `.verify/run.sh` fails before it starts, with `ErrNoContainerChecks`. Test: `TestExecuteInContainerRequiresChecksWhenRepoHasNoVerifyScript`.
- **Exit-code semantics (decision B).** A non-zero agent exit fails the attempt even when the checks pass. The checks do not run for that attempt. The failure is repaired like any failing check. This changes the host path too. Test: `TestLoopFailsAttemptWhenAgentExitsWithError` (this replaces the old test that asserted the checks decide).
- **Egress sidecar wiring.** `Execute` creates an internal network for the run, `<prefix>-<run id>` (`--internal`, subnet assigned by the runtime), starts the sidecar on the default network and that network, gives the agent and its checks only that network with `HTTPS_PROXY` set, and stops the sidecar and deletes the network when the run ends. Test: `TestEgressSidecarBlocksDirectAndUnlistedTraffic`. The probe showed direct egress blocked, the proxy reachable, a 403 for an unlisted host, and a 200 tunnel for a listed host.
- **Engine coverage.** `go test -cover ./internal/engine/` reports 92.6%, up from 86.1%. The egress hooks (`CreateNetwork`, `DeleteNetwork`, `StartSidecar`) are injectable, and the tests `TestExecuteEgress*` cover the egress wiring without touching the machine.
- **Parallel runs (Step 6).** Each run gets its own internal network, so runs cannot reach each other's containers. The decision and the probes behind it are in `docs/decisions/step6-parallel-runs.md`. Test: `TestConcurrentRunsCannotReachEachOthersSidecar` runs two egress runs at once and checks that one run's agent cannot reach the other's sidecar, and that both networks are gone afterwards.
- **Sidecar image.** The sidecar no longer defaults to `agentrun-egress:latest`. An image is required, and the README shows building one with an explicit tag.
- **Submit command.** `cmd/agentrun` provides `submit`. Test: `TestSubmitCommandRunsEndToEnd` runs the built binary against a real repository and checks the patch.
- **Container service restart during a run.** `container system stop` during a run ends `Execute` in a terminal state, leaves no container behind, and the service comes back with `container system start`. Test: `TestRunEndsWhenContainerServiceRestarts`. This test stops every container service on the machine, so it is not part of a routine run.

## Not done

- **Host sleep.** Sleep cannot be automated from a test. Manual procedure: start a submit run with an agent that sleeps for two minutes, put the host to sleep for a minute, wake it, then confirm that `container list --all` shows no `agentrun-` container and that the run status is terminal. Record the result here before unattended runs (Step 6).
- **Real agent prompts.** The agent CLIs (`claude`, `agy`, `cursor`) have not been run with a real prompt inside a container. That needs `ANTHROPIC_API_KEY`, `GEMINI_API_KEY`, or `CURSOR_API_KEY`. The engine assumes each CLI reads its prompt from stdin, which is unverified for these CLIs.
- **Network leak on a killed process.** If the `agentrun` process is killed before Execute's cleanup runs, that run's network and sidecar remain. Nothing sweeps them yet. `container network list` shows them.

## Notes for the operator

- `agentrun-internal` is created on the machine by earlier runs and is not removed by any test or command. Runs now use `agentrun-internal-<run id>`, so the base network is no longer used by the engine.
- The capability suite now includes tests that stop the container service, and tests that build and run the egress image. Each takes a few seconds to a few minutes.

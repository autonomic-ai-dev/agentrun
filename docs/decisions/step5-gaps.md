# Step 5 gaps

Status: the gaps listed in the earlier version of this note are closed, except where a line below says otherwise.

## Done

- **Cancel did not stop a container.** Container runs record their name in `container.name`. `Cancel` runs `container stop` on it. Test: `TestCancelStopsRunningContainer`.
- **A cancel during container start was lost.** Each container run has a watcher that retries `container stop` while the run is cancelled, until the run returns. Test: `TestCancelBeforeContainerRunningStopsContainer`.
- **Container runs without checks.** A container run with no checks and no `.verify/run.sh` fails before it starts, with `ErrNoContainerChecks`. Test: `TestExecuteInContainerRequiresChecksWhenRepoHasNoVerifyScript`.
- **Exit-code semantics (decision B).** A non-zero agent exit fails the attempt even when the checks pass. The checks do not run for that attempt. The failure is repaired like any failing check. This changes the host path too. Test: `TestLoopFailsAttemptWhenAgentExitsWithError` (this replaces the old test that asserted the checks decide).
- **Egress sidecar wiring.** `Execute` creates the internal network `agentrun-internal` (192.168.128.0/24, `--internal`) if it is missing, starts the sidecar on the default network and the internal network, gives the agent and its checks only the internal network with `HTTPS_PROXY` set, and stops the sidecar when the run ends. Test: `TestEgressSidecarBlocksDirectAndUnlistedTraffic`. The probe showed direct egress blocked, the proxy reachable, a 403 for an unlisted host, and a 200 tunnel for a listed host.
- **Sidecar image.** The sidecar no longer defaults to `agentrun-egress:latest`. An image is required, and the README shows building one with an explicit tag.
- **Submit command.** `cmd/agentrun` provides `submit`. Test: `TestSubmitCommandRunsEndToEnd` runs the built binary against a real repository and checks the patch.
- **Container service restart during a run.** `container system stop` during a run ends `Execute` in a terminal state, leaves no container behind, and the service comes back with `container system start`. Test: `TestRunEndsWhenContainerServiceRestarts`. This test stops every container service on the machine, so it is not part of a routine run.

## Not done

- **Host sleep.** Sleep cannot be automated from a test. Manual procedure: start a submit run with an agent that sleeps for two minutes, put the host to sleep for a minute, wake it, then confirm that `container list --all` shows no `agentrun-` container and that the run status is terminal. Record the result here before unattended runs (Step 6).
- **Real agent prompts.** The agent CLIs (`claude`, `agy`, `cursor`) have not been run with a real prompt inside a container. That needs `ANTHROPIC_API_KEY`, `GEMINI_API_KEY`, or `CURSOR_API_KEY`. The engine assumes each CLI reads its prompt from stdin, which is unverified for these CLIs.
- **Engine coverage.** Unit-test statement coverage of `internal/engine` is 86.1%, below the 90% target. The gap is in the egress wiring (`behindProxy`, `startEgress`), which only the live tests reach, and in error branches of the container path.
- **Parallel runs.** Step 6. Not started.
- **Shared network.** All runs share `agentrun-internal` and its subnet. Two concurrent runs each get their own sidecar, but they share one network. Not tested with concurrent runs.

## Notes for the operator

- `agentrun-internal` is created on the machine and is not removed by any test or command.
- The capability suite now includes tests that stop the container service, and tests that build and run the egress image. Each takes a few seconds to a few minutes.

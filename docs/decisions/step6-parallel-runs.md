# Decision: one internal network per run (Step 6, parallel runs)

## Context

`agentrun submit` executes one run per process. Parallel runs means several `submit` processes at once. Today every run that uses egress joins the same network, `agentrun-internal` (192.168.128.0/24), which `egress.EnsureInternalNetwork` creates if missing.

Facts checked on this machine, 2026-11-05:

- **Runs can reach each other on the shared network.** Two containers were started on `agentrun-internal`. One ran `nc` against the other's internal address and port 8080, and the connection succeeded (exit 0). An agent in run A can therefore open connections to any container in run B, including run B's egress sidecar.
- **Creating the network twice fails.** `container network create --internal --subnet 192.168.128.0/24 agentrun-internal` returns `network agentrun-internal already exists`. `EnsureInternalNetwork` lists networks and then creates one, with no handling for that error, so two runs that start on a machine without the network can race and one of them fails.
- **The runtime assigns subnets itself.** `container network create --internal probe-auto-net` received `192.168.129.0/24` with no `--subnet` flag. A fixed subnet is not needed.
- **The sidecar's address is read from inspect output.** `container inspect` lists each attachment under `status.networks[]`, with `network` and `ipv4Address` (for example `192.168.129.2/24`). `egress.ExtractInternalIP` ignores that structure and matches the regex `192.168.128.x` anywhere in the output, so it cannot find the address of a network on another subnet.
- **Run IDs and container names are already unique.** `engine.GenerateID` checks the runs directory, containers are named `agentrun-<run>-<nanoseconds>`, and the sidecar is named `agentrun-egress-<run>`.

## Options

1. **Keep the shared network.** Smallest change. It keeps the lateral reachability measured above, and it still needs the create race fixed.
2. **One internal network per run.** Each run creates `<network>-<run id>` with `--internal` and no subnet, attaches its sidecar and its agent and check containers to it, and deletes it when the run ends.

## Decision

Option 2. Runs are untrusted workloads, and the network is the only boundary between them. The shared network leaves an agent able to reach other runs' containers, which the egress allowlist does not cover. Per-run networks remove that path and the create race together, because each name is unique.

Changes this implies:

- `egress`: replace `EnsureInternalNetwork` with `CreateInternalNetwork(name)` (no subnet) and add `DeleteNetwork(name)`. Remove `InternalSubnet`.
- `egress.ExtractInternalIP` takes the network name and reads that network's address from `status.networks[]`, stripping the prefix length.
- `engine`: the network name is `EgressOptions.Network + "-" + run.ID`. `EgressOptions` gets `CreateNetwork` and `DeleteNetwork` hooks, in the same style as `StartSidecar`, so unit tests never touch the machine. Execute deletes the run's network after the sidecar stops.
- `submit --network` keeps its name and now names the prefix. The default stays `agentrun-internal`, so a run's network is `agentrun-internal-<run id>`.

## Consequences

- Each run needs one subnet from the runtime's pool. The pool size bounds how many runs can hold an egress network at once. This has not been measured.
- A process that is killed before Execute's cleanup runs leaves its network behind. `container network list` shows it. No sweep command exists yet.
- Network creation and deletion add two runtime calls per run.

## Test plan

- Unit tests (fakes): network name per run, create and delete ordering, cleanup on sidecar failure, and the existing egress failure paths.
- `ExtractInternalIP` tests use the real inspect shape, including a network that is not 192.168.128.x.
- Capability test: two concurrent `submit` runs with egress, each reaches its own allowlisted host, neither can reach the other's containers, and no `agentrun-internal-*` network is left after both finish.
- Host sleep and real agent prompts stay in the manual list in `docs/decisions/step5-gaps.md`.

# Feature Contract: Container Isolation

Describes how a run's container is specified, validated, and launched on Apple's `container` CLI, so that each agent runs as a non-root process with a read-only root filesystem and an explicit tmpfs `/tmp`.

---

## Sub-features

### `container-run-args`
- **Intent**: Turn a validated `container.Spec` into `container run` arguments.
- **Behavior**: `RunArgs` emits user, read-only root, tmpfs, mounts, network(s), `--name`, sorted `--label k=v`, `--memory`, and `--env` from the spec.
- **Outcome**: The same spec always yields the same argument list.

### `container-spec-validation`
- **Intent**: Reject specs that would produce ambiguous or unsafe container arguments.
- **Behavior**: Names, labels, mount paths, and environment keys containing `,` or `=` are rejected with the sentinel errors declared in `container.go`.
- **Outcome**: Invalid specs fail before any container is created.

### `container-apple-backend`
- **Intent**: Run a container attached and return its exit code.
- **Behavior**: `internal/container/apple` implements `container.Backend` by running `container run` and streaming output to an `io.Writer`.
- **Outcome**: The supervisor records the exit code while the container is attached.

---

## How to get to it (user POV)

- **Build args**: `container.RunArgs(spec)` (see `internal/container/args.go`)
- **Validate**: `spec.Validate()` (see `internal/container/validate.go`)
- **Backend**: `(*apple.Apple).Run(ctx, spec)` in `internal/container/apple`
- **Fake backend for tests**: `internal/container/containertest`

---

## Verification Suites (Targeted Test Commands)

- **Unit Command**: `go test -race -count=1 -cover ./internal/container/...`
- **Live Capability Suite (needs Apple `container` running)**: `go test -tags capability -count=1 -v -timeout 10m ./test/capability/`
- **Allowlisted Command**: `sh .verify/run.sh all`

---

## Code

- `internal/container/`
- `test/capability/`

---

## Gotchas

- Containers run as root with a writable root filesystem unless the spec sets user, `--read-only`, and tmpfs `/tmp` explicitly.
- Apple `container` does not keep a detached container's exit code, so the exit code must be read while attached.
- The capability suite is excluded from the default test run by the `capability` build tag. Its containers are named `spike-` and `spike-net-`; check for leftovers with `container list --all` if a run is interrupted.

# Feature Contract: Egress Allowlist Proxy

A CONNECT-only HTTP proxy that lets a run reach only the host names on its allowlist. A sidecar starts the proxy beside a run's container and reports the address the container should use.

---

## Sub-features

### `egress-connect-allowlist`
- **Intent**: Allow outbound TLS tunnels only to allowed host names.
- **Behavior**: CONNECT requests whose host:port is on the allowlist are tunneled; anything else gets 403 (IP literals are not special-cased); a malformed host:port gets 400; non-CONNECT methods get 405.
- **Outcome**: A container with no direct network route can reach only the approved hosts.

### `egress-sidecar`
- **Intent**: Run the proxy for one run and expose its address.
- **Behavior**: `StartSidecar(ctx, SidecarConfig)` starts the proxy; `Stop(ctx)` shuts it down; `ExtractInternalIP` reads the container's internal address from inspect output.

---

## How to get to it (user POV)

- **Proxy**: `egress.New(allowed, opts...)` in `internal/egress/proxy.go`, returns an `http.Handler`
- **Sidecar**: `egress.StartSidecar(ctx, egress.SidecarConfig{...})`
- **Command**: `cmd/agentrun-egress`

---

## Verification Suites (Targeted Test Commands)

- **Unit Command**: `go test -race -count=1 -cover ./internal/egress/... ./cmd/agentrun-egress/...`
- **Allowlisted Command**: `sh .verify/run.sh all`

---

## Code

- `internal/egress/`
- `cmd/agentrun-egress/`

---

## Gotchas

- Per-container network allowlisting is not available in Apple `container`, so the proxy is the enforcement point for `model-api` mode.
- The proxy tests use a local TCP echo server and `localhost`; they do not reach the network.

# Knowledge base: Egress allowlist proxy

- Feature map: `.verify/features/egress-proxy.md`
- Proxy: `internal/egress/proxy.go` (`New(allowed, opts...)` returns an `http.Handler`; CONNECT-only; 405 for other methods; 400 for a malformed host:port; 403 for any host:port not on the allowlist, IP literals included, since they are not special-cased)
- Sidecar: `internal/egress/sidecar.go` (`StartSidecar`, `Stop`, `ExtractInternalIP`)
- Command: `cmd/agentrun-egress/`
- Apple `container` has no per-container network allowlist, so this proxy is the enforcement point for `model-api` mode

# Knowledge base: Container isolation

- Feature map: `.verify/features/container-isolation.md`
- Spec and validation: `internal/container/container.go` (`Spec`, sentinel errors), `validate.go` (`Spec.Validate`)
- Argument builder: `internal/container/args.go` (`RunArgs` emits `--user`, `--read-only`, `--tmpfs`, `--network`, `--name`, `--label`, `--memory`, `--env`)
- Backend: `internal/container/apple/apple.go` (`(*Apple).Run`), tested against `containertest` fakes
- Live checks: `test/capability/` behind the `capability` build tag, run by hand; the default `go test ./...` does not run them
- Name, label, mount, and env values containing `,` or `=` are rejected before arguments are built

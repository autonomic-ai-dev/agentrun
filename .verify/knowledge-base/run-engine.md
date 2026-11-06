# Knowledge base: Run engine

- Feature map: `.verify/features/run-engine.md`
- Types and statuses: `internal/engine/types.go` (`queued`, `running`, `succeeded`, `failed`, `cancelled`)
- Store: `internal/engine/store.go` (`CreateRun`, `GetRun`, `ListRuns`, `SetStatus`)
- Pipeline: `internal/engine/pipeline.go` (`Execute` runs `verify.Loop`, saves `outcome.json`, commits with hooks disabled, diffs against the base)
- Cancel and result: `internal/engine/cancel.go`, `internal/engine/result.go`
- CLI entry: `cmd/agentrun/main.go` (`submit` calls `engine.CreateRun` and `engine.Execute`; `status`, `result`, `logs`, `cancel` call `engine.GetRun`, `engine.GetResult`, `engine.Cancel`)
- Agent adapter: `internal/agent/adapter.go` (`CommandFor` passes the prompt as an argument, never through a shell)
- Engine is uncommitted work in progress; `README.md` still says the engine is not built

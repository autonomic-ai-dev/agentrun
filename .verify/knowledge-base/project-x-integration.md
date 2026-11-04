# Knowledge base: Project X integration

- Feature map: `.verify/features/project-x-integration.md`
- Runner (validate, inspect receipts): `internal/projectx/runner.go` (`NewRunner`, `ValidatePlan`, `ValidatePlanBytes`, `InspectReceipt`, `InspectReceiptCLI`)
- In-container MCP config: `internal/projectx/mcp.go` (`DefaultMCPConfig`, `GenerateMCPConfigJSON`, `WriteAgentMCPConfig`)
- Provenance digest: `internal/record/` (`WithPlanDigest`, 64 lowercase hex, stored as `sha256:`-prefixed)
- Evidence output: `cmd/agentrun-dev/evidence.go`
- Integration test: `test/integration/projectx_test.go` (uses `verify.SaveOutcome`)
- Tests need a `project-x` binary; `findProjectXBin` in `internal/projectx/runner_test.go` resolves it

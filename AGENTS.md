# AGENTS.md

Instructions for coding agents working in this repository.

## Write code with agentrun

Make code changes through an agentrun run, not by editing the working tree directly. The prototype is `prototype/agentrun-proto`. A run starts from committed HEAD, works in its own worktree, and leaves its result on a branch `agentrun/<id>`. Nothing lands in your checkout until a person reviews and merges that branch.

Submit a task:

```sh
AGENTRUN_PROTO_DATA=<scratch>/proto-data \
AGENTRUN_AGENT_CMD='claude -p --permission-mode acceptEdits' \
prototype/agentrun-proto submit "<task>" --repo .
```

- `<scratch>` must be outside this repository. Set `AGENTRUN_PROTO_DATA` explicitly every time.
- `submit` refuses a repository with uncommitted changes unless `--include-dirty` is given. With that flag the run still starts from HEAD, so uncommitted edits are not part of the run.
- Commit the change you depend on before submitting. Do not rely on a run to pick up uncommitted work.

Follow the run:

```sh
prototype/agentrun-proto list
prototype/agentrun-proto logs <id>
prototype/agentrun-proto result <id>
```

Review the branch before merging, using the checklist in `docs/verification.md` section 2:

1. The result is `succeeded`.
2. `git diff --stat <base> agentrun/<id>` touches only the files the task named.
3. No new comments were added.
4. The checks pass in the run's worktree.
5. New regression tests fail on the previous commit.

Do not merge a run branch that has not been reviewed. Do not start the next backlog item until the previous one is merged, as `docs/self-build-backlog.md` requires.

Only make a direct edit in the working tree when a person asks for one, and say so in your report.

## Rules for code in this repository

- No new comments. Do not add comments, docstrings, TODOs, or explanatory annotations to code.
- `gofmt -l .` prints nothing and `go vet ./...` passes.
- `go test -race -count=1 ./...` passes before a push.
- Changes to `internal/container/` or `test/capability/` also run the capability suite, as described in `docs/verification.md`.
- Runtime output goes under `.verify/artifacts/`. Fixtures under `verify/` are read-only.
- Record a verification with `agent-brain verify -- <cmd>` before you report that checks pass.

## Where things are

- `README.md`: product status and the commands that exist today.
- `docs/self-build-backlog.md`: the ordered backlog of self-build items.
- `docs/verification.md`: the verification steps for a push, an agent run, and a remote check.
- `docs/features/`: one page per feature that exists.
- `.verify/features/` and `.verify/knowledge-base/`: feature contracts and their notes, checked by the stop gate.

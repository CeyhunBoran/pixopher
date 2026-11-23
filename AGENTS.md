# Working on pixopher

pixopher is a pixel art CLI in Go, built for AI agents to draw with while
humans watch live.

## Start here

- `docs/` is the source of truth. Read `docs/README.md`, then
  `docs/vision.md` and `docs/architecture.md`.
- Work milestone by milestone from `docs/roadmap.md`. A milestone's
  acceptance criteria are its definition of done. Don't build features from
  later milestones early.
- Unclear? Add it to `docs/open-questions.md`. If you must decide something
  to make progress, record it in `docs/decisions.md`.
- Change the docs in the same commit as the code they describe, and tick
  roadmap boxes when items land.

## Rules

- Go 1.27 (toolchain 1.27.4), pure Go, **no CGO**. Keep the package dependency direction from
  `docs/architecture.md`.
- Every drawing/editing command is an op in the registry (`internal/ops`). The
  CLI, batch scripts, MCP, schema and guide are generated from it. Never
  hand-write a cobra command for something that should be an op.
- Every mutating op needs a golden test and an apply-then-undo identity test.
- Error codes and `--json` shapes are public API (see `docs/cli.md`).
- Output must be deterministic (same input gives the same bytes on every OS).

## Commands

```sh
go build ./cmd/pixopher
go test ./...                 # add -update to refresh golden files
go test -race ./...
go vet ./... && golangci-lint run
```

## Commits

- One-line Conventional Commits, lowercase: `feat:`, `fix:`, `docs:`,
  `refactor:`, `test:`, `chore:`. No body unless essential.
- No AI attribution trailers or tool/vendor names in commits, code or docs.
- Don't reference unrelated private projects in code, docs, fixtures or
  commit messages. Test fixtures are drawn from scratch.

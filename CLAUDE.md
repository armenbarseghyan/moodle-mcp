# moodle-mcp — working rules

Read-only Moodle MCP server in Go, plus Claude skills under `skills/`. High bar: small, obvious, tested.

## Commands

- `make test` — `go test -race -count=1 ./...`; prefer a narrow `go test -race -run '^TestX$' ./internal/<pkg>/` while iterating.
- `make lint` — `go vet` + golangci-lint (pinned).
- `make golden` — regenerate tool golden files; only for an intended output change, and review the diff.
- `make dev` — fake Moodle + `mcpcall` for trying tools and skills without the real site.

## Invariants

- **Read-only.** No tool may write to Moodle.
- **Control flow never reads prose.** Branch on Moodle `errorcode`, typed/sentinel errors (`internal/moodle/errors.go`), IDs, enums and status codes — never on `message`/`exception` text. Regex is for syntax (paths, URLs, IDs), never for intent.
- **Dependencies are justified.** Every new module in `go.mod` needs a reason the standard library or existing deps cannot cover.
- **Tests guard behaviour, not implementation.** Pure refactor → no existing test changes. Feature/fix → tests added. Never edit a test or golden file just to make the diff green.

## How to work

- Unfamiliar area → `codebase-orientation`. Bug → `diagnose` (reproduce first, hypothesis → probe → minimal fix + regression test). Unclear plan → `grill-me`. Before commit → `validation-review` with the `reviewer` / `logic-hunter` / `anti-slop` / `go-dev` agents.
- Long multi-step work: save a chain link (`.chains/`) at milestones.

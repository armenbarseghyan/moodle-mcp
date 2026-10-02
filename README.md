# moodle-mcp

A read-only MCP server for Moodle (FH JOANNEUM, Moodle 4.5), written in Go. It answers study
questions — what is due, what teachers posted, where a piece of material is — by combining
several Moodle Web Service calls behind each tool. It cannot change anything in Moodle: the
client refuses every function outside an allowlist of 9 getters.

Architecture and design decisions: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).
Setting it up as a student (token, build, Claude Code / Claude Desktop): [docs/STUDENT-SETUP.md](docs/STUDENT-SETUP.md).

## Install

Requires Go 1.25+ (with `GOTOOLCHAIN=auto` the right version is downloaded automatically).

```bash
make build        # → bin/moodle-mcp
```

## Environment

| Variable | Required | Default | Meaning |
|---|---|---|---|
| `MOODLE_URL` | yes | — | site URL, `https://moodle.fh-joanneum.at` |
| `MOODLE_TOKEN` | yes | — | personal token of the `moodle_mobile_app` service |
| `MOODLE_DOWNLOAD_DIR` | no | `~/Downloads/moodle` | where `moodle_download` and dashboards put files |
| `MOODLE_LOG_LEVEL` | no | `info` | `debug`, `info`, `warn`, `error` |
| `MCP_HTTP_ADDR` | no | — | same as `-http`: serve streamable HTTP instead of stdio |
| `MCP_HTTP_TOKEN` | for non-loopback addresses | — | bearer token HTTP clients must send |

The token is read from the environment only. Logs go to stderr; the token is masked
(`[REDACTED]`) in logs, errors and tool output. If the variables are missing the server still
starts and every tool explains what is missing.

Token: Moodle → Profile → Preferences → Security keys, service *Moodle mobile web service*
(details and alternatives in the [setup guide](docs/STUDENT-SETUP.md)).

## Connect to Claude Code

```bash
claude mcp add moodle --scope user \
  -e MOODLE_URL=https://moodle.fh-joanneum.at \
  -e MOODLE_TOKEN=<your token> \
  -- /absolute/path/to/moodle-mcp/bin/moodle-mcp
```

Check: `moodle_whoami` shows the user, the site version and any missing functions.

## HTTP instead of stdio

```bash
MCP_HTTP_TOKEN=<secret> ./bin/moodle-mcp -http 127.0.0.1:8765
```

The endpoint is `http://127.0.0.1:8765/mcp` (streamable HTTP); `GET /healthz` is a liveness
check. Protection: DNS rebinding (requests to localhost with a foreign `Host` are rejected),
browser cross-origin requests are rejected, and with `MCP_HTTP_TOKEN` every request needs
`Authorization: Bearer …`. The server refuses to listen on a non-loopback address (`0.0.0.0`,
LAN) without a token, because it holds your Moodle token.

```bash
claude mcp add moodle-http --transport http http://127.0.0.1:8765/mcp --header "Authorization: Bearer <secret>"
```

## Tools

| Tool | Parameters | What it does |
|---|---|---|
| `moodle_deadlines` | `days=14`, `include_overdue=true`, `refresh` | Deadlines until the end of day N (Vienna): calendar + assignment due dates, deduplicated, submission status for every assignment, unsubmitted ones overdue by up to 7 days |
| `moodle_announcements` | `days=7`, `refresh` | Recent posts in the announcement forums of all courses |
| `moodle_courses` | `include_past`, `refresh` | Active courses: id, name, dates, progress |
| `moodle_course_contents` | `course`, `refresh` | A course's material by section (incl. Moodle 4.5 subsections) with file links. `course` is an id or part of the name; ambiguous queries return candidates |
| `moodle_search` | `query`, `course=""`, `in_files=false`, `refresh` | Search names, descriptions, sections and file names in all active courses (or one). With `in_files=true` also the text of PDF, docx, pptx, ipynb, zip and Moodle pages, with page numbers and a quote |
| `moodle_grades` | `course=""` | Grades: score, maximum, percentage, feedback; one course or all |
| `moodle_download` | `fileurl`, `dest=""` | Downloads a file of this Moodle and returns the local path |
| `moodle_whoami` | — | Diagnostics: user, site, version, available functions |

Output is compact markdown. Dates look like `2026-10-07 17:15 (Wednesday) — in 5 days`,
Europe/Vienna. File links are browser links (`…/pluginfile.php/…`): they open where you are
signed in to Moodle and never contain the token. Output is English; all user-facing strings go
through a `golang.org/x/text/message` catalog, so other languages can be added later.

In-memory cache: courses, course contents and forums 15 min; deadlines, submission status and
announcements 5 min; grades are never cached; file text for `in_files` until the file changes
(URL + size + modification time). `refresh=true` bypasses the cache.

Search inside files: the first call downloads the course files into memory (18 files, ~4 s for
the real 8 courses), later calls use the cache. Files over 40 MB, videos and images are
skipped. PDFs are read in pure Go (a vendored, patched `ledongthuc/pdf`, see
`third_party/ledongthuc-pdf/PATCHES.md`); spaces are rebuilt from glyph positions, and words of
6+ letters also match ignoring spaces, since PDFs often lose or invent them.

## Skills

Seven Claude Code skills — briefing, finding material and answering from it, working through an
assignment, grades, a visual dashboard, course profiles from the syllabi, and diagnostics — live
in [skills/](skills/README.md):

```bash
make install-skills
```

## Development

```bash
make test      # go test -race ./...
make cover     # coverage
make lint      # go vet + golangci-lint v2 (via go run, nothing to install)
make golden    # rewrite the expected tool output (testdata/golden)
make dev       # bin/fakemoodle (fake Moodle) and bin/mcpcall (call a tool from the shell)
```

Skill evals: `dev/skill-evals/` holds the prompts, the grader and how to run them.

Tests never touch the network: a fake Moodle (`internal/moodletest`) serves the fixtures
`testdata/moodle/<wsfunction>.<scenario>.json` — real responses (anonymised, `*.real*`) and
synthetic ones built from the Moodle 4.5 schemas (`*.synthetic*`).

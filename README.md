# moodle-mcp

[![CI](https://github.com/armenbarseghyan/moodle-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/armenbarseghyan/moodle-mcp/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A read-only MCP server for Moodle (FH JOANNEUM, Moodle 4.5), written in Go. It answers study
questions — what is due, what teachers posted, where a piece of material is — by combining
several Moodle Web Service calls behind each tool. It cannot change anything in Moodle: the
client refuses every function outside an allowlist of 10 getters.

## Quick start

```bash
git clone https://github.com/armenbarseghyan/moodle-mcp.git
cd moodle-mcp && ./setup.sh                              # macOS / Linux, any shell
powershell -ExecutionPolicy Bypass -File .\setup.ps1     # Windows
```

The setup builds the server (or downloads a verified release binary when Go is missing), asks
for your FH login once, gets and checks the token, registers the server with Claude Code and/or Codex (whichever is installed) and
links the skills. Step-by-step guide, the token pitfalls and troubleshooting:
**[docs/STUDENT-SETUP.md](docs/STUDENT-SETUP.md)**. Architecture and design decisions:
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Build

Requires Go 1.26 (any Go 1.21+ downloads the right toolchain automatically).

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

Token: `./setup.sh` gets it from `login/token.php` (service `moodle_mobile_app`). The
*Security keys* page stays empty for students, which is expected. See the
[setup guide](docs/STUDENT-SETUP.md#about-the-token--four-pitfalls).

## Connect to Claude Code or Codex

`setup.sh` does this; by hand (the path must be absolute). Codex: the same with
`codex mcp add moodle --env … --env … -- <path>`.

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
| `moodle_deadlines` | `days=14`, `include_overdue=true`, `refresh` | Deadlines until the end of day N (Vienna): calendar + assignment due dates, deduplicated, submission status for every assignment and attempt status for every quiz, unsubmitted assignments overdue by up to 7 days |
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

Seven skills (Claude Code and Codex) — briefing, finding material and answering from it, working through an
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
make fuzz      # all fuzz targets, FUZZTIME=30s each (PDF/zip/HTML parsing, responses, links, search)
make dev       # bin/fakemoodle (fake Moodle) and bin/mcpcall (call a tool from the shell)
make setup-test  # setup.sh (and setup.ps1's login, if pwsh is installed) end to end against the fake Moodle
```

`make lint` also runs `shellcheck`. On Windows, `dev/setup_test.ps1` tests `setup.ps1`. CI runs all
of this on Linux, macOS and Windows. Pushing a `v*` tag publishes release binaries plus
`checksums.txt`; the setup scripts download them on machines without Go.

Skill evals: `dev/skill-evals/` holds the prompts, the grader and how to run them.

Tests never touch the network: a fake Moodle (`internal/moodletest`) serves the fixtures
`testdata/moodle/<wsfunction>.<scenario>.json` — real responses (anonymised, `*.real*`) and
synthetic ones built from the Moodle 4.5 schemas (`*.synthetic*`).

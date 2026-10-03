# moodle-mcp

[![CI](https://github.com/armenbarseghyan/moodle-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/armenbarseghyan/moodle-mcp/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/armenbarseghyan/moodle-mcp)](https://github.com/armenbarseghyan/moodle-mcp/releases/latest)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**Your FH JOANNEUM Moodle inside Claude Code or Codex.** Ask about deadlines, materials,
grades and announcements in plain language (any language), and get answers with links that
open in your browser.

> *"What's due this week?"* · *"How do I connect to the lab VM from home?"* ·
> *"What do I need to do for Homework R2?"* · *"How are my grades?"* ·
> *"Make a dashboard of my next two weeks"* · *"Was gibt es Neues in PDP?"*

It is **read-only**: it can read your courses, but it cannot submit, post or change
anything in Moodle. It runs on your own computer, with your own account.

---

## Install

**You need:** [Claude Code](https://claude.com/claude-code) or the
[Codex CLI](https://developers.openai.com/codex), [git](https://git-scm.com/downloads), and your
FH username and password. Go is *not* needed. Without Go, the setup downloads a ready-made,
checksum-verified program.

### macOS / Linux

Open a terminal and run:

```bash
git clone https://github.com/armenbarseghyan/moodle-mcp.git
```

```bash
cd moodle-mcp && ./setup.sh
```

### Windows

Open **PowerShell** and run:

```powershell
git clone https://github.com/armenbarseghyan/moodle-mcp.git
cd moodle-mcp
powershell -ExecutionPolicy Bypass -File .\setup.ps1
```

### Then

1. The script asks for your **FH username and password once**. They go only to
   `moodle.fh-joanneum.at` and are not saved anywhere.
2. Answer **Y** when it offers to install the skills (recommended).
3. When it prints `Done`, **restart Claude Code / Codex** and ask:
   *"check my moodle connection"*.

The script connects every client it finds. Use `--claude` or `--codex` (`-Claude` / `-Codex`
on Windows) to pick one. Want the details of each step, the manual way, or help with an
error? → **[Setup guide](docs/STUDENT-SETUP.md)**.

### First thing to try

Ask *"set up my course profiles from the syllabi"* once per semester. The assistant reads every
course's syllabus and saves how it is graded (weights, must-pass parts, attendance), so later
answers about grades and priorities follow the real rules.

## Update

```bash
cd moodle-mcp && git pull && ./setup.sh
```

On Windows run `git pull`, then `setup.ps1` again as above. It asks for your login again and
replaces the old registration.

## Uninstall

```bash
claude mcp remove moodle -s user     # Claude Code
codex mcp remove moodle              # Codex
```

Then delete the `moodle-mcp` folder and the `moodle-*` links in `~/.claude/skills` and
`~/.agents/skills`.

## If something doesn't work

| Problem | Fix |
|---|---|
| The assistant has no Moodle tools | restart Claude Code / Codex; still nothing → run the setup again |
| "token is invalid or revoked" | run the setup again |
| Moodle's *Security keys* page is empty | that's normal for students; the setup gets the token another way |
| `Failed to connect` / `CONNECTION_CLOSED` | see [Troubleshooting](docs/STUDENT-SETUP.md#troubleshooting) |
| `running scripts is disabled` (Windows) | start it exactly as shown: `powershell -ExecutionPolicy Bypass -File .\setup.ps1` |

Or just ask the assistant *"moodle doesn't work, help me"*. The `moodle-setup` skill walks
through the checks.

## What's inside

- **8 tools:** deadlines (assignments and quizzes, with submission status), announcements,
  courses, course contents, search (also *inside* PDFs, Word, PowerPoint, notebooks, with
  page numbers), grades with feedback, file download, and a self-check.
- **7 [skills](skills/README.md)** on top of them: briefing, finding and quoting material,
  working through an assignment, grades, a visual dashboard, course profiles from the
  syllabi, and setup diagnostics.

**Privacy.** The server talks only to `moodle.fh-joanneum.at`, and your token stays in your
client's config on your computer. It is masked in all logs and output. What you ask about is
sent to the assistant like anything else you type into a chat. See [SECURITY.md](SECURITY.md).

---

## Reference

Architecture and design decisions: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

### Environment

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

### Registering by hand

`setup.sh` does this; by hand (the path must be absolute). Codex: the same with
`codex mcp add moodle --env … --env … -- <path>`.

```bash
claude mcp add moodle --scope user \
  -e MOODLE_URL=https://moodle.fh-joanneum.at \
  -e MOODLE_TOKEN=<your token> \
  -- /absolute/path/to/moodle-mcp/bin/moodle-mcp
```

Check: `moodle_whoami` shows the user, the site version and any missing functions.

### HTTP instead of stdio

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

### Tools

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

### Development

Requires Go 1.26 (any Go 1.21+ downloads the right toolchain automatically).


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

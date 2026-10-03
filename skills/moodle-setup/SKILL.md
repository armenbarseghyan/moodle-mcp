---
name: moodle-setup
description: Diagnose and set up the Moodle MCP server (moodle-mcp) — token, connection, available functions, installing and connecting it to Claude Code, HTTP mode. Use this skill whenever moodle_* tools return an error ("invalid token", "MOODLE_TOKEN not set", "no connection", timeout), when the user says "moodle isn't working", "the moodle mcp won't connect", "how do I renew my token", "how do I connect moodle to claude", "Moodle geht nicht", or when no moodle_* tools are available in the session at all.
allowed-tools: mcp__moodle__moodle_whoami
---

# Diagnosing moodle-mcp

The server is a read-only Go binary (`moodle-mcp`) that reads `MOODLE_URL` and `MOODLE_TOKEN`
from its environment. Nearly every problem is the token, the network or the Claude Code
registration. Go from simple to complex and tell the user what each step showed.

Reply in the user's language.

## 1. Are the tools there?

If there are no `moodle_*` tools in the session, the server is not registered or failed to
start. Suggest the user runs (run it yourself only if you have a shell and they agree):

```bash
claude mcp get moodle
```

- **Not registered, or anything about the token** — the fix is the setup script in the
  repository folder. It builds the server, gets the token, checks it, and registers the
  absolute path. The user runs it in their own terminal because it asks for the FH password,
  which must never go into the chat:
  - macOS / Linux (any shell): `./setup.sh`
  - Windows: `powershell -ExecutionPolicy Bypass -File .\setup.ps1`

  No repository yet: `git clone https://github.com/armenbarseghyan/moodle-mcp.git`, then the
  same. Claude Code needs a restart afterwards.
- **`✘ Failed to connect` / `CONNECTION_CLOSED` with no details** — the binary crashed at
  startup. Have the user run it by hand with the same variables. The error is on stderr:
  `MOODLE_URL=https://moodle.fh-joanneum.at MOODLE_TOKEN=<token> <path>/bin/moodle-mcp`.
  If it starts and waits silently, the binary is fine and the registration is wrong: re-run
  the setup.
- **`ENOENT` / command not found** — registered with a relative path or bare name. Claude Code
  starts servers with a reduced `PATH`, so it must be absolute. Re-run the setup.

## 2. Token and site

Call `moodle_whoami`. It always goes to Moodle directly.

| Result | Meaning | What to do |
|---|---|---|
| name, login, all required functions present | everything works | the problem was transient or in the request |
| "invalid or revoked token" | token revoked, or a wrong value was registered (e.g. 64 chars) | the user re-runs `./setup.sh` / `setup.ps1` (see step 1) |
| "MOODLE_TOKEN / MOODLE_URL not set" | server started without env | re-run the setup |
| "access control" / functions missing | the token's service lacks functions | the token must belong to the `moodle_mobile_app` service; another service won't do |
| "maintenance" | Moodle is in maintenance mode | wait |
| "no connection", timeout | network, VPN or site down | open the site in a browser; off campus Moodle usually works without VPN |

## Getting a token

The token belongs to the **Moodle mobile web service** (`moodle_mobile_app`). The setup
script (step 1) gets it. Explain these points when the user does it by hand or is confused:

- **The "Security keys" page (`/user/managetoken.php`) is empty for FH students** and has no
  create button. Students lack `moodle/webservice:createtoken`. This is not a dead end: the
  token comes from `login/token.php` with `service=moodle_mobile_app`, using the FH username
  and password, just like the official Moodle app.
- **Use `token`, not `privatetoken`.** The answer is `{"token":"…","privatetoken":"…"}`. Only
  `token` (32 hex characters) is the API key; both together are 64 characters and give
  `invalidtoken`.
- **Shell differences** if they read the password themselves: zsh needs `read -rs "?Prompt " P`;
  bash's `read -rsp` fails in zsh with "no coprocess". Pasting several lines at once makes
  `read` swallow the next line as the password.
- **curl encoding:** `--data-urlencode password@-` (name before `@`). With `@-` alone, curl
  encodes the `=` too and Moodle answers `missingparam`.

Manual commands for every OS are in `docs/STUDENT-SETUP.md` → *Manual setup*. Point the user
there instead of improvising. Never ask for the password or the token in the chat.

## 3. Works, but data seems missing

- Deadlines missing although Moodle shows them — teachers may hide assignments until they open;
  the server reports "N assignments hidden". Not an error.
- Stale data — cache (courses 15 min, deadlines 5 min): call again with `refresh=true`.
- A file won't download — the link must point to the same Moodle (`…/pluginfile.php/…`); the
  server deliberately refuses other sites so the token never leaves.

## HTTP mode (on request)

```bash
MCP_HTTP_TOKEN=<secret> <path>/bin/moodle-mcp -http 127.0.0.1:8765
claude mcp add moodle-http --transport http http://127.0.0.1:8765/mcp --header "Authorization: Bearer <secret>"
```

Without `MCP_HTTP_TOKEN` the server only listens on localhost — by design.

## Security

Never print the token, never ask the user to paste it into the chat, never write it into
project files. If the user did send it, recommend issuing a new one.

---
name: moodle-setup
description: Diagnose and set up the Moodle MCP server (moodle-mcp) — token, connection, available functions, installing and connecting it to Claude Code, HTTP mode. Use this skill whenever moodle_* tools return an error ("invalid token", "MOODLE_TOKEN not set", "no connection", timeout), when the user says "moodle isn't working", "the moodle mcp won't connect", "how do I renew my token", "how do I connect moodle to claude", "мудл не работает", "как обновить токен", "Moodle geht nicht", or when no moodle_* tools are available in the session at all.
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

- `✘ Failed to connect` / `Connection closed` — check the binary path (`Command:`), that it is
  built (`make build` in the repository) and starts: `<path>/moodle-mcp -version`.
- No server — register it. The user pastes the token themselves; never ask them to send the
  token in the chat:

```bash
claude mcp add moodle -s user -e MOODLE_URL=https://moodle.fh-joanneum.at -e MOODLE_TOKEN=<token> -- <path>/bin/moodle-mcp
```

Claude Code needs a restart after registering.

## 2. Token and site

Call `moodle_whoami`. It always goes to Moodle directly.

| Result | Meaning | What to do |
|---|---|---|
| name, login, all required functions present | everything works | the problem was transient or in the request |
| "invalid or revoked token" | token reset or expired | new token (see "Getting a token" below), then `claude mcp remove moodle -s user` and `add` again with the new token |
| "MOODLE_TOKEN / MOODLE_URL not set" | server started without env | re-register with `-e …` |
| "access control" / functions missing | the token's service lacks functions | the token must belong to the `moodle_mobile_app` service; another service won't do |
| "maintenance" | Moodle is in maintenance mode | wait |
| "no connection", timeout | network, VPN or site down | open the site in a browser; off campus Moodle usually works without VPN |

## Getting a token

The token must belong to the **Moodle mobile web service** (`moodle_mobile_app`). At FH
JOANNEUM the app signs in with the FH username and password (no browser SSO), so there are
three ways, from most to least convenient:

1. **Security keys page.** Moodle in the browser → user menu → *Einstellungen / Preferences* →
   *Sicherheitsschlüssel / Security keys*. Copy the key of "Moodle mobile web service"; *Reset*
   issues a new one (and revokes the old one).
2. **No key listed?** Sign in once to the official **Moodle app** (site
   `https://moodle.fh-joanneum.at`). That creates the key; reload the Security keys page.
3. **From the terminal** (the official endpoint the app uses). The password is typed into the
   user's own terminal, never into the chat:

```bash
read -r -p "FH username: " U; read -r -s -p "FH password: " P; echo
curl -s https://moodle.fh-joanneum.at/login/token.php --data-urlencode "username=$U" --data-urlencode "password=$P" --data-urlencode "service=moodle_mobile_app"; unset P
```

   The answer is `{"token":"…","privatetoken":…}`; only `token` is needed. An `invalidlogin`
   error means wrong credentials; `enablewsdescription` means the service is disabled — then
   ask FH IT.

The full step-by-step guide for new users (also for classmates) is in the repository:
`docs/STUDENT-SETUP.md` (English) and `docs/STUDENT-SETUP.ru.md` (Russian).

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

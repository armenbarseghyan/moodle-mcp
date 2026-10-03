# moodle-mcp — setup guide for students

Connect your FH JOANNEUM Moodle to Claude Code in about five minutes. Afterwards you can ask,
in any language, things like *"what's due this week?"*, *"where is the guide for the lab VM?"*
or *"what did the teacher write about my homework?"*.

> **Read-only by design.** The server can only read your Moodle: courses, materials,
> deadlines, grades, announcements. It cannot submit, post, enrol or change anything.

**Contents:** [Quick setup](#quick-setup) · [What the setup does](#what-the-setup-does) ·
[About the token](#about-the-token--four-pitfalls) · [Manual setup](#manual-setup-fallback) ·
[Troubleshooting](#troubleshooting) · [Privacy](#privacy)

---

## Quick setup

You need:

| | |
|---|---|
| **Claude Code** | CLI, desktop app or IDE extension — [claude.com/claude-code](https://claude.com/claude-code) |
| **git** | to get the code (or download the ZIP from GitHub) |
| **Go** | optional (1.21 or newer; it fetches the 1.26 toolchain itself). With Go the server is built from source; without it the setup downloads a prebuilt, checksum-verified binary from the GitHub release |
| **FH account** | your own FH username and password — every student uses **their own** token |

### macOS and Linux

```bash
git clone https://github.com/armenbarseghyan/moodle-mcp.git
```

```bash
cd moodle-mcp && ./setup.sh
```

This works the same from zsh (the macOS default), bash, fish or any other shell: `setup.sh` runs
under bash itself, so nothing depends on the shell you type in.

### Windows (PowerShell)

```powershell
git clone https://github.com/armenbarseghyan/moodle-mcp.git
cd moodle-mcp
powershell -ExecutionPolicy Bypass -File .\setup.ps1
```

`-ExecutionPolicy Bypass` applies to this one run only; it does not change your system settings.
Works in Windows PowerShell 5.1 (built in) and PowerShell 7.

### Then

The script asks for your FH username and password **once**, checks everything and registers the
server. When it prints `Done`, **restart Claude Code** and ask *"check my moodle connection"*.

| Option | setup.sh | setup.ps1 |
|---|---|---|
| already have a token | `MOODLE_TOKEN=<token> ./setup.sh` | `$env:MOODLE_TOKEN = "<token>"` before the run |
| skip the skills | `--no-skills` | `-NoSkills` |
| install skills without asking | `--skills` | `-Skills` |
| another Moodle site | `--url https://…` | `-Url https://…` |

Running the script again is safe: it rebuilds the server and replaces the old registration.
After `git pull`, run it again to get the new version.

---

## What the setup does

1. **Checks** that `claude` (and on macOS/Linux `curl`) is installed.
2. **Builds** `bin/moodle-mcp` with Go — or, without Go, downloads the release binary for
   your system and verifies its SHA-256 checksum before using it.
3. **Gets your token** from `https://moodle.fh-joanneum.at/login/token.php`, the endpoint the
   official Moodle app signs in with. The password is read hidden, sent only there (through
   stdin, so it never appears in the process list) and not stored anywhere.
4. **Validates the token**: it must be exactly 32 hex characters (see pitfall 4 below).
5. **Checks it against Moodle** (`core_webservice_get_site_info`) and prints
   `signed in as <your name>`. A bad token stops here, before anything is registered.
6. **Registers** the server with Claude Code for your user
   (`claude mcp add moodle --scope user -e MOODLE_URL=… -e MOODLE_TOKEN=… -- <absolute path>`)
   and checks that it connects.
7. **Links the skills** into `~/.claude/skills` (asks first). Skills teach Claude good
   workflows on top of the tools: briefings, finding material and answering from it with page
   numbers, assignment checklists, grades with feedback, a visual dashboard and self-diagnosis.
   They are written in English and work in any language you write in.

The token ends up only in Claude Code's own config (`~/.claude.json`), never in the repository.

---

## About the token — four pitfalls

The token is a key that lets the server read Moodle **as you**. It must belong to the
**Moodle mobile web service** (`moodle_mobile_app`). The setup script handles all of the
following; they matter if you do it by hand or wonder why the script works the way it does.

### 1. The "Security keys" page is empty — that is not a dead end

Moodle has a page *Preferences → Security keys* (`/user/managetoken.php`) that lists tokens.
For FH students it stays **empty**, and there is no button to create one: students don't have
the permission to create tokens there (`moodle/webservice:createtoken`). That is expected.
The token comes from `login/token.php` with `service=moodle_mobile_app` — the same way the
official Moodle app gets one — using your normal FH username and password.

### 2. Reading a password in the terminal differs between shells

- bash: `read -rsp "Password: " P`
- zsh (macOS default): `read -rs "?Password: " P` — bash's `-p` means something else in zsh and
  fails with `read: -p: no coprocess`.
- Pasting several lines at once: the second line is swallowed as the answer to the next
  `read`, e.g. your next command becomes the "password". Type or paste one line at a time.

The manual snippet below avoids all of this with plain `printf`/`stty`, which behave the same
in sh, bash and zsh.

### 3. curl must URL-encode the password correctly

Passwords with `&`, `=`, `+`, spaces or umlauts break a plain `-d "password=$P"`. Use
`--data-urlencode`, and when the password comes from stdin, write the field name before the
`@`: `--data-urlencode password@-`. Without the name (`--data-urlencode @-`) curl encodes the
whole input including the `=`, Moodle sees no `password` field and answers `missingparam`.

### 4. Use `token`, not `privatetoken`

The answer looks like `{"token":"0123…cdef","privatetoken":"…"}`. Only **`token`** — 32 hex
characters — is the API key. `privatetoken` is something else (the app uses it to open
browser sessions). Copying both together gives 64 characters and Moodle answers
`invalidtoken`.

### Keep it secret

- The token works like your password for Moodle — **don't share it**, don't post it in group
  chats, don't paste it into a Claude chat, don't commit it to git. moodle-mcp only reads, but
  the token itself is not limited to reading.
- Running the setup again gives you a working token again; Moodle usually hands out the same
  one while it is valid.
- Shared it by accident? Change your FH password and ask the FH helpdesk
  (helpdesk@fh-joanneum.at) to revoke your mobile-app tokens — students cannot delete tokens
  themselves (pitfall 1).

---

## Manual setup (fallback)

Only if the script can't be used. Same steps, by hand.

### A. Get the token

macOS / Linux — works in sh, bash and zsh (fish: type `bash` first):

```bash
printf 'FH username: '; read -r U; printf 'FH password: '; stty -echo; read -r P; stty echo; printf '\n'
```

```bash
printf '%s' "$P" | curl -sS https://moodle.fh-joanneum.at/login/token.php --data-urlencode "username=$U" --data-urlencode "password@-" --data-urlencode "service=moodle_mobile_app"; unset P; echo
```

Windows (PowerShell) — prints only the token:

```powershell
$u = Read-Host "FH username"; $p = Read-Host "FH password" -AsSecureString
$r = Invoke-RestMethod -Method Post -Uri https://moodle.fh-joanneum.at/login/token.php -Body @{ username = $u; password = (New-Object Net.NetworkCredential('', $p)).Password; service = "moodle_mobile_app" }
if ($r.token) { $r.token } else { $r }
```

Take the value of **token** (32 hex characters). Errors:

| Error | Meaning |
|---|---|
| `invalidlogin` | wrong username or password |
| `missingparam` | the password field didn't arrive — see pitfall 3 |
| `enablewsdescription` / `servicenotavailable` | the mobile service is disabled for you — ask the FH helpdesk |

### B. Build the server

```bash
go build -o bin/moodle-mcp ./cmd/moodle-mcp
```

On Windows: `go build -o bin\moodle-mcp.exe .\cmd\moodle-mcp`. Without Go, download
`moodle-mcp-<os>-<arch>` from the [latest release](https://github.com/armenbarseghyan/moodle-mcp/releases/latest)
into `bin/` (on macOS/Linux: `chmod +x` it).

### C. Register it

Use the **absolute** path to the binary (see Troubleshooting, *ENOENT*):

```bash
claude mcp add moodle --scope user -e MOODLE_URL=https://moodle.fh-joanneum.at -e MOODLE_TOKEN=<token> -- /Users/you/moodle-mcp/bin/moodle-mcp
```

Check with `claude mcp get moodle` — it should say **Connected** — and restart Claude Code.
Skills: link (or copy) every folder of `skills/` into `~/.claude/skills/`; on macOS/Linux
`make install-skills` does that.

**Claude desktop app** instead of Claude Code: *Settings → Developer → Edit Config*
(`claude_desktop_config.json`), then restart the app:

```json
{
  "mcpServers": {
    "moodle": {
      "command": "/Users/you/moodle-mcp/bin/moodle-mcp",
      "env": { "MOODLE_URL": "https://moodle.fh-joanneum.at", "MOODLE_TOKEN": "<token>" }
    }
  }
}
```

### Optional settings

Add them as further `-e NAME=value` when registering.

| Variable | Default | Meaning |
|---|---|---|
| `MOODLE_DOWNLOAD_DIR` | `~/Downloads/moodle` | where downloaded files and dashboards go |
| `MOODLE_LOG_LEVEL` | `info` | `debug` for troubleshooting (logs never contain the token) |

---

## After the setup

- **Course profiles (once per semester):** ask *"set up my course profiles from the syllabi"*.
  Claude reads every course's syllabus and saves its rules (components and weights, what must
  be passed separately, attendance, exemption exams) to `~/.config/moodle-mcp/courses/`.
  Check the table it shows you and confirm. Answers about grades, assignments and priorities
  then use these rules.
- **Try:** *"what's due this week?"*, *"how do I connect to the lab VM from home?"*,
  *"what do I need to do for Homework R2?"*, *"how are my grades?"*,
  *"make a dashboard of my next two weeks"*. Links in answers open in your browser, where you
  are signed in to Moodle.

---

## Troubleshooting

### `CONNECTION_CLOSED` / "Failed to connect" with no details

The server crashed at startup and Claude Code doesn't show why. Run the binary by hand with the
same variables and read what it prints on stderr:

```bash
MOODLE_URL=https://moodle.fh-joanneum.at MOODLE_TOKEN=<token> ~/moodle-mcp/bin/moodle-mcp
```

```powershell
$env:MOODLE_URL = "https://moodle.fh-joanneum.at"; $env:MOODLE_TOKEN = "<token>"; & "$HOME\moodle-mcp\bin\moodle-mcp.exe"
```

If it starts fine it waits silently for input (stop it with Ctrl+C) — then the problem is the
registration (path, variables): run the setup again.

### `ENOENT` / "command not found" when Claude Code starts the server

Claude Code starts MCP servers with a reduced `PATH`, so a bare `moodle-mcp` or a relative
path is not found even if it works in your terminal. Register the **absolute** path — the setup
script always does.

### More

| Symptom | Fix |
|---|---|
| "Moodle token is invalid or revoked" / `invalidtoken` | run the setup again; if you pasted a token by hand, check it is 32 characters (pitfall 4) |
| `read: -p: no coprocess` | zsh — see pitfall 2, or just use `./setup.sh` |
| `missingparam` from `login/token.php` | see pitfall 3 |
| "MOODLE_TOKEN is not set" | the `-e MOODLE_TOKEN=…` part is missing — run the setup again |
| "function not allowed / access control" | the token belongs to another service — it must be `moodle_mobile_app` |
| `setup.ps1 cannot be loaded … running scripts is disabled` | start it with `powershell -ExecutionPolicy Bypass -File .\setup.ps1` |
| no deadlines although Moodle shows some | teachers may hide assignments until they open; the answer says "N hidden" |
| data seems outdated | ask again with "refresh" — courses are cached 15 min, deadlines 5 min |

Still stuck? Ask Claude *"moodle doesn't work, help me"* — the **moodle-setup** skill walks
through the checks.

---

## Privacy

- The server runs on **your** computer and talks only to `moodle.fh-joanneum.at`.
- What you ask about (deadlines, file contents, grades) is sent to Claude as part of the
  conversation, like anything else you paste into a chat.
- Downloaded files stay in `MOODLE_DOWNLOAD_DIR` on your machine.

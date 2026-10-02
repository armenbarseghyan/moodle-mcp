# moodle-mcp — setup guide for students

Connect your FH JOANNEUM Moodle to Claude in about 15 minutes. Afterwards you can ask, in any
language, things like *"what's due this week?"*, *"where is the guide for the lab VM?"* or
*"what did the teacher write about my homework?"*.

> **Read-only by design.** The server can only read your Moodle: courses, materials,
> deadlines, grades, announcements. It cannot submit, post, enrol or change anything.

---

## What you need

| | |
|---|---|
| Claude | **Claude Code** (CLI, desktop app or IDE extension) — recommended — or the **Claude desktop app** |
| Go | 1.25 or newer, to build the server once — [go.dev/dl](https://go.dev/dl/) |
| Moodle | your own FH JOANNEUM account (every student uses **their own** token) |

Check Go: `go version` should print `go1.25` or newer.

---

## Step 1 — Get the code and build it

```bash
git clone <repository URL you were given> moodle-mcp
cd moodle-mcp
make build            # creates bin/moodle-mcp
```

No `make` (e.g. on Windows)? Build directly:

```bash
go build -o bin/moodle-mcp ./cmd/moodle-mcp          # macOS / Linux
go build -o bin\moodle-mcp.exe .\cmd\moodle-mcp      # Windows (PowerShell)
```

Check it: `bin/moodle-mcp -version` prints the version.

Note the **absolute path** of the binary, you need it below, e.g.
`/Users/anna/moodle-mcp/bin/moodle-mcp` or `C:\Users\anna\moodle-mcp\bin\moodle-mcp.exe`.

---

## Step 2 — Get your Moodle token

The token is a key that lets the server read Moodle **as you**. It must belong to the
**Moodle mobile web service** (`moodle_mobile_app`). Use the first way that works:

### Way A — Security keys page (easiest)

1. Open <https://moodle.fh-joanneum.at> and sign in.
2. Click your avatar (top right) → **Einstellungen** (*Preferences*).
3. Under *Benutzerkonto* (*User account*) open **Sicherheitsschlüssel** (*Security keys*).
4. Find the row **Moodle mobile web service** and copy the **Schlüssel** (*Key*), a string of
   32 characters.

If the row exists but you don't see the key or it doesn't work, click **Zurücksetzen**
(*Reset*): Moodle issues a new key and the old one stops working.

### Way B — No "Moodle mobile web service" row?

The key is created the first time you use the official Moodle app.

1. Install the **Moodle** app (by Moodle Pty Ltd) on your phone.
2. Enter the site `https://moodle.fh-joanneum.at` and sign in with your FH username and password.
3. Reload the *Sicherheitsschlüssel* page in the browser — the row is there now. Continue with
   Way A, step 4.

### Way C — From the terminal

This calls the same official endpoint the Moodle app uses. FH JOANNEUM's app login uses your
FH username and password, so this works too. Your password is typed **only into your own
terminal**; it is not stored and not shown.

macOS / Linux:

```bash
read -r -p "FH username: " U; read -r -s -p "FH password: " P; echo
curl -s https://moodle.fh-joanneum.at/login/token.php \
  --data-urlencode "username=$U" --data-urlencode "password=$P" \
  --data-urlencode "service=moodle_mobile_app"; unset P
```

Windows (PowerShell):

```powershell
$u = Read-Host "FH username"; $p = Read-Host "FH password" -AsSecureString
$plain = [Runtime.InteropServices.Marshal]::PtrToStringAuto([Runtime.InteropServices.Marshal]::SecureStringToBSTR($p))
Invoke-RestMethod -Method Post -Uri https://moodle.fh-joanneum.at/login/token.php `
  -Body @{ username = $u; password = $plain; service = "moodle_mobile_app" }
Remove-Variable plain
```

The answer looks like `{"token":"0123…cdef","privatetoken":"…"}`. You need only **token**.

| Error | Meaning |
|---|---|
| `invalidlogin` | wrong username or password |
| `enablewsdescription` / `servicenotavailable` | the mobile service is disabled for you — ask FH IT |

### Keep the token secret

- The token works like your password for Moodle — **don't share it**, don't post it in group
  chats, don't paste it into a Claude chat, don't commit it to git.
- moodle-mcp only reads, but the token itself is not limited to reading.
- Lost it, shared it by accident, or want to switch the access off? Moodle → *Sicherheitsschlüssel*
  → **Zurücksetzen**. The old token stops working immediately.

---

## Step 3 — Connect it to Claude

### Claude Code (recommended)

Replace the two placeholders and run once:

```bash
claude mcp add moodle -s user \
  -e MOODLE_URL=https://moodle.fh-joanneum.at \
  -e MOODLE_TOKEN=<your token> \
  -- <absolute path>/bin/moodle-mcp
```

Windows: write the command on one line and use the path to `moodle-mcp.exe`.

Check: `claude mcp get moodle` should say **✔ Connected**. Restart Claude Code.

### Claude desktop app

Open *Settings → Developer → Edit Config* (file `claude_desktop_config.json`) and add:

```json
{
  "mcpServers": {
    "moodle": {
      "command": "<absolute path>/bin/moodle-mcp",
      "env": {
        "MOODLE_URL": "https://moodle.fh-joanneum.at",
        "MOODLE_TOKEN": "<your token>"
      }
    }
  }
}
```

Restart the app. The Moodle tools appear in the tools menu of a new chat.

### Optional settings

| Variable | Default | Meaning |
|---|---|---|
| `MOODLE_DOWNLOAD_DIR` | `~/Downloads/moodle` | where downloaded files and dashboards go |
| `MOODLE_LOG_LEVEL` | `info` | `debug` for troubleshooting (logs never contain the token) |

---

## Step 4 — Install the skills (Claude Code)

Skills teach Claude good workflows on top of the tools: briefings, finding material and
answering from it with page numbers, assignment checklists, grades with feedback, a visual
dashboard and self-diagnosis.

```bash
make install-skills          # links ./skills/* into ~/.claude/skills
```

Without `make`: copy (or link) every folder from `skills/` into `~/.claude/skills/`.
Restart Claude Code. The skills are written in English but work in any language you write in.

---

## Step 5 — Let Claude read your syllabi (once per semester)

Ask *"set up my course profiles from the syllabi"*. Claude reads every course's syllabus and
saves its rules (components and weights, what must be passed separately,
attendance, exemption exams) to `~/.config/moodle-mcp/courses/`. Check the overview table it shows you and
confirm. From then on answers about grades, assignments and priorities use these rules.

## Step 6 — Try it

Ask Claude:

- *"check my moodle connection"* → shows your name, the site and whether
  everything is available
- *"what's due this week?"*
- *"how do I connect to the lab VM from home?"*
- *"what do I need to do for Homework R2?"*
- *"how are my grades?"*
- *"make a dashboard of my next two weeks"*

Links in answers open in your browser where you are signed in to Moodle.

---

## Troubleshooting

| Symptom | Fix |
|---|---|
| `claude mcp get moodle` → *Failed to connect* | wrong path to the binary, or not built — run `make build`, check the path is absolute |
| "Moodle token is invalid or revoked" | get a new token (Step 2) and register again: `claude mcp remove moodle -s user`, then Step 3 |
| "MOODLE_TOKEN is not set" | the `-e MOODLE_TOKEN=…` part is missing — register again |
| "function not allowed / access control" | the token is from another service — it must be *Moodle mobile web service* |
| no deadlines although Moodle shows some | teachers may hide assignments until they open; the answer says "N hidden" |
| data seems outdated | ask again with "refresh" — courses are cached 15 min, deadlines 5 min |

Still stuck? Ask Claude *"moodle doesn't work, help me"* — the **moodle-setup** skill walks
through the checks.

---

## Privacy

- The server runs on **your** computer and talks only to `moodle.fh-joanneum.at`.
- What you ask about (deadlines, file contents, grades) is sent to Claude as part of the
  conversation, like anything else you paste into a chat. Don't use it for content you are not
  allowed to share with an AI assistant.
- Downloaded files stay in `MOODLE_DOWNLOAD_DIR` on your machine.

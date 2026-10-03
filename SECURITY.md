# Security

moodle-mcp holds a personal Moodle token, so its security properties are deliberate:

- **Read-only.** The client refuses every Moodle function outside an allowlist of getters
  (`internal/moodle/allowlist.go`); nothing that submits, posts, enrols or changes data is in
  the code.
- **The token stays local.** It is read only from `MOODLE_TOKEN`. It is redacted in logs,
  errors and tool output, and is never sent anywhere but the configured Moodle. File downloads
  are refused for URLs outside that Moodle, and file links in answers are browser links
  without the token.
- **HTTP mode** refuses non-loopback addresses without `MCP_HTTP_TOKEN` and rejects DNS
  rebinding and cross-origin browser requests.
- **The setup scripts** send the password only to `<MOODLE_URL>/login/token.php`, via stdin
  rather than the command line, and never store it. Release binaries are checked against
  `checksums.txt` before use.

## Reporting a vulnerability

Please report it privately through GitHub: *Security → Report a vulnerability* on this
repository. Don't open a public issue. Never include a real token in a report; if one
leaked, see "Keep it secret" in [docs/STUDENT-SETUP.md](docs/STUDENT-SETUP.md).

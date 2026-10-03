# Changelog

## [0.2.0] — 2026-10-03

### Added
- `moodle_whats_new`: everything that changed in the last N days in one call. It covers new or
  updated course files (grouped by course, with section and links), announcements and new
  grades. The briefing and dashboard skills use it.
- Dashboard:
  - the calendar has labels, a legend and short labels on phones;
  - new section "New material";
  - grades show teacher feedback and course totals;
  - the summary cards are clickable.
- Codex support: `setup.sh` / `setup.ps1` register with every installed client. The skills are
  linked to `~/.agents/skills` for Codex.

### Changed
- Deadlines:
  - two lines per item (what, then when and status);
  - "Overdue" and "Coming up" sections;
  - a neutral ⬜ before the deadline, ❌ only when overdue;
  - ⏰ for open work due within 48 h.
- Grades: Moodle's localised "87,50" is shown as "87.5" next to the maximum and the percentage.
- `setup.sh`: numbered, coloured steps and a short "what to ask first" summary.

### Fixed
- Error messages redact every field of a Moodle error object, not only the message.
- Error snippets cannot form the token through `%q` escapes. Both were found by fuzzing.

## [0.1.0] — 2026-10-03

First release.

- Read-only MCP server for Moodle 4.5 with 8 tools:
  - deadlines with submission and quiz status;
  - announcements;
  - courses and course contents;
  - search, including inside PDF, docx, pptx, ipynb and zip;
  - grades with feedback;
  - download;
  - diagnostics.
- Seven Claude Code skills, including a visual HTML dashboard.
- `setup.sh` (macOS/Linux, any shell) and `setup.ps1` (Windows) get the token from
  `login/token.php`, verify it and register the server. Without Go they download a
  checksum-verified release binary.
- CI on Linux, macOS and Windows; release binaries for six platforms.

[0.2.0]: https://github.com/armenbarseghyan/moodle-mcp/releases/tag/v0.2.0
[0.1.0]: https://github.com/armenbarseghyan/moodle-mcp/releases/tag/v0.1.0

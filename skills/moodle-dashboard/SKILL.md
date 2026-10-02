---
name: moodle-dashboard
description: Visual study dashboard from Moodle (FH JOANNEUM) — a polished HTML page with "urgent" cards, a day-by-day deadline timeline with submission status, teacher announcements and grades, light and dark theme. Use this skill whenever the user wants to see their studies visually, in any language — "make a dashboard", "show my week as a page", "visualise my deadlines", "study overview page", "Übersicht als Seite" — or asks for a briefing as a page rather than text. For a plain text "what's due" use moodle-briefing.
allowed-tools: mcp__moodle__moodle_announcements, mcp__moodle__moodle_deadlines, mcp__moodle__moodle_grades, mcp__moodle__moodle_courses, Bash(python3:*), Bash(open:*), Write, Read, Glob
---

# Visual dashboard

The page is built from a ready-made template (`assets/dashboard.html`) by `scripts/render.py`:
you collect the data as JSON, the script validates it and injects it into the page. Don't write
HTML by hand — the template is already designed, responsive (phone to desktop), with a dark
theme and print styles, so the page looks equally good every time.

## 1. Data

Call in parallel: `moodle_deadlines(days=14)`, `moodle_announcements(days=7)`,
`moodle_courses()`, and `moodle_grades()` (one call; include grades unless the user asked for
deadlines only). Change the period if asked ("this month" → `days=30`).

## 2. JSON

Write a file (e.g. in a temporary folder) of this shape. `lang` sets the page's labels
(`en`; further languages can be added in the template's `I18N` table). Write every text value
in English; keep quotes, course and file names in the original. Empty
sections are empty lists or absent; the template shows "nothing here" on its own.

```json
{
  "lang": "en",
  "title": "Studies · 5–16 October",
  "eyebrow": "Moodle · FH JOANNEUM",
  "student": "Max Student",
  "range": "deadlines until 16.10",
  "generated": "Sat 03.10, 09:12",
  "today": "2026-10-03",
  "urgent": [
    {"title": "Submit the Home Assignment Unix Shells draft", "course": "Refresher on Unix Shells and LaTeX",
     "when": "Tue 20.10, 08:00", "rel": "in 17 days", "url": "https://…/mod/assign/view.php?id=…",
     "note": "Uploaded but not submitted for grading", "level": "amber"}
  ],
  "deadlines": [
    {"date": "2026-10-05", "day": "Mon 05.10", "rel": "in 2 days", "time": "23:59", "title": "Quiz LaTeX Basics", "kind": "quiz",
     "course": "Refresher on Unix Shells and LaTeX", "status": "na", "url": "https://…"}
  ],
  "announcements": [
    {"date": "2026-09-30", "title": "Raumänderung morgen", "course": "Refresher on Unix Shells and LaTeX", "when": "Wed 30.09, 09:30",
     "rel": "3 days ago", "summary": "Tomorrow's lecture moves to another room; bring a laptop.",
     "quote": "Raum 0.12", "unread": true, "url": "https://…/mod/forum/discuss.php?d=…"}
  ],
  "grades": [{"item": "Homework R1", "course": "Programming and Data Processing", "grade": "87.50 / 100",
              "percent": 87.5, "url": "https://…"}],
  "courses": [{"name": "Programming and Data Processing", "url": "https://…/course/view.php?id=…"}],
  "plan": ["Today: check whether Homework R0 is still accepted", "By Mon 23:59: Quiz LaTeX Basics", "Thu–Fri: Homework R2 (due Sat 10:00)"],
  "notes": ["1 more assignment is hidden by the teacher."]
}
```

Filling it in well is what makes the page useful:
- `today` and each item's `date` (`YYYY-MM-DD`, Vienna) drive the calendar grid at the top: whole
  weeks around today and all deadlines, today highlighted, each deadline as a coloured chip.
- `deadlines` — chronological, grouped by an identical `day` value. `kind` in the user's
  language ("quiz"). `status`: `done`, `graded`,
  `draft` (uploaded, not submitted), `todo` (not submitted), `overdue`, `reopened`, `na`
  (quiz or not an assignment), `unknown`. Put overdue unsubmitted items here too, first, with
  `overdue`.
- `urgent` — same rules as the text briefing: overdue and unsubmitted; unsubmitted and due in
  < 48 h; drafts; class moves from announcements. Phrase each as an **action** ("Submit the
  draft…"). `level: "amber"` for drafts and "soon"; omit it for red.
- `announcements` — `summary` is one sentence in the user's language, `quote` the key fact in
  the original (room, time). `unread` when the tool marks a post unread.
- Links exactly as the tools return them (browser links, no token). The script rejects
  `/webservice/` and `token=`.
- `plan` — 2–4 short steps in order of urgency, each with a time anchor ("Today", "By Mon
  23:59"). Ground every step in the data; skip the plan if nothing is due.
- With course profiles (`~/.config/moodle-mcp/courses/`), add the weight to deadline titles
  where useful ("Homework R2 · 30 % pool") and order `urgent` by must-pass/weight first.
- Invent nothing: no data — empty list. "N assignments hidden" goes into `notes`.

## 3. Render and show

```bash
python3 <skill base dir>/scripts/render.py /tmp/moodle-dashboard.json
```

The script prints the HTML path (by default `$MOODLE_DOWNLOAD_DIR` or `~/Downloads/moodle`, file `dashboard-<date>.html`). If it
reports invalid data, fix the JSON and run it again.

Then show the page: open it (`open <path>` on macOS). If the session has an Artifact tool and
the user wants a link (to open on the phone or share), publish the same file as a private
artifact — the page already meets the usual requirements (title, colour tokens, dark theme,
mobile layout).

## 4. Chat reply

Short, in the user's language: the path or link, plus 2–3 lines of the essentials (urgent
items, number of deadlines, new announcements) so the user doesn't have to open the page if
that is enough.

---
name: moodle-course-profile
description: Course profiles built from the syllabus of each Moodle course (FH JOANNEUM) — how a course is graded (components, weights, what must be passed separately, pass rules, retakes), attendance, exemption exams, tools, topics. Use this skill to build or update the profiles ("read my syllabi", "set up my courses", "составь профили курсов"), and whenever the user asks about the rules of a course, in any language — "how is DQL graded", "what do I need to pass statistics", "how much is the homework worth", "сколько весит домашка", "что обязательно сдать", "wie wird der Kurs benotet". Other moodle skills read these profiles; build them first if they are missing.
allowed-tools: mcp__moodle__moodle_courses, mcp__moodle__moodle_search, mcp__moodle__moodle_course_contents, mcp__moodle__moodle_download, Read, Write, Bash(python3:*)
---

# Course profiles

Moodle says *what* is due and *when*; the syllabus says *by which rules*: weights, components
that must be passed on their own, attendance. A profile captures those rules once
per semester in a small JSON file, so every answer about grades, assignments and priorities can
use them.

Profiles live **on the user's machine**, not in any repository (they name lecturers and belong
to one programme): `$MOODLE_PROFILES_DIR`, default `~/.config/moodle-mcp/courses/`, one file per
course: `<programme>/<CODE>.json` (e.g. `DAT26/PDP.json`).

Reply in the user's language; keep component names and quotes in the original.

## Answering a question about a course

1. Find the profile: `ls ~/.config/moodle-mcp/courses/*/` and match the course by code, name
   or Moodle id. If there is none, build it (below) — it takes a minute per course.
2. Answer from the profile and quote the syllabus where it matters (the profile keeps short
   quotes). Link the syllabus (`source.url`).
3. If `verified` is false, add one line: "from the syllabus, not yet confirmed by you".
4. For "what do I need to pass": combine the profile with `moodle_grades` (points so far) —
   but only claim a computed number when the grade items clearly map to components; otherwise
   show the rules and the grades side by side.

## Building or updating profiles

1. `moodle_courses()` → the active courses (code is usually in the syllabus file name or the
   course name, e.g. `Syllabus_DAT26_PDP.pdf` → programme `DAT26`, code `PDP`).
2. For each course: `moodle_search("syllabus", course=<id>)` → `moodle_download(fileurl)` →
   **read the PDF with `Read`** (it renders the pages; the assessment table's ☐/☒ checkboxes
   decide `must_pass` and must be read visually, not guessed from text).
3. Fill the JSON (schema below). Rules that make profiles trustworthy:
   - Copy weights exactly; they must add up to 100 per attempt (the validator checks).
   - `must_pass` is true only where the syllabus ticks ☒ Yes.
   - Put extra conditions from "Additional comments" into `pass_rules` / `notes`
     (e.g. "≥ 61 % on the written exam", "homework points carry over to the next attempt").
   - Leave out what the syllabus does not say. Never fill gaps with typical values.
4. Validate and save:

```bash
python3 <skill base dir>/scripts/profile.py check /tmp/PDP.json        # fix and repeat until OK
python3 <skill base dir>/scripts/profile.py save  /tmp/PDP.json        # writes <dir>/DAT26/PDP.json
python3 <skill base dir>/scripts/profile.py table                       # overview of all profiles
```

5. Show the overview table and ask the user to check it; when they confirm, set
   `"verified": true` (`profile.py verify DAT26/PDP`).

## Schema

```json
{
  "schema": 1,
  "programme": "DAT26", "code": "PDP", "name": "Programming and Data Processing",
  "moodle_course_id": 12308, "semester": 1, "ects": 5, "continuous_assessment": true,
  "lecturers": ["…"],
  "source": {"file": "Syllabus_DAT26_PDP.pdf", "url": "https://…/pluginfile.php/…", "read_at": "2026-10-03"},
  "assessment": {
    "first_attempt": [
      {"name": "Written Assessment R", "weight": 17.5, "must_pass": true, "kind": "exam"},
      {"name": "Homework Assignments", "weight": 30, "must_pass": false, "kind": "homework"}
    ],
    "retake": [ … same shape … ],
    "pass_rules": ["…"],
    "notes": ["Points obtained from homework assignments are carried over to the next attempt."]
  },
  "attendance": {"min_percent": 75, "online_camera": true},
  "exemption_exam": {"available": false},
  "tools": ["R", "Python", "Git"],
  "topics": ["Programming paradigms", "…"],
  "structure": ["…planned units, if the syllabus lists them…"],
  "verified": false
}
```

`kind` is one of `exam`, `test`, `homework`, `project`, `participation`, `presentation`,
`self_assessment`, `oral`, `other`. The FH grading scale (91/81/71/61 %, rounding per DIN 1333)
is the default; add `"scale"` only if a syllabus differs.

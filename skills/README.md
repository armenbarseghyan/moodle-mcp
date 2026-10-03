# Skills for moodle-mcp

Skills for Claude Code and Codex that turn the moodle-mcp tools into good study workflows. Written and
tested in English.

| Skill | Use it for | Example prompts |
|---|---|---|
| [moodle-briefing](moodle-briefing/SKILL.md) | what is urgent, what's new (announcements, new files, grades), what is due — prioritised | "what's due this week?", "anything new since Monday?", "gibt es was Dringendes?" |
| [moodle-materials](moodle-materials/SKILL.md) | find material and answer from it, with page and quote | "how do I connect to the lab VM from home?" |
| [moodle-assignment](moodle-assignment/SKILL.md) | one assignment: deadline, status, task sheet, checklist, plan, earlier feedback | "what do I need to do for Homework R2?" |
| [moodle-grades](moodle-grades/SKILL.md) | grades, percentages, teacher feedback with translation | "how are my grades?" |
| [moodle-dashboard](moodle-dashboard/SKILL.md) | a visual HTML page: calendar, urgent cards, timeline, announcements, grades, plan | "make a dashboard of my next two weeks" |
| [moodle-course-profile](moodle-course-profile/SKILL.md) | each course's rules from its syllabus: components, weights, must-pass parts, attendance, exemption exams — used by the other skills | "how is DQL graded?" |
| [moodle-setup](moodle-setup/SKILL.md) | connection problems, getting and renewing a token, registration, HTTP mode | "moodle doesn't work" |

## Install

```bash
make install-skills      # symlinks every skill into ~/.claude/skills (setup.sh also does ~/.agents/skills for Codex)
make uninstall-skills    # removes those symlinks again
```

Symlinks mean `git pull` updates the skills without reinstalling. Restart Claude Code (or Codex) after
installing.

## Design rules shared by all skills

- **Keep quotes, course and file names in the original** (German Moodle texts are quoted, then explained).
- **Links exactly as the tools return them**: browser links into Moodle, never `/webservice/`
  or `token=`.
- **Invent nothing**: empty data is said plainly; general knowledge is labelled as such.
- **Read-only**: the skills point to where to click; the server cannot submit or post.
- **Actions over facts**: "submit the draft" rather than "status: draft".

## Course profiles

`moodle-course-profile` reads each syllabus once per semester and saves the rules as JSON in
`~/.config/moodle-mcp/courses/<programme>/<CODE>.json` (outside the repository: they name
lecturers and belong to one programme). `scripts/profile.py check|save|table|verify` validates
them — e.g. weights must add up to 100, so a table row lost while reading the PDF cannot slip
through. Briefing, assignment, grades and dashboard use the profiles when they exist.

## How they were tested

`dev/skill-evals/` holds the eval set (9 realistic prompts across all seven skills) and a programmatic grader. Runs use a fake Moodle (`dev/fakemoodle`, the same
fixture world as the Go tests) through the real server binary (`dev/mcpcall`), so nothing
touches the real site. Each prompt runs with and without the skills; results, grading and a
review page live in `skills-workspace/` (git-ignored).

Results (subagent runs on the fake world, programmatic grading; the prompts were in Russian,
German and English at the time and have since been switched to English):

- Iteration 2 — 8 prompts: **100 % of assertions with skills vs. 94 % without**, same time,
  ~5 % more tokens. The baseline is already strong because the server output is well
  structured; the skills add what the model does not do reliably on its own — carrying earlier
  teacher feedback into the next assignment, a complete token-recovery path, a consistent
  prioritised format, and a deterministic, themed dashboard.
- Iteration 3 — course profiles: "how much is the homework worth / what must I pass" **7/7 vs.
  3/7** (the baseline cannot see the rules without a profile in the fake world). No regressions in briefing, assignment and grades (7/7 each).

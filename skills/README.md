# Skills for moodle-mcp

Claude Code skills that turn the moodle-mcp tools into good study workflows. Written in
English; they trigger and answer in whatever language the student writes (tested in Russian,
German and English).

| Skill | Use it for | Example prompts |
|---|---|---|
| [moodle-briefing](moodle-briefing/SKILL.md) | what is urgent, new announcements, what is due — prioritised | "what's due this week?", "что у меня по учёбе?", "gibt es was Dringendes?" |
| [moodle-materials](moodle-materials/SKILL.md) | find material and answer from it, with page and quote | "how do I connect to the lab VM from home?", "где инструкция по ssh?" |
| [moodle-assignment](moodle-assignment/SKILL.md) | one assignment: deadline, status, task sheet, checklist, plan, earlier feedback | "what do I need to do for Homework R2?" |
| [moodle-grades](moodle-grades/SKILL.md) | grades, percentages, teacher feedback with translation | "how are my grades?", "что написал преподаватель?" |
| [moodle-dashboard](moodle-dashboard/SKILL.md) | a visual HTML page: calendar, urgent cards, timeline, announcements, grades, plan | "make a dashboard of my next two weeks" |
| [moodle-setup](moodle-setup/SKILL.md) | connection problems, getting and renewing a token, registration, HTTP mode | "moodle doesn't work", "как обновить токен" |

## Install

```bash
make install-skills      # symlinks every skill into ~/.claude/skills
make uninstall-skills    # removes those symlinks again
```

Symlinks mean `git pull` updates the skills without reinstalling. Restart Claude Code after
installing.

## Design rules shared by all skills

- **Answer in the user's language**, keep quotes, course and file names in the original.
- **Links exactly as the tools return them**: browser links into Moodle, never `/webservice/`
  or `token=`.
- **Invent nothing**: empty data is said plainly; general knowledge is labelled as such.
- **Read-only**: the skills point to where to click; the server cannot submit or post.
- **Actions over facts**: "submit the draft" rather than "status: draft".

## How they were tested

`dev/skill-evals/` holds the eval set (8 realistic prompts across all six skills, in three
languages) and a programmatic grader. Runs use a fake Moodle (`dev/fakemoodle`, the same
fixture world as the Go tests) through the real server binary (`dev/mcpcall`), so nothing
touches the real site. Each prompt runs with and without the skills; results, grading and a
review page live in `skills-workspace/` (git-ignored).

Latest result (iteration 2): **100 % of assertions with skills vs. 94 % without**, same time,
~5 % more tokens. The baseline is already strong because the server output is well structured;
the skills add what the model does not do reliably on its own — carrying earlier teacher
feedback into the next assignment, a complete token-recovery path, a consistent prioritised
format, and a deterministic, themed dashboard.

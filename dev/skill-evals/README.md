# Skill evals

- `evals.json` — 8 realistic prompts (Russian, German, English) covering all six skills, plus
  the facts of the fake world they are graded against.
- `grade.py <workspace>/iteration-N` — programmatic assertions per prompt (facts, language,
  safety rules, promised behaviours); writes `grading.json` in the skill-creator format.
- `run_evals.py <workspace>/iteration-N` — runs every prompt with and without skills through
  `claude -p` against the fake Moodle. Needs a logged-in `claude` CLI.

The fake world: `make dev`, then `bin/fakemoodle -url-file /tmp/fake.url` serves
`moodletest.World` (the fixtures of the Go tests). `bin/mcpcall <tool> '<json>'` calls a tool
through the real server binary; with `MOODLE_URL` set to the fake server and `MOODLE_TOKEN` to
`bin/fakemoodle -token` nothing touches the real site.

When the CLI cannot log in (e.g. inside the desktop app's sandbox), run each prompt with a
subagent that reaches Moodle only through a per-run `mcpcall` wrapper, as done for the
current results. Layout: `<iteration>/eval-<id>-<name>/{with_skill,without_skill}/run-1/{outputs/answer.md,timing.json,grading.json}`;
then the skill-creator `aggregate_benchmark` and `eval-viewer/generate_review.py`.
Keep HTML outputs out of `outputs/` (the viewer inlines them and breaks on `</script>`).

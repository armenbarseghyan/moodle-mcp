---
name: moodle-grades
description: Grades from Moodle (FH JOANNEUM) — scores, percentages, teacher feedback, what is still ungraded, how a course is going. Use this skill whenever the user asks about grades or performance, in any language — "how are my grades", "what did I get for R1", "what did the teacher write", "are grades out yet?", "Noten", "wie sind meine Noten" — even if Moodle is not named. Not for deadlines (moodle-briefing).
allowed-tools: mcp__moodle__moodle_grades, mcp__moodle__moodle_courses, Read, Glob
---

# Grades

Grades are sensitive: be exact and infer nothing. Everything you show comes from
`moodle_grades`, which always fetches fresh data from Moodle.

Reply in the user's language; keep teacher feedback in the original and add a translation.

## Gather

- One course — `moodle_grades(course=<id or part of the name>)`. If the server returns a list
  of candidates, ask which course.
- Everything — `moodle_grades()` without parameters.

## Course rules

Course profiles (skill moodle-course-profile) hold each course's rules from the syllabus:
`$MOODLE_PROFILES_DIR`, default `~/.config/moodle-mcp/courses/<programme>/<CODE>.json`.
When a profile exists, show next to the grades what they count towards: the component and its
weight, components that must be passed separately (★), extra pass rules ("≥ 61 % on the
written exam"), and the course's own scale if it differs from 91/81/71/61. Compute a projected
course grade only when grade items map clearly onto components; otherwise show rules and grades
side by side and say why you are not computing a total.

## Output

Template (write the headings in the user's language):

```markdown
## 🎓 Grades

### <Course>
| Work | Grade | % | Graded |
|---|---|---|---|
| [Homework R1](…) | **87.5** / 100 | 87.5 % | 01.10 |

💬 **Feedback on Homework R1:** "<teacher's text, original>" — <translation if needed>
⏳ Not graded yet: N · 📊 Course total: **…**

_No grades yet: <courses, one line>_
```

- Teacher feedback is the most valuable part: show it in full when short, translated, with the
  original kept. If it contains a remark ("Plots beschriften"), highlight it as something to
  apply in the next assignment.
- Take percentages from the tool. Convert to the Austrian 1–5 grade only with the scale from
  the course profile (FH default: ≥91 → 1, ≥81 → 2, ≥71 → 3, ≥61 → 4, below → 5, rounding per
  DIN 1333); without a profile say the scale comes from the syllabus and offer to read it
  (skill moodle-course-profile).
- No grades means none yet — say so in one line, without a table.
- No comparisons with other students and no verdicts like "failing" — facts and what can be
  done next.

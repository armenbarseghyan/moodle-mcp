---
name: moodle-grades
description: Grades from Moodle (FH JOANNEUM) — scores, percentages, teacher feedback, what is still ungraded, how a course is going. Use this skill whenever the user asks about grades or performance, in any language — "how are my grades", "what did I get for R1", "what did the teacher write", "are grades out yet?", "как у меня с оценками", "какую оценку я получил за R1", "что написал преподаватель", "Noten", "wie sind meine Noten" — even if Moodle is not named. Not for deadlines (moodle-briefing).
allowed-tools: mcp__moodle__moodle_grades, mcp__moodle__moodle_courses
---

# Grades

Grades are sensitive: be exact and infer nothing. Everything you show comes from
`moodle_grades`, which always fetches fresh data from Moodle.

Reply in the user's language; keep teacher feedback in the original and add a translation.

## Gather

- One course — `moodle_grades(course=<id or part of the name>)`. If the server returns a list
  of candidates, ask which course.
- Everything — `moodle_grades()` without parameters.

## Output

Template (write the headings in the user's language):

```markdown
## 🎓 Grades

### <Course>
| Work | Grade | % | Graded |
|---|---|---|---|
| [Homework R1](…) | **87.50** / 100 | 87.5 % | 01.10 |

💬 **Feedback on Homework R1:** "<teacher's text, original>" — <translation if needed>
⏳ Not graded yet: N · 📊 Course total: **…**

_No grades yet: <courses, one line>_
```

- Teacher feedback is the most valuable part: show it in full when short, translated, with the
  original kept. If it contains a remark ("Plots beschriften"), highlight it as something to
  apply in the next assignment.
- Take percentages from the tool; do not convert to the Austrian 1–5 scale yourself — courses
  use different conversion rules. If asked "what grade is that?", say the scale depends on
  the course and offer to find it in the syllabus (skill moodle-materials).
- No grades means none yet — say so in one line, without a table.
- No comparisons with other students and no verdicts like "failing" — facts and what can be
  done next.

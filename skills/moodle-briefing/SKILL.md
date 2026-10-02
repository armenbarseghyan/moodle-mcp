---
name: moodle-briefing
description: Study briefing from Moodle (FH JOANNEUM) — what is urgent, what teachers announced, what is due and whether it is submitted. Use this skill whenever the user asks about their studies in general or about the near future, in any language — "what's due this week", "anything urgent on moodle?", "did I miss anything?", "good morning, what's up at uni", "что у меня по учёбе", "что сдавать на неделе", "есть что-то срочное в мудле?", "was steht diese Woche an", "habe ich etwas verpasst" — even if Moodle is not named, and for daily or weekly check-ins. Do not use it to find a specific material (moodle-materials) or to work through one assignment (moodle-assignment).
allowed-tools: mcp__moodle__moodle_announcements, mcp__moodle__moodle_deadlines, mcp__moodle__moodle_grades, mcp__moodle__moodle_courses, Read, Glob
---

# Study briefing

Goal: answer "what matters for my studies right now" in a 20-second read. The user is a
student whose worst case is missing a room change or leaving a draft unsubmitted. So the
briefing does not re-list Moodle — it **prioritises**.

Reply in the user's language (the language of their message), whatever language the tools or
Moodle use.

## Gather

Call both in parallel (they are independent):
- `moodle_announcements` — posts by teachers. Most important: room changes, cancellations and
  assignment changes show up here at the last minute.
- `moodle_deadlines` — deadlines with submission status and an overdue block.

Pick the window from the question:

| Question | announcements `days` | deadlines `days` |
|---|---|---|
| "today", "this morning", "anything urgent" | 3 | 2 |
| unspecified, "this week" | 7 | 7 |
| "next two weeks", "soon" | 7 | 14 |
| "this month" | 14 | 30 |

Call `moodle_grades` only if the user asked about grades or an announcement says grades are
out ("Noten sind online"). It is not part of the default briefing.

If a tool returns a token or connection error, say what happened in one line and suggest
checking with `moodle_whoami` (skill moodle-setup). Never guess data.

## Prioritise

Sort everything into three levels, top to bottom:

1. **🔴 Urgent** — overdue and not submitted; due within 48 h and not submitted; **a draft that
   was never submitted** (status "draft" is the classic trap: the file is uploaded, but the
   teacher does not see it until it is submitted); an announcement moving or cancelling the
   next class.
2. **📣 Announcements** — the other recent posts, 1–2 sentences of substance each.
3. **📅 Coming up** — the remaining deadlines by date; already submitted ones in one line at
   the end.

If course profiles exist (skill moodle-course-profile; files in
`~/.config/moodle-mcp/courses/`), use them to order within a level: a component that must be
passed separately or carries a large weight beats a 4 % exercise. Mention the weight briefly
("30 % of the grade") where it changes what the student should do first.

If nothing is urgent, say so in one line ("Nothing urgent ✨") — that is valuable too.

## Output

Template (shown in English; write the headings and text in the user's language):

```markdown
## 📚 Studies: <period>, as of <today>

### 🔴 Urgent
- **<action to take>** — <course> · due **<date time>** (<in N days>) · [open](<link>)

### 📣 Announcements
- **<course>**: <one-sentence gist>. "<key quote in the original language>" · [open](<link>)

### 📅 Coming up
| When | Course | What | Status |
|---|---|---|---|
| Wed 07.10, 17:15 · in 5 days | Programming… | [Homework R1](…) | ✅ submitted |

✅ Already submitted: <one line> · _N more assignments are still hidden by teachers_

**Next I can:** <1–2 concrete next steps, e.g. "open Homework R2" or "find the R2 lecture">
```

What makes the briefing useful:
- **Actions, not facts.** "Submit the Home Assignment draft" beats "status: draft".
- **Translate announcements, keep key facts verbatim** (room, time, names): the user will look
  for them in Moodle.
- **Links exactly as the tools return them.** They open in the user's logged-in browser. Do not
  rewrite, shorten or invent links.
- **No duplicates.** An item in "Urgent" does not appear again in the table.
- **Empty means empty.** No deadlines and no posts — say so briefly and mention hidden
  assignments if the tool counted any (they open later; worth checking back).
- **Cache.** If the tool marks data as cached and the user asks whether something changed "right
  now", call again with `refresh=true`.
- **Read-only.** The server cannot submit, send drafts or enrol. Point to the place to click,
  with the link.

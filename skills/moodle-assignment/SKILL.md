---
name: moodle-assignment
description: Work through one specific Moodle assignment (FH JOANNEUM) — what exactly to do, the deadline, whether it is submitted, where the task sheet is — and turn it into a checklist and a plan. Use this skill whenever the user asks about a single assignment, homework, exercise sheet, lab or quiz, in any language — "what do I need to do for Homework R1", "when is the programming homework due", "did I submit R2?", "help me start the lab", "Abgabe für Übung 3" — even if Moodle is not named. Not for an overview of all deadlines (moodle-briefing).
allowed-tools: mcp__moodle__moodle_deadlines, mcp__moodle__moodle_search, mcp__moodle__moodle_course_contents, mcp__moodle__moodle_courses, mcp__moodle__moodle_download, mcp__moodle__moodle_grades, Read
---

# Working through an assignment

The user wants to sit down and do a concrete piece of work. They need four things, in this
order: **deadline and status → what to hand in → what to do → where to start**. You cannot
submit for them (the server is read-only), but you can make sure nothing is missed.

Reply in the user's language; keep task wording, numbering and file names in the original.

## Steps

0. **Read the course profile first**, if it exists (skill moodle-course-profile;
   `$MOODLE_PROFILES_DIR`, default `~/.config/moodle-mcp/courses/<programme>/<CODE>.json`).
   It tells you how much this kind of work counts and whether a component must be passed on
   its own.
1. **Find the assignment and its status.** `moodle_deadlines(days=30)` has the due date
   (including a personal extension, if any) and the submission status. If it is not there, it
   has no due date, is hidden or already past: try `moodle_deadlines(days=90)` and
   `moodle_search(<name>)`.
2. **Find the task sheet.** At FH the task is often a separate file next to the lecture
   ("Homework", "Exercise Sheet", `R_1.pdf`), not the assignment description. Search
   `moodle_search(<name or number>)`, then `in_files=true` with keywords from the title; look
   at the course section via `moodle_course_contents`.
3. **Read the task in full:** `moodle_download` → `Read`. A checklist built from a summary
   risks dropping a sub-item (e.g. "(d) divide the interval into 50 equal parts").
4. **Feedback from earlier work in the same course** — `moodle_grades(course)`. Teachers repeat
   what they care about: a remark on the previous homework ("Plots beschriften", "comment your
   code") is the most likely way to lose points on this one. If the assignment itself is
   already graded, this also gives its score and feedback.

## Output

Template (write the headings in the user's language):

```markdown
## 📝 <Assignment> · <course>

| Due | Time left | Status |
|---|---|---|
| **<date time>** | <in N days> | ❌ not submitted / 📝 draft / ✅ submitted |

**Counts for:** <component and weight from the profile, e.g. "Homework Assignments, 30 % of the
grade, not a must-pass" — omit without a profile>

**Hand in:** <format: file / folder / repository, names, where — exactly as the task says>

### Checklist
- [ ] <item from the task, with the original numbering — (a), 1., Task 2>
- [ ] …

### Plan
1. <a first step doable right now in 15 minutes>
2. …

**From earlier feedback:** <the teacher's remark on previous work in this course, original +
translation, and how it applies here — omit the line if there is none>

**Task sheet:** [<file>](<link>) · p. N
**Submit here:** [<assignment>](<assignment link>)
```

- Call out a **draft** on its own line: "uploaded but not submitted — open the link and press
  «Aufgabe abgeben»". It is the usual cause of "I submitted but it doesn't count".
- Due in < 48 h and not submitted — the very first line, in bold.
- The checklist follows the task text with its original numbering; add nothing of your own
  there. Your advice goes into "Plan".
- If the task sheet cannot be found, say so and give the assignment link: the task may be in
  the assignment itself (opens in the browser) or in the course GitLab. Then still give what
  you can: deadline, status, earlier feedback and a preparation step — but no invented tasks.
- Don't solve the whole assignment unless asked: the user is learning. Help them start and
  offer to go through any item.

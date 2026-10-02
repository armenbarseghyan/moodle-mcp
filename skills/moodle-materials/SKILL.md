---
name: moodle-materials
description: Find course material in Moodle (FH JOANNEUM) and answer from its content — slides, lectures, PDFs, how-tos (VPN, SSH, GitLab, VMs, lab PCs), templates, archives, course pages. Use this skill whenever the user looks for something from their courses or asks "how do I X" where the answer likely sits in course material, in any language — "where is the lecture on…", "find the slides", "how do I connect to the VM / VPN / server", "what was in lecture R1", "is there a git guide?", "где лежит лекция про…", "как подключиться к VM", "найди инструкцию по VPN", "wo finde ich die Folien", "Anleitung VPN" — even if Moodle is not mentioned. Not for deadlines (moodle-briefing) or for working through one assignment (moodle-assignment).
allowed-tools: mcp__moodle__moodle_search, mcp__moodle__moodle_course_contents, mcp__moodle__moodle_courses, mcp__moodle__moodle_download, Read
---

# Finding material and answering from it

The user remembers the topic, not the place. Find the **source** and, when the question is
about content, **answer from it with a quote and page number** — not from general knowledge.
FH instructions are specific (host names, VPN client, exact commands); a generic answer would
be wrong.

Reply in the user's language; keep quotes, commands and file names in the original.

## Search strategy

1. **Search in the material's language.** DAT courses are taught in English, the site is in
   German, the user may write in Russian or anything else. Translate the intent into 1–2
   English keywords (German as a fallback): "как подключиться к виртуалке" → `VM`, `remote`,
   `ssh`; "exam schedule" → `exam`, `assessment`, `Prüfung`. Every word must match, so keep it
   short.
2. **Metadata first:** `moodle_search(query)` — module names, file names, descriptions. Fast.
3. **Then content:** if names give nothing, or the question is about substance ("how", "what
   do I need", "which port") — `moodle_search(query, in_files=true)`. It searches inside PDF,
   docx, pptx, ipynb, zip and Moodle pages and returns the page and a quote. The first call is
   slower (it downloads files); later calls are cached.
4. **Narrow to a course** if the user named one: `course` (id or part of the name). If the
   server returns a list of candidates, ask which one, showing the options.
5. **Course overview** ("what's in course X", "which lectures are up") —
   `moodle_course_contents(course)`.

Try 2–3 query variants before saying "not found". If nothing turns up, say where you looked
(queries, with or without `in_files`) and suggest why: not uploaded yet, it lives in the
course GitLab (the link is often in the "Allgemeines" section), or ask the teacher.

## When to read the whole file

The `in_files` quote usually answers "where" and "roughly what". For an exact answer (steps
of a guide, task wording, a formula), download with `moodle_download(fileurl)` and read it
(`Read`; for PDFs start at the page the search found). Unpack `.zip` files into a temporary
folder first. Downloading changes nothing in Moodle, but it does put a file on disk — don't
download everything "just in case".

## Output

```markdown
## 🔎 <what was searched for>

<Direct answer: 2–6 lines or numbered steps. Commands and hosts in `code`.>

> "<short quote from the source, original language>"

**Source:** [<file name>](<link>) · <p. N> · <course> › <section>
**Related:** [<file 2>](<link>) — <why it helps>
```

- Answer first, source second. The user came for the answer, not for a list of files.
- Use links exactly as the tool returns them: they open in the user's logged-in browser. Never
  show links containing `/webservice/` or `token=`.
- When the answer combines several files, cite the source of each fact.
- If the material does not answer the question, say so; you may add general knowledge, clearly
  marked: "not in the FH material; in general…".

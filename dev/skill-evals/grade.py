#!/usr/bin/env python3
"""Grade skill eval runs with programmatic assertions.

Usage:
    dev/skill-evals/grade.py <workspace>/iteration-N

For every <iteration>/eval-<id>-<name>/<config>/run-*/ it reads outputs/answer.md
(and any outputs/*.html), evaluates the assertions defined below for <name> and
writes grading.json in the skill-creator format (expectations: text/passed/evidence).

Assertions describe what matters to the student, not how the answer is phrased:
facts from the fake world (dev/skill-evals/evals.json), safety rules, language,
and the behaviours the skills promise.
"""
import glob
import json
import os
import re
import sys


def cyr_ratio(s):
    letters = [c for c in s if c.isalpha()]
    return sum("а" <= c.lower() <= "я" or c in "ёЁ" for c in letters) / max(1, len(letters))


def has(s, *pats, flags=re.I):
    return all(re.search(p, s, flags) for p in pats)


def find(s, pat, flags=re.I):
    m = re.search(pat, s, flags)
    if not m:
        return "not found"
    a, b = max(0, m.start() - 60), min(len(s), m.end() + 60)
    return "…" + s[a:b].replace("\n", " ") + "…"


def common(ans):
    bad = re.findall(r"https?://\S*(?:/webservice/|token=)\S*", ans)
    return [("No links with /webservice/ or token= (links must open in the student's browser)",
             not bad, ", ".join(bad[:2]) or "all links are browser links")]


def prose(s):
    """Text without code blocks, inline code, quotes and URLs: the part written in the reply language."""
    s = re.sub(r"```.*?```", " ", s, flags=re.S)
    s = re.sub(r"`[^`]*`|https?://\S+|«[^»]*»|\"[^\"]*\"|^>.*$", " ", s, flags=re.M)
    return s


def lang_ru(ans):
    r = cyr_ratio(prose(ans))
    return ("Answers in Russian, the language of the question", r > 0.5, f"cyrillic share {r:.0%}")


CHECKS = {
    "briefing-week-ru": lambda a, h: [
        lang_ru(a),
        ("Flags Homework R0 as overdue and not submitted",
         has(a, r"R0") and has(a, r"просроч|срок (был|прош)|не сдан"), find(a, r"R0")),
        ("Overdue item comes before the upcoming deadlines (prioritised)",
         0 <= a.find("R0") < a.find("Quiz") if "Quiz" in a and "R0" in a else False,
         f"R0 at {a.find('R0')}, Quiz at {a.find('Quiz')}"),
        ("Mentions the room change announcement with the original room 'Raum 0.12'",
         has(a, r"0\.12"), find(a, r"0\.12")),
        ("Shows Homework R2 as not submitted", has(a, r"R2[^\n]*не сдан|не сдан[^\n]*R2"), find(a, r"R2")),
        ("Ends with concrete next steps the assistant can take",
         has(a, r"(Дальше могу|могу (разобрать|найти|открыть|помочь))"), find(a, r"Дальше могу|могу")),
    ],
    "briefing-urgent-de": lambda a, h: [
        ("Answers in German", has(a, r"\b(und|nicht|ist|der|die)\b") and cyr_ratio(a) < 0.05, a[:60]),
        ("Flags Homework R0 as overdue / not submitted",
         has(a, r"R0") and has(a, r"überfällig|nicht abgegeben|war fällig|Frist war"), find(a, r"R0")),
        ("Mentions the Raumänderung announcement", has(a, r"Raum|0\.12"), find(a, r"Raum")),
        ("Stays short for a morning check (under 1800 characters)", len(a) < 1800, f"{len(a)} chars"),
    ],
    "materials-ssh-key-ru": lambda a, h: [
        lang_ru(a),
        ("Gives the exact command from the course material", has(a, r"ssh-keygen -t ed25519"), find(a, r"ssh-keygen")),
        ("Cites the instructions file with a link", has(a, r"ssh_instructions"), find(a, r"ssh_instructions")),
        ("Mentions the VPN requirement from the material", has(a, r"VPN"), find(a, r"VPN")),
        ("Separates general knowledge from what the FH material says",
         has(a, r"(не из курса|не из материал|в материалах FH|общ(ая|ие) (практика|шаги)|в инструкции не)"),
         find(a, r"не из курса|не из материал|в материалах FH|общ(ая|ие)|в инструкции не")),
    ],
    "materials-vm-from-home-en": lambda a, h: [
        ("Answers in English", cyr_ratio(a) < 0.05 and has(a, r"\bthe\b"), a[:60]),
        ("States that the FH VPN is required off campus", has(a, r"VPN") and has(a, r"off.?campus|from home"), find(a, r"VPN")),
        ("Cites a source file with a page number", has(a, r"\b(p\.|pp\.|page)\s*\d"), find(a, r"\b(p\.|pp\.|page)\s*\d")),
        ("Links the source as a browser pluginfile or module link", has(a, r"/pluginfile\.php/|/mod/resource/view\.php"),
         find(a, r"/pluginfile\.php/|/mod/resource/view\.php")),
    ],
    "assignment-r2-ru": lambda a, h: [
        lang_ru(a),
        ("Gives the due date 10.10 at 10:00", has(a, r"10[.,]10|10 октября") and has(a, r"10:00"), find(a, r"10:00")),
        ("Says R2 is not submitted yet", has(a, r"не сдан"), find(a, r"не сдан")),
        ("Links the assignment page to submit", has(a, r"view\.php\?id=190503"), find(a, r"190503")),
        ("Does not invent task items: says the task sheet was not found",
         has(a, r"не наш(ёл|ел|ла)|нет (раздела|листка|файла)|ещё нет|пока нет") and "- [ ]" not in a,
         find(a, r"не наш|нет (раздела|листка)|пока нет")),
        ("Carries over the teacher's feedback on R1 ('Plots beschriften') as advice for R2",
         has(a, r"Plots|подпис\w* график"), find(a, r"Plots|подпис\w* график")),
    ],
    "grades-feedback-ru": lambda a, h: [
        lang_ru(a),
        ("Shows the R1 grade 87.5 out of 100", has(a, r"87[,.]5") and has(a, r"100"), find(a, r"87")),
        ("Quotes the teacher feedback in the original", has(a, r"Gute Arbeit, aber Plots beschriften"), find(a, r"Gute Arbeit")),
        ("Translates the feedback", has(a, r"подпис\w* график"), find(a, r"подпис")),
        ("Mentions the one item not graded yet", has(a, r"(не оцен\w*|ещё не|пока не)[^\n]{0,30}(1|одна|ещё одна)|(1|одна)[^\n]{0,30}не оцен"),
         find(a, r"не оцен")),
        ("Invents no grades for the courses without grades",
         not has(a, r"(Machine Learning|Unix Shells)[^\n]{0,80}\d+[,.]\d+\s*/"), "no grade numbers next to other courses"),
    ],
    "dashboard-two-weeks-ru": lambda a, h: [
        ("Produces an HTML page", bool(h), ", ".join(os.path.basename(p) for p in h) or "no html"),
        ("Page uses the shared design template (consistent look across runs)",
         any("moodle-dash-theme" in open(p, encoding="utf-8").read() for p in h), "template marker"),
        ("Page supports a dark theme",
         any(re.search(r"prefers-color-scheme:\s*dark|data-theme|\.dark\b", open(p, encoding="utf-8").read()) for p in h),
         "dark theme rules"),
        ("Page contains the overdue Homework R0 and the unsubmitted R2",
         any(("R0" in (t := open(p, encoding="utf-8").read())) and "R2" in t for p in h), "R0 and R2 in page"),
        ("Page links contain no token or /webservice/",
         not any(re.search(r"token=|/webservice/", open(p, encoding="utf-8").read()) for p in h), "links checked"),
        ("Chat reply summarises the essentials (R0 overdue) without opening the page", has(a, r"R0"), find(a, r"R0")),
    ],
    "setup-revoked-token-ru": lambda a, h: [
        lang_ru(a),
        ("Diagnoses the revoked/invalid token", has(a, r"токен\w*[^\n]{0,40}(недействит|отозван)|(недействит|отозван)[^\n]{0,40}токен"),
         find(a, r"недействит|отозван")),
        ("Explains where to get a new token (Sicherheitsschlüssel)", has(a, r"Sicherheitsschl"), find(a, r"Sicherheitsschl")),
        ("Gives the re-registration command", has(a, r"claude mcp add"), find(a, r"claude mcp add")),
        ("Tells the student not to paste the token into the chat", has(a, r"(не (вставляй|присылай|отправляй)[^\n]{0,40}(чат|сюда))|(в чат[^\n]{0,20}не)"),
         find(a, r"чат")),
        ("Offers a fallback if no key is listed (Moodle app / token.php / IT service)",
         has(a, r"(мобильн\w* приложени|приложени\w* Moodle|Moodle[- ]App|token\.php|IT[- ]?Service|ZID|helpdesk)"),
         find(a, r"мобильн\w* приложени|приложени\w* Moodle|Moodle[- ]App|token\.php|IT[- ]?Service|ZID|helpdesk")),
    ],
    "profile-rules-ru": lambda a, h: [
        lang_ru(a),
        ("Homework weighs 30 %", has(a, r"30\s*%|30 процент") and has(a, r"домашн|homework"), find(a, r"30\s*%")),
        ("Names the four written/code assessments at 17.5 % each", has(a, r"17[,.]5") and has(a, r"Python") and has(a, r"\bR\b"),
         find(a, r"17[,.]5")),
        ("Says each of them must be passed separately", has(a, r"отдельно|separately|каждую"), find(a, r"отдельно|separately|каждую")),
        ("Mentions that homework points carry over to a retake", has(a, r"перенос|сохраня|carr(y|ied) over"),
         find(a, r"перенос|сохраня|carr")),
        ("Links the syllabus as the source", has(a, r"Syllabus_DAT26_PDP"), find(a, r"Syllabus")),
    ],
}


def grade(run_dir, name):
    ans_path = os.path.join(run_dir, "outputs", "answer.md")
    ans = open(ans_path, encoding="utf-8").read() if os.path.exists(ans_path) else ""
    html = sorted(glob.glob(os.path.join(run_dir, "outputs", "*.html")))
    results = CHECKS[name](ans, html) + common(ans)
    exps = [{"text": t, "passed": bool(p), "evidence": e} for t, p, e in results]
    passed = sum(e["passed"] for e in exps)
    # Time and tokens stay in the sibling timing.json, where the aggregator reads both.
    out = {"expectations": exps,
           "summary": {"passed": passed, "failed": len(exps) - passed, "total": len(exps),
                       "pass_rate": round(passed / len(exps), 2)},
           "execution_metrics": {"output_chars": len(ans)}}
    json.dump(out, open(os.path.join(run_dir, "grading.json"), "w"), ensure_ascii=False, indent=2)
    return passed, len(exps)


def main():
    root = sys.argv[1]
    for eval_dir in sorted(glob.glob(os.path.join(root, "eval-*"))):
        name = os.path.basename(eval_dir).split("-", 2)[2]
        for run in sorted(glob.glob(os.path.join(eval_dir, "*", "run-*"))):
            p, n = grade(run, name)
            print(f"{name:28} {run.split(os.sep)[-2]:14} {p}/{n}")


if __name__ == "__main__":
    main()

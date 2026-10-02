#!/usr/bin/env python3
"""Check, save and list course profiles (see ../SKILL.md for the schema).

Usage:
    profile.py check  <file.json>        validate; exit 1 with a list of problems
    profile.py save   <file.json>        validate and write to <dir>/<programme>/<code>.json
    profile.py table                     overview of all saved profiles (markdown)
    profile.py show   <PROG/CODE|CODE>   print one profile
    profile.py verify <PROG/CODE|CODE>   mark a profile as confirmed by the user

<dir> is $MOODLE_PROFILES_DIR, default ~/.config/moodle-mcp/courses.
"""
import glob
import json
import os
import re
import sys

LEVELS = ["none", "learning_only", "homework", "allowed_except_assessments"]
KINDS = {"exam", "test", "homework", "project", "participation", "presentation",
         "self_assessment", "oral", "other"}
LEVEL_LABEL = {"none": "⛔ no AI", "learning_only": "📖 learning only", "homework": "📝 homework only",
               "allowed_except_assessments": "✅ allowed (not in assessments)"}


def base_dir():
    return os.path.expanduser(os.environ.get("MOODLE_PROFILES_DIR") or "~/.config/moodle-mcp/courses")


def check(p):
    errs = []
    req = {"schema": int, "programme": str, "code": str, "name": str, "ects": (int, float),
           "continuous_assessment": bool, "assessment": dict, "ai_policy": dict}
    for k, t in req.items():
        if k not in p:
            errs.append(f"{k} is required")
        elif not isinstance(p[k], t):
            errs.append(f"{k} has the wrong type")
    if errs:
        return errs
    if p["schema"] != 1:
        errs.append("schema must be 1")
    if not re.fullmatch(r"[A-Za-z0-9_-]+", p["programme"]) or not re.fullmatch(r"[A-Za-z0-9_-]+", p["code"]):
        errs.append("programme and code must be plain identifiers (letters, digits, - and _)")
    a = p["assessment"]
    if not a.get("first_attempt"):
        errs.append("assessment.first_attempt must list the components")
    for attempt in ("first_attempt", "retake"):
        comps = a.get(attempt) or []
        for i, c in enumerate(comps):
            where = f"assessment.{attempt}[{i}]"
            if not c.get("name"):
                errs.append(f"{where}.name is required")
            if not isinstance(c.get("weight"), (int, float)) or not 0 < c["weight"] <= 100:
                errs.append(f"{where}.weight must be a number in (0, 100]")
            if not isinstance(c.get("must_pass"), bool):
                errs.append(f"{where}.must_pass must be true or false (☒ Yes / ☒ No in the syllabus)")
            if c.get("kind") not in KINDS:
                errs.append(f"{where}.kind must be one of {sorted(KINDS)}")
        if comps:
            total = sum(c.get("weight", 0) for c in comps if isinstance(c.get("weight"), (int, float)))
            if abs(total - 100) > 0.01:
                errs.append(f"assessment.{attempt} weights add up to {total:g}, not 100 — a row is missing or mistyped")
    ai = p["ai_policy"]
    if ai.get("level") not in LEVELS:
        errs.append(f"ai_policy.level must be one of {LEVELS}")
    if not ai.get("quote"):
        errs.append("ai_policy.quote must keep the syllabus sentence")
    src = p.get("source", {})
    if src.get("url") and ("token=" in src["url"] or "/webservice/" in src["url"]):
        errs.append("source.url must be a browser link (no token, no /webservice/)")
    return errs


def load(path):
    with open(path, encoding="utf-8") as f:
        return json.load(f)


def find(ref):
    ref = ref.removesuffix(".json")
    if "/" in ref:
        path = os.path.join(base_dir(), ref + ".json")
        return path if os.path.exists(path) else None
    hits = glob.glob(os.path.join(base_dir(), "*", ref + ".json")) + \
        glob.glob(os.path.join(base_dir(), "*", ref.upper() + ".json"))
    return sorted(set(hits))[0] if hits else None


def table():
    rows = []
    for path in sorted(glob.glob(os.path.join(base_dir(), "*", "*.json"))):
        p = load(path)
        comps = p["assessment"]["first_attempt"]
        parts = "; ".join(f"{c['name']} {c['weight']:g}{' ★' if c['must_pass'] else ''}" for c in comps)
        rules = "; ".join(p["assessment"].get("pass_rules", []))
        rows.append(f"| **{p['code']}** {p['name']} | {p['ects']:g} | {parts}"
                    f"{(' — ' + rules) if rules else ''} | {LEVEL_LABEL[p['ai_policy']['level']]} | "
                    f"{'✓' if p.get('verified') else '—'} |")
    if not rows:
        return f"No profiles in {base_dir()} yet."
    head = ("| Course | ECTS | Assessment, 1st attempt (★ = must pass separately) | AI | Verified |\n"
            "|---|---|---|---|---|")
    ects = sum(load(p)["ects"] for p in glob.glob(os.path.join(base_dir(), "*", "*.json")))
    return head + "\n" + "\n".join(rows) + f"\n\nTotal: {ects:g} ECTS"


def main(argv):
    if len(argv) < 2:
        print(__doc__, file=sys.stderr)
        return 2
    cmd = argv[1]
    if cmd in ("check", "save"):
        p = load(argv[2])
        errs = check(p)
        if errs:
            print("profile is invalid:\n- " + "\n- ".join(errs), file=sys.stderr)
            return 1
        if cmd == "check":
            print("OK")
            return 0
        out = os.path.join(base_dir(), p["programme"], p["code"] + ".json")
        os.makedirs(os.path.dirname(out), exist_ok=True)
        with open(out, "w", encoding="utf-8") as f:
            json.dump(p, f, ensure_ascii=False, indent=2)
            f.write("\n")
        print(out)
        return 0
    if cmd == "table":
        print(table())
        return 0
    if cmd in ("show", "verify"):
        path = find(argv[2]) if len(argv) > 2 else None
        if not path:
            print(f"no profile {argv[2:]} in {base_dir()}", file=sys.stderr)
            return 1
        p = load(path)
        if cmd == "verify":
            p["verified"] = True
            with open(path, "w", encoding="utf-8") as f:
                json.dump(p, f, ensure_ascii=False, indent=2)
                f.write("\n")
        print(json.dumps(p, ensure_ascii=False, indent=2))
        return 0
    print(__doc__, file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv))

#!/usr/bin/env python3
"""Render the study dashboard: inject a JSON data file into the HTML template.

Usage:
    render.py data.json [out.html]

Without out.html the page is written to $MOODLE_DOWNLOAD_DIR (default
~/Downloads/moodle) as dashboard-<date>.html.
The path of the written file is printed on stdout.

The data is validated against the shape documented in SKILL.md; problems are
reported on stderr with a non-zero exit code so they can be fixed and the
command re-run. All text is rendered with textContent in the page, so data is
never interpreted as HTML.
"""
import datetime
import json
import os
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
TEMPLATE = os.path.join(HERE, "..", "assets", "dashboard.html")
PLACEHOLDER = "__DASHBOARD_DATA__"
STATUSES = {"done", "graded", "draft", "todo", "overdue", "reopened", "na", "unknown"}


def validate(d):
    errs = []
    if not isinstance(d, dict):
        return ["top level must be an object"]
    if not d.get("title"):
        errs.append("title is required")
    iso = re.compile(r"^\d{4}-\d{2}-\d{2}$")
    if d.get("today") and not iso.match(str(d["today"])):
        errs.append("today must be YYYY-MM-DD")
    for key in ("urgent", "deadlines", "announcements", "grades", "courses", "notes", "plan"):
        if key in d and not isinstance(d[key], list):
            errs.append(f"{key} must be a list")
    for i, x in enumerate(d.get("deadlines", [])):
        for f in ("day", "title"):
            if not x.get(f):
                errs.append(f"deadlines[{i}].{f} is required")
        if x.get("date") and not iso.match(str(x["date"])):
            errs.append(f"deadlines[{i}].date must be YYYY-MM-DD")
        if x.get("status") and x["status"] not in STATUSES:
            errs.append(f"deadlines[{i}].status must be one of {sorted(STATUSES)}")
    for i, x in enumerate(d.get("urgent", [])):
        if not x.get("title"):
            errs.append(f"urgent[{i}].title is required")
    for i, x in enumerate(d.get("announcements", [])):
        if not x.get("title"):
            errs.append(f"announcements[{i}].title is required")
    for i, x in enumerate(d.get("grades", [])):
        if "percent" in x and x["percent"] is not None and not isinstance(x["percent"], (int, float)):
            errs.append(f"grades[{i}].percent must be a number")
    for path, url in urls(d):
        if not (url.startswith("https://") or url.startswith("http://")):
            errs.append(f"{path} must be an http(s) URL")
        if "token=" in url or "/webservice/" in url:
            errs.append(f"{path} must be a browser link (no token, no /webservice/)")
    return errs


def urls(d):
    for key in ("urgent", "deadlines", "announcements", "grades", "courses"):
        for i, x in enumerate(d.get(key, []) or []):
            if isinstance(x, dict) and x.get("url"):
                yield f"{key}[{i}].url", x["url"]


def main(argv):
    if len(argv) < 2:
        print(__doc__, file=sys.stderr)
        return 2
    with open(argv[1], encoding="utf-8") as f:
        data = json.load(f)
    errs = validate(data)
    if errs:
        print("dashboard data is invalid:\n- " + "\n- ".join(errs), file=sys.stderr)
        return 1
    with open(TEMPLATE, encoding="utf-8") as f:
        tpl = f.read()
    # "</" would let data close the <script> block early.
    payload = json.dumps(data, ensure_ascii=False).replace("</", "<\\/")
    html = tpl.replace(PLACEHOLDER, payload)

    out_dir = os.path.expanduser(os.environ.get("MOODLE_DOWNLOAD_DIR") or "~/Downloads/moodle")
    out = argv[2] if len(argv) > 2 else os.path.join(
        out_dir, f"dashboard-{datetime.date.today().isoformat()}.html")
    os.makedirs(os.path.dirname(os.path.abspath(out)), exist_ok=True)
    with open(out, "w", encoding="utf-8") as f:
        f.write(html)
    print(os.path.abspath(out))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))

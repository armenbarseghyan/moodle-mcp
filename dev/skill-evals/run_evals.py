#!/usr/bin/env python3
"""Run the moodle skills against a fake Moodle with `claude -p`, with and without skills.

Usage:
    dev/skill-evals/run_evals.py <workspace>/iteration-N [--only name,name] [--jobs 4]

Each eval runs twice: in a work dir whose .claude/skills links to ./skills (with_skill)
and in an empty work dir (without_skill). Both see only the moodle-mcp server pointed at
the fake Moodle (dev/fakemoodle), never the real site, and only project settings.
Results: <iteration>/<eval-name>/<config>/{outputs/answer.md, outputs/*.html,
transcript.jsonl, timing.json} plus eval_metadata.json per eval.
"""
import argparse
import concurrent.futures as cf
import json
import os
import shutil
import subprocess
import sys
import tempfile
import time

REPO = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
EVALS = os.path.join(os.path.dirname(os.path.abspath(__file__)), "evals.json")
ALLOWED = ["mcp__moodle", "Read", "Write", "Glob", "Grep", "Skill",
           "Bash(python3:*)", "Bash(unzip:*)", "Bash(mkdir:*)", "Bash(ls:*)", "Bash(cat:*)"]


def start_fake(tmp):
    binpath = os.path.join(tmp, "fakemoodle")
    subprocess.run(["go", "build", "-o", binpath, "./dev/fakemoodle"], cwd=REPO, check=True)
    token = subprocess.run([binpath, "-token"], capture_output=True, text=True, check=True).stdout.strip()
    url_file = os.path.join(tmp, "fake.url")
    proc = subprocess.Popen([binpath, "-url-file", url_file], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    for _ in range(100):
        if os.path.exists(url_file) and open(url_file).read().strip():
            return proc, open(url_file).read().strip(), token
        time.sleep(0.1)
    proc.kill()
    sys.exit("fake moodle did not start")


def workdir(tmp, name, with_skills):
    d = os.path.join(tmp, name)
    os.makedirs(os.path.join(d, ".claude"), exist_ok=True)
    if with_skills:
        os.symlink(os.path.join(REPO, "skills"), os.path.join(d, ".claude", "skills"))
    return d


def run_one(ev, config, base_url, token, tmp, out_root):
    run_dir = os.path.join(out_root, ev["name"], config)
    out_dir = os.path.join(run_dir, "outputs")
    os.makedirs(out_dir, exist_ok=True)
    dl_dir = os.path.join(tmp, f"dl-{ev['name']}-{config}")
    os.makedirs(dl_dir, exist_ok=True)
    tok = "revoked-token-000" if ev.get("token") == "revoked" else token
    mcp = {"mcpServers": {"moodle": {"command": os.path.join(REPO, "bin", "moodle-mcp"),
                                      "env": {"MOODLE_URL": base_url, "MOODLE_TOKEN": tok,
                                              "MOODLE_DOWNLOAD_DIR": dl_dir, "MOODLE_LOG_LEVEL": "warn"}}}}
    cfg = os.path.join(tmp, f"mcp-{ev['name']}-{config}.json")
    with open(cfg, "w") as f:
        json.dump(mcp, f)
    cwd = workdir(tmp, f"wd-{ev['name']}-{config}", config == "with_skill")
    cmd = ["claude", "-p", ev["prompt"], "--mcp-config", cfg, "--strict-mcp-config",
           "--setting-sources", "project", "--no-session-persistence",
           "--output-format", "stream-json", "--verbose",
           "--allowedTools", *ALLOWED, "--add-dir", dl_dir]
    env = dict(os.environ, MOODLE_DOWNLOAD_DIR=dl_dir)
    start = time.time()
    p = subprocess.run(cmd, cwd=cwd, env=env, capture_output=True, text=True, timeout=900)
    dur = time.time() - start
    with open(os.path.join(run_dir, "transcript.jsonl"), "w") as f:
        f.write(p.stdout)
    answer, usage, skills, tools = "", {}, [], []
    for line in p.stdout.splitlines():
        try:
            m = json.loads(line)
        except json.JSONDecodeError:
            continue
        if m.get("type") == "result":
            answer = m.get("result", "")
            usage = m.get("usage", {})
        if m.get("type") == "assistant":
            for c in m.get("message", {}).get("content", []):
                if c.get("type") == "tool_use":
                    tools.append(c["name"])
                    if c["name"] == "Skill":
                        skills.append(c.get("input", {}).get("skill", ""))
    with open(os.path.join(out_dir, "answer.md"), "w") as f:
        f.write(answer or f"(no result; exit {p.returncode})\n\n{p.stderr[-2000:]}")
    for name in os.listdir(dl_dir):
        if name.endswith(".html"):
            shutil.copy(os.path.join(dl_dir, name), os.path.join(out_dir, name))
    tokens = sum(v for k, v in usage.items() if k.endswith("tokens") and isinstance(v, int))
    with open(os.path.join(run_dir, "timing.json"), "w") as f:
        json.dump({"total_tokens": tokens, "duration_ms": int(dur * 1000), "total_duration_seconds": round(dur, 1),
                   "skills_invoked": skills, "tool_calls": tools}, f, indent=2)
    return ev["name"], config, round(dur, 1), skills


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("iteration_dir")
    ap.add_argument("--only", default="")
    ap.add_argument("--jobs", type=int, default=4)
    args = ap.parse_args()
    evals = json.load(open(EVALS))["evals"]
    if args.only:
        keep = set(args.only.split(","))
        evals = [e for e in evals if e["name"] in keep]
    out_root = os.path.abspath(args.iteration_dir)
    os.makedirs(out_root, exist_ok=True)
    for ev in evals:
        d = os.path.join(out_root, ev["name"])
        os.makedirs(d, exist_ok=True)
        with open(os.path.join(d, "eval_metadata.json"), "w") as f:
            json.dump({"eval_id": ev["id"], "eval_name": ev["name"], "prompt": ev["prompt"],
                       "expected_output": ev["expected_output"], "assertions": []}, f, ensure_ascii=False, indent=2)

    subprocess.run(["make", "build"], cwd=REPO, check=True, stdout=subprocess.DEVNULL)
    tmp = tempfile.mkdtemp(prefix="moodle-skill-evals-")
    proc, base_url, token = start_fake(tmp)
    print(f"fake moodle at {base_url}; workdirs in {tmp}", flush=True)
    try:
        with cf.ThreadPoolExecutor(args.jobs) as pool:
            futs = [pool.submit(run_one, ev, cfg, base_url, token, tmp, out_root)
                    for ev in evals for cfg in ("with_skill", "without_skill")]
            for fut in cf.as_completed(futs):
                try:
                    print("done", *fut.result(), flush=True)
                except Exception as e:  # keep the other runs going
                    print("FAILED", repr(e), flush=True)
    finally:
        proc.kill()


if __name__ == "__main__":
    main()

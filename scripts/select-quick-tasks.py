#!/usr/bin/env python3
"""Pick the tasks for `deeporg eval bench --quick` and check how well they work.

The quick check runs a fixed handful of public-suite tasks once each. A task is
worth a slot when it separates strong runs from weak ones: one every run passes
(or every run fails) tells you nothing. This script measures that from past
per-task results and keeps the best tasks in every task family, so a family
that breaks still shows up.

Sources of per-task results (task ids survive suite refreezes):
  * published run pages under website/content/benchmarks/deeporg/runs/
  * local reports: any */.graphjin-evals/reports/*.json under the repo

For every task it computes, across runs that contain it:
  score = corr(task pass rate, run's full-pass recall without the task) * stdev(task pass rate)
The pass rate is the report's consistency (share of attempts that passed),
which is also what a single quick attempt is expected to score.

Usage:
  scripts/select-quick-tasks.py            # print the validation summary
  scripts/select-quick-tasks.py --write    # also rewrite cmd/benchmark/quick-tasks.json

Validation holds each model out in turn, picks tasks from the other models'
runs, and simulates one-attempt quick runs of the held-out model.
"""

import glob
import hashlib
import json
import os
import random
import re
import statistics as st
import sys
from collections import defaultdict

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SUITE_PATH = os.path.join(ROOT, "cmd/benchmark/public-suite.json")
OUT_PATH = os.path.join(ROOT, "cmd/benchmark/quick-tasks.json")

# Slots per family: roughly proportional to the family's share of the suite,
# at least two each, 25 in all.
ALLOC = {
    "aggregate": 4,
    "window": 4,
    "action": 3,
    "ranking": 3,
    "discovery": 3,
    "reactive": 3,
    "refusal": 3,
    "cross-source": 2,
}
MIN_RUNS = 15  # a task needs this many past runs before its score counts
MIN_OVERLAP = 40  # a run needs this many current-suite tasks to be used


def load_suite():
    with open(SUITE_PATH) as f:
        suite = json.load(f)
    return suite, {t["id"]: t["category"] for t in suite["tasks"]}


def published_runs():
    runs = {}
    for path in glob.glob(os.path.join(ROOT, "website/content/benchmarks/deeporg/runs/2*.md")):
        text = open(path).read()
        front = text.split("---")[1]
        meta = dict(re.findall(r"^(\w+): (.*)$", front, re.M))
        rows, header = {}, None
        for line in text.splitlines():
            if line.startswith("| Task ID |"):
                header = [c.strip() for c in line.strip().strip("|").split("|")]
                continue
            if header and line.startswith("| gjv1_"):
                cells = dict(zip(header, [c.strip() for c in line.strip().strip("|").split("|")]))
                rows[cells["Task ID"]] = {"pass": cells["Pass"] == "yes", "cons": float(cells["Consistency"])}
        runs[meta["run_id"]] = {"model": meta.get("model", ""), "rows": rows}
    return runs


def local_runs():
    runs = {}
    for dirpath, dirnames, filenames in os.walk(ROOT):
        if "node_modules" in dirpath or "/.git" in dirpath:
            dirnames[:] = []
            continue
        if not dirpath.endswith(".graphjin-evals/reports"):
            continue
        for name in filenames:
            if not name.endswith(".json"):
                continue
            try:
                report = json.load(open(os.path.join(dirpath, name)))
            except (OSError, ValueError):
                continue
            if report.get("run_status") != "complete" or not isinstance(report.get("tasks"), list):
                continue
            rows = {t["task_id"]: {"pass": t["pass"], "cons": t["consistency"]} for t in report["tasks"]}
            runs.setdefault(report["run_id"], {"model": report["provenance"].get("model", ""), "rows": rows})
    return runs


def dedupe(runs):
    # Copies of one run (rescores, copied state dirs) would count it twice.
    seen, out = set(), {}
    for rid, run in sorted(runs.items()):
        key = hashlib.sha1(json.dumps(sorted((t, r["cons"]) for t, r in run["rows"].items())).encode()).hexdigest()
        if key in seen:
            continue
        seen.add(key)
        out[rid] = run
    return out


def recall(run, cat, exclude=None):
    xs = [r["pass"] for t, r in run["rows"].items() if t in cat and t != exclude]
    return sum(xs) / len(xs)


def pick(runs, cat):
    scored = defaultdict(list)
    for task in cat:
        xs, ys = [], []
        for run in runs.values():
            if task in run["rows"]:
                xs.append(run["rows"][task]["cons"])
                ys.append(recall(run, cat, task))
        if len(xs) < MIN_RUNS or st.pstdev(xs) == 0 or st.pstdev(ys) == 0:
            score = -1.0
        else:
            score = st.correlation(xs, ys) * st.pstdev(xs)
        # Ties (tasks with no spread, such as refusals every model passes)
        # go to the best-measured task, so a family's last slot is a stable
        # canary rather than an arbitrary pick.
        scored[cat[task]].append((score, len(xs), task))
    chosen = []
    for family, slots in ALLOC.items():
        chosen += [t for _, _, t in sorted(scored[family], reverse=True)[:slots]]
    return chosen


def one_attempt(run, tasks, rng):
    xs = [1 if rng.random() < run["rows"][t]["cons"] else 0 for t in tasks if t in run["rows"]]
    return sum(xs) / len(xs)


def validate(runs, cat):
    rng = random.Random(1)
    points = []
    for model in sorted({r["model"] for r in runs.values()}):
        train = {k: r for k, r in runs.items() if r["model"] != model}
        chosen = pick(train, cat)
        for rid, run in runs.items():
            if run["model"] != model:
                continue
            covered = [t for t in chosen if t in run["rows"]]
            if len(covered) < 20:
                continue
            points.append((model, recall(run, cat), run, covered))
    print(f"held-out runs scored: {len(points)}")
    noise = [st.pstdev([one_attempt(p[2], p[3], rng) for _ in range(500)]) for p in points]
    print(f"one quick run moves by about ±{st.median(noise) * 100:.1f} points (median stdev)")
    for gap in (0.05, 0.10):
        agree = total = 0
        for i in range(len(points)):
            for j in range(i + 1, len(points)):
                a, b = points[i], points[j]
                if abs(a[1] - b[1]) < gap:
                    continue
                for _ in range(200):
                    total += 1
                    agree += (one_attempt(a[2], a[3], rng) - one_attempt(b[2], b[3], rng)) * (a[1] - b[1]) > 0
        if total:
            print(f"runs ≥{gap * 100:.0f} points apart on the full suite: quick orders them right {agree / total:.0%}")


def main():
    suite, cat = load_suite()
    runs = dedupe({**published_runs(), **local_runs()})
    runs = {k: r for k, r in runs.items() if sum(t in cat for t in r["rows"]) >= MIN_OVERLAP}
    print(f"runs used: {len(runs)} across {len({r['model'] for r in runs.values()})} models")
    validate(runs, cat)
    chosen = pick(runs, cat)
    print(f"selected {len(chosen)} tasks")
    if "--write" in sys.argv:
        order = {t["id"]: i for i, t in enumerate(suite["tasks"])}
        doc = {
            "suite": suite["name"],
            "repeats": 1,
            "selection": "scripts/select-quick-tasks.py: per family, the tasks whose past pass rate best tracks a run's full-suite score",
            "tasks": sorted(chosen, key=order.get),
        }
        with open(OUT_PATH, "w") as f:
            json.dump(doc, f, indent=2)
            f.write("\n")
        print(f"wrote {os.path.relpath(OUT_PATH, ROOT)}")


if __name__ == "__main__":
    main()

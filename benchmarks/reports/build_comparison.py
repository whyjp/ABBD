#!/usr/bin/env python3
"""Build reports/harness_comparison.{csv,html} from results/normalized/*.jsonl.

Every number in the output comes from a normalized row. With no rows the outputs say "no runs yet"
and contain no scores. Official scores and in-house metrics stay in separate columns and separate
sections; they are never summed or averaged together.

Normalized row (one JSON object per line):
  run_id, benchmark ("terminal-bench-2.0" | "swe-milestone" | "slop-code-bench" | "swe-ci" | "skillsbench"),
  task, group ("A".."F"), harness, harness_version, model, rep (int),
  status ("completed" | "timeout" | "agent_error" | "env_error" | "not_run"),
  official: {metric_name: value}      # exactly as the benchmark's own evaluator reports it
  in_house: {metric_name: value}      # our extra metrics (regression, kg hit rate, ...)
  regression (int | null), cost_usd, duration_s, input_tokens, output_tokens, tool_calls,
  failure_class (str | null), log_path, dataset_revision, harness_revision
"""
import csv
import glob
import html
import json
import os
import random
import sys
from collections import defaultdict

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)
REQUIRED = ("run_id", "benchmark", "task", "group", "harness", "model", "rep", "status", "official", "in_house")
STATUSES = {"completed", "timeout", "agent_error", "env_error", "not_run"}
TABLE_COLUMNS = ("Benchmark", "Task/Repo", "Harness", "Model", "Official Score", "Regression", "Cost", "Duration")


def load_rows(norm_dir):
    rows = []
    for path in sorted(glob.glob(os.path.join(norm_dir, "*.jsonl"))):
        for n, line in enumerate(open(path, encoding="utf-8"), 1):
            if not line.strip():
                continue
            row = json.loads(line)
            missing = [k for k in REQUIRED if k not in row]
            if missing or row["status"] not in STATUSES:
                raise SystemExit(f"{path}:{n} invalid row (missing {missing}, status {row.get('status')!r})")
            rows.append(row)
    return rows


def official_primary(row):
    """The benchmark's headline metric, by name, so the table never mixes metric kinds."""
    primary = {"terminal-bench-2.0": "resolved", "swe-milestone": "score", "slop-code-bench": "checkpoint_pass_rate",
               "swe-ci": "average_normalized_change", "skillsbench": "pass"}.get(row["benchmark"])
    return primary, row["official"].get(primary)


def bootstrap_ci(diffs, iters=10000, seed=0):
    if len(diffs) < 2:
        return None
    rng = random.Random(seed)
    means = sorted(sum(rng.choice(diffs) for _ in diffs) / len(diffs) for _ in range(iters))
    return means[int(0.025 * iters)], means[int(0.975 * iters) - 1]


def paired(rows, base_group, other_group):
    """Per (benchmark, task, model): mean primary metric over reps for each group, then the paired
    difference. Returns counts of improved / worsened / both solved / both failed and a bootstrap CI."""
    cell = defaultdict(list)
    for r in rows:
        name, v = official_primary(r)
        if r["status"] == "not_run" or v is None:
            continue
        cell[(r["benchmark"], r["task"], r["model"], r["group"])].append(float(v))
    keys = {(b, t, m) for (b, t, m, g) in cell}
    diffs, counts = [], {"improved": 0, "worsened": 0, "both_solved": 0, "both_failed": 0, "same_partial": 0}
    for b, t, m in sorted(keys):
        a, o = cell.get((b, t, m, base_group)), cell.get((b, t, m, other_group))
        if not a or not o:
            continue
        ma, mo = sum(a) / len(a), sum(o) / len(o)
        diffs.append(mo - ma)
        if mo > ma:
            counts["improved"] += 1
        elif mo < ma:
            counts["worsened"] += 1
        elif ma >= 1.0:
            counts["both_solved"] += 1
        elif ma <= 0.0:
            counts["both_failed"] += 1
        else:
            counts["same_partial"] += 1
    mean = sum(diffs) / len(diffs) if diffs else None
    return {"pairs": len(diffs), "mean_diff": mean, "ci95": bootstrap_ci(diffs), **counts}


def table_rows(rows):
    out = []
    for r in rows:
        name, v = official_primary(r)
        out.append({"Benchmark": r["benchmark"], "Task/Repo": r["task"], "Harness": f'{r["group"]}:{r["harness"]}',
                    "Model": r["model"], "Official Score": "" if v is None else f"{name}={v}",
                    "Regression": "" if r.get("regression") is None else r["regression"],
                    "Cost": "" if r.get("cost_usd") is None else round(r["cost_usd"], 4),
                    "Duration": "" if r.get("duration_s") is None else round(r["duration_s"], 1),
                    "run_id": r["run_id"], "status": r["status"], "log_path": r.get("log_path", ""),
                    "harness_revision": r.get("harness_revision", ""), "dataset_revision": r.get("dataset_revision", "")})
    return out


def write_csv(path, trows):
    cols = list(TABLE_COLUMNS) + ["run_id", "status", "log_path", "harness_revision", "dataset_revision"]
    with open(path, "w", newline="", encoding="utf-8") as f:
        w = csv.DictWriter(f, fieldnames=cols)
        w.writeheader()
        w.writerows(trows)


def section(title, body):
    return f"<section><h2>{html.escape(title)}</h2>{body}</section>"


def empty(note):
    return f'<p class="empty">{html.escape(note)}</p>'


def html_table(headers, data):
    head = "".join(f"<th>{html.escape(str(h))}</th>" for h in headers)
    body = "".join("<tr>" + "".join(f"<td>{html.escape(str(d.get(h, '')))}</td>" for h in headers) + "</tr>" for d in data)
    return f"<table><thead><tr>{head}</tr></thead><tbody>{body}</tbody></table>"


def write_html(path, rows, trows, leaderboard):
    none = "No runs yet. This section fills from results/normalized/*.jsonl."
    groups = sorted({r["group"] for r in rows})
    pairs = [{"comparison": f"A vs {g}", **paired(rows, "A", g)} for g in groups if g != "A"]
    for p in pairs:
        p["ci95"] = "" if p["ci95"] is None else f"[{p['ci95'][0]:.3f}, {p['ci95'][1]:.3f}]"
    in_house = [{"run_id": r["run_id"], "benchmark": r["benchmark"], "task": r["task"], "group": r["group"],
                 **{k: v for k, v in r["in_house"].items()}} for r in rows if r["in_house"]]
    ih_cols = ["run_id", "benchmark", "task", "group"] + sorted({k for d in in_house for k in d} - {"run_id", "benchmark", "task", "group"})
    efficiency = [{"run_id": r["run_id"], "group": r["group"], "cost_usd": r.get("cost_usd"), "duration_s": r.get("duration_s"),
                   "input_tokens": r.get("input_tokens"), "output_tokens": r.get("output_tokens"), "tool_calls": r.get("tool_calls"),
                   "official": official_primary(r)[1]} for r in rows]
    failures = defaultdict(int)
    for r in rows:
        if r.get("failure_class"):
            failures[(r["group"], r["failure_class"])] += 1
    fail_rows = [{"group": g, "failure_class": c, "count": n} for (g, c), n in sorted(failures.items())]
    sections = [
        section("1. Public leaderboard comparison (external, not our runs)",
                html_table(["benchmark", "harness", "version", "model", "score", "source_url", "fetched"], leaderboard) if leaderboard else empty("No verified external entries.")),
        section("2. Same-model success rate by group (official metric only)",
                html_table(list(TABLE_COLUMNS), trows) if trows else empty(none)),
        section("3. Paired comparison vs group A (official metric, bootstrap 95% CI over tasks)",
                html_table(["comparison", "pairs", "mean_diff", "ci95", "improved", "worsened", "both_solved", "both_failed", "same_partial"], pairs) if pairs else empty(none)),
        section("4. Cumulative stage accuracy, regression and architecture damage (in-house metrics)",
                html_table(ih_cols, in_house) if in_house else empty(none)),
        section("5. Tokens, time and cost against performance", html_table(list(efficiency[0].keys()), efficiency) if efficiency else empty(none)),
        section("6. SSOT / Graph component effect (groups C to F against their predecessor)",
                empty(none) if not pairs else html_table(["comparison", "pairs", "mean_diff", "ci95"], pairs)),
        section("7. Failure causes and trajectories", html_table(["group", "failure_class", "count"], fail_rows) if fail_rows else empty(none)),
    ]
    doc = f"""<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Harness Comparison</title><style>
:root{{--bg:#fff;--fg:#1a1a1a;--muted:#666;--line:#ddd}}
@media (prefers-color-scheme: dark){{:root{{--bg:#151515;--fg:#eee;--muted:#999;--line:#333}}}}
body{{background:var(--bg);color:var(--fg);font:14px/1.5 system-ui,sans-serif;margin:0 auto;max-width:1100px;padding:16px}}
table{{border-collapse:collapse;width:100%;display:block;overflow-x:auto}}td,th{{border:1px solid var(--line);padding:4px 8px;text-align:left}}
.empty{{color:var(--muted)}}</style></head><body><h1>Harness comparison</h1>
<p>{len(rows)} normalized run rows. Official scores and in-house metrics are reported separately.</p>
{''.join(sections)}</body></html>"""
    open(path, "w", encoding="utf-8").write(doc)


def main():
    rows = load_rows(os.path.join(ROOT, "results", "normalized"))
    lb_path = os.path.join(ROOT, "results", "external_leaderboard", "leaderboard.json")
    leaderboard = json.load(open(lb_path, encoding="utf-8"))["entries"] if os.path.exists(lb_path) else []
    trows = table_rows(rows)
    write_csv(os.path.join(HERE, "harness_comparison.csv"), trows)
    write_html(os.path.join(HERE, "harness_comparison.html"), rows, trows, leaderboard)
    print(f"{len(rows)} rows -> reports/harness_comparison.csv, reports/harness_comparison.html")


if __name__ == "__main__":
    sys.exit(main())

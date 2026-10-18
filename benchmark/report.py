#!/usr/bin/env python3
"""Summarize a benchmark run directory into report.md (PRD appendix A metrics this benchmark can
measure: feature correctness, rule retention, regressions, own-test health, cost, turns, time;
plus blueprint freshness for the abbd arm). With --reps N each arm gets mean [min, max] over its
N runs. Hidden test functions named TestHiddenTnnRule<Name> are rule checks; all others are
feature checks."""
import json
import os
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
SUITE_DIRS = {"v1": HERE, "v2": os.path.join(HERE, "v2")}
RULE_TEST = re.compile(r"^TestHidden(T\d+)Rule(\w+)$")


def load(out, name):
    p = os.path.join(out, name + ".json")
    return json.load(open(p, encoding="utf-8")) if os.path.exists(p) else None


def runs_of(out, meta, arm):
    """Runs of one arm: <arm>-r<k> when the run used reps, <arm> for older single-run directories."""
    names = [f"{arm}-r{k}" for k in range(1, meta["reps"] + 1)] if "reps" in meta else [arm]
    return [(n, rows) for n in names if (rows := load(out, n))]


def tests_at(row, task_id):
    h = row["hidden"][task_id]
    return h.get("tests") or {task_id: h["passed"]}


def rule_of(name):
    m = RULE_TEST.match(name)
    return m.group(2) if m else None


def ratio(passed, total):
    return passed / total if total else None


def metrics(rows):
    order = [r["task"] for r in rows]
    own_step = {tid: tests_at(rows[i], tid) for i, tid in enumerate(order)}
    final = {name: ok for tid in order for name, ok in tests_at(rows[-1], tid).items()}
    feat_first = [ok for t in own_step.values() for n, ok in t.items() if not rule_of(n)]
    rule_first = [ok for t in own_step.values() for n, ok in t.items() if rule_of(n)]
    feat_final = [ok for n, ok in final.items() if not rule_of(n)]
    rule_final = [ok for n, ok in final.items() if rule_of(n)]
    rule_curve, regressions, prev = [], [], {}
    for i, r in enumerate(rows):
        now = {name: ok for tid in order[: i + 1] for name, ok in tests_at(r, tid).items()}
        rules_now = [ok for n, ok in now.items() if rule_of(n)]
        rule_curve.append((sum(rules_now), len(rules_now)))
        regressions += [f"{n.removeprefix('TestHidden')} broke at {r['task']}" for n, ok in now.items() if prev.get(n) and not ok]
        prev = now
    applied = sum(t for _, t in rule_curve)
    per_rule = {}
    for n, ok in final.items():
        if rule := rule_of(n):
            p, t = per_rule.get(rule, (0, 0))
            per_rule[rule] = (p + ok, t + 1)
    return {
        "feature_first": ratio(sum(feat_first), len(feat_first)),
        "feature_final": ratio(sum(feat_final), len(feat_final)),
        "rule_first": ratio(sum(rule_first), len(rule_first)),
        "rule_cumulative": ratio(sum(p for p, _ in rule_curve), applied),
        "rule_final": ratio(sum(rule_final), len(rule_final)),
        "rule_curve": rule_curve,
        "per_rule": {k: ratio(p, t) for k, (p, t) in per_rule.items()},
        "per_rule_counts": per_rule,
        "regressions": regressions,
        "own_tests_green_steps": sum(1 for r in rows if r["own_tests_pass"]),
        "steps": len(rows),
        "errors": sum(1 for r in rows if r["session"].get("is_error")),
        "cost_usd": sum((r["session"].get("total_cost_usd") or 0) for r in rows),
        "turns": sum((r["session"].get("num_turns") or 0) for r in rows),
        "minutes": sum((r["session"].get("duration_ms") or 0) for r in rows) / 60000,
    }


def spread(values, pct=False, digits=2):
    vals = [v for v in values if v is not None]
    if not vals:
        return "n/a"
    f = (lambda v: f"{v * 100:.0f}%") if pct else (lambda v: f"{v:.{digits}f}")
    mean = sum(vals) / len(vals)
    return f(mean) if len(vals) == 1 else f"{f(mean)} [{f(min(vals))}, {f(max(vals))}]"


def main(out):
    meta = json.load(open(os.path.join(out, "meta.json"), encoding="utf-8"))
    suite = meta.get("suite", "v1")
    spec = json.load(open(os.path.join(SUITE_DIRS[suite], "tasks.json"), encoding="utf-8"))
    reps = meta.get("reps", 1)
    lines = [f"# ABBD benchmark {suite} · {os.path.basename(out)}", "",
             f"model `{meta['model']}` · budget/task ${meta['budget_per_task']} · abbd `{meta['abbd_commit']}` · reps {reps} · tasks {', '.join(meta['tasks'])}", "",
             "Each cell is the mean over reps, with [min, max] when there is more than one rep. Feature first-try counts a task's feature tests at its own step; "
             "rule first-try counts a task's rule tests at its own step (did the new path honor rules stated earlier); rule cumulative sums rule passes over "
             "every step's applicable rule tests; regressions count test functions that passed at one step and failed at the next.", "",
             "| arm | runs | feature first-try | feature final | rule first-try | rule cumulative | rule final | regressions | own tests green | session errors | cost USD | turns | minutes |",
             "|---|---|---|---|---|---|---|---|---|---|---|---|---|"]
    detail, rules_seen, per_arm_rules = [], [], {}
    for arm in meta["arms"]:
        runs = runs_of(out, meta, arm)
        if not runs:
            continue
        ms = [metrics(rows) for _, rows in runs]
        col = lambda k, **kw: spread([m[k] for m in ms], **kw)
        lines.append(f"| {arm} | {len(ms)} | {col('feature_first', pct=True)} | {col('feature_final', pct=True)} | {col('rule_first', pct=True)} | "
                     f"{col('rule_cumulative', pct=True)} | {col('rule_final', pct=True)} | {spread([len(m['regressions']) for m in ms], digits=1)} | "
                     f"{spread([m['own_tests_green_steps'] for m in ms], digits=1)}/{ms[0]['steps']} | {spread([m['errors'] for m in ms], digits=1)} | "
                     f"{col('cost_usd')} | {spread([m['turns'] for m in ms], digits=0)} | {col('minutes', digits=1)} |")
        per_arm_rules[arm] = ms
        for m in ms:
            rules_seen += [r for r in m["per_rule"] if r not in rules_seen]
        for (name, rows), m in zip(runs, ms):
            curve = ", ".join(f"{p}/{t}" for p, t in m["rule_curve"])
            detail.append(f"\n## {name}\n\n- rule tests passing after each task: {curve}\n- regressions: {', '.join(m['regressions']) or 'none'}")
            for r in rows:
                st = r["session"]
                fails = [n.removeprefix("TestHidden") for tid in r["hidden"] for n, ok in tests_at(r, tid).items() if not ok]
                extra = ""
                if r.get("abbd"):
                    a = r["abbd"]
                    extra = f" · blueprint {a.get('freshness')} receipts {a.get('receipts')} active {a.get('active_turn')}"
                detail.append(f"- {r['task']}: failing {', '.join(fails) or 'none'} · own tests {'green' if r['own_tests_pass'] else 'RED'} · "
                              f"${st.get('total_cost_usd')} · turns {st.get('num_turns')} · error {st.get('is_error')}{extra}")
    if rules_seen:
        rules = spec.get("rules", {})
        conflicts = {t["conflicts_with"]: t["id"] for t in spec["tasks"] if t.get("conflicts_with")}
        lines += ["", "## Rules (final step: rule tests passed / applicable)", "",
                  "| rule | stated in | " + " | ".join(per_arm_rules) + " |", "|---|---|" + "---|" * len(per_arm_rules)]
        for rule in rules_seen:
            stated = rules.get(rule, {}).get("introduced_by", "?") + (f", conflict {conflicts[rule]}" if rule in conflicts else "")
            cells = []
            for ms in per_arm_rules.values():
                counts = [m["per_rule_counts"].get(rule) for m in ms]
                total = next((c[1] for c in counts if c), 0)
                cells.append(f"{spread([m['per_rule'].get(rule) for m in ms], pct=True)} of {total}")
            lines.append(f"| {rule} | {stated} | " + " | ".join(cells) + " |")
        lines += [""] + [f"- **{k}**: {v['text']}" for k, v in rules.items()]
    lines += detail
    lines += ["", f"Caveats: {reps} run(s) per arm; with fewer than about five reps the [min, max] range is a sanity check, not a confidence interval. "
              "The arms share the host CLAUDE.md (--setting-sources does not turn it off). Hidden tests define correctness only for what they check. "
              "Treat differences as hypotheses for the next rung, not results."]
    open(os.path.join(out, "report.md"), "w", encoding="utf-8").write("\n".join(lines) + "\n")
    print("\n".join(lines))


if __name__ == "__main__":
    sys.stdout.reconfigure(encoding="utf-8")
    main(sys.argv[1])

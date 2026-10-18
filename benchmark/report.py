#!/usr/bin/env python3
"""Summarize a benchmark run directory into report.md (PRD appendix A metrics that this pilot
can measure: cumulative correctness, regression introduction, own-test health, cost, turns,
time; plus blueprint freshness for the abbd arm)."""
import json
import os
import sys


def load(out, arm):
    p = os.path.join(out, arm + ".json")
    return json.load(open(p, encoding="utf-8")) if os.path.exists(p) else None


def metrics(rows):
    passed_at_own_step = {}
    regressions = []
    for i, r in enumerate(rows):
        tid = r["task"]
        passed_at_own_step[tid] = r["hidden"][tid]["passed"]
        for prev, ok in passed_at_own_step.items():
            if prev != tid and ok and not r["hidden"][prev]["passed"]:
                regressions.append(f"{prev} broke at {tid}")
    final = rows[-1]["hidden"]
    cost = sum((r["session"].get("total_cost_usd") or 0) for r in rows)
    turns = sum((r["session"].get("num_turns") or 0) for r in rows)
    secs = sum((r["session"].get("duration_ms") or 0) for r in rows) / 1000
    curve = [sum(1 for h in r["hidden"].values() if h["passed"]) for r in rows]
    return {
        "tasks": len(rows),
        "first_try": sum(1 for v in passed_at_own_step.values() if v),
        "final_pass": sum(1 for h in final.values() if h["passed"]),
        "curve": curve,
        "regressions": sorted(set(regressions)),
        "own_tests_green_steps": sum(1 for r in rows if r["own_tests_pass"]),
        "errors": sum(1 for r in rows if r["session"].get("is_error")),
        "cost_usd": round(cost, 2),
        "turns": turns,
        "minutes": round(secs / 60, 1),
    }


def main(out):
    meta = json.load(open(os.path.join(out, "meta.json"), encoding="utf-8"))
    lines = [f"# ABBD benchmark pilot · {os.path.basename(out)}", "",
             f"model `{meta['model']}` · budget/task ${meta['budget_per_task']} · abbd `{meta['abbd_commit']}` · tasks {', '.join(meta['tasks'])}", "",
             "| arm | first-try pass | final pass | regressions | own tests green (steps) | session errors | cost USD | turns | minutes |",
             "|---|---|---|---|---|---|---|---|---|"]
    detail = []
    for arm in meta["arms"]:
        rows = load(out, arm)
        if not rows:
            continue
        m = metrics(rows)
        lines.append(f"| {arm} | {m['first_try']}/{m['tasks']} | {m['final_pass']}/{m['tasks']} | {len(m['regressions'])} | {m['own_tests_green_steps']}/{m['tasks']} | {m['errors']} | {m['cost_usd']} | {m['turns']} | {m['minutes']} |")
        detail.append(f"\n## {arm}\n\n- cumulative hidden passes after each task: {m['curve']}\n- regressions: {', '.join(m['regressions']) or 'none'}")
        for r in rows:
            st = r["session"]
            fails = [k for k, v in r["hidden"].items() if not v["passed"]]
            extra = ""
            if r.get("abbd"):
                a = r["abbd"]
                extra = f" · blueprint {a.get('freshness')} receipts {a.get('receipts')} active {a.get('active_turn')}"
            detail.append(f"- {r['task']}: failing {fails or '—'} · own tests {'green' if r['own_tests_pass'] else 'RED'} · ${st.get('total_cost_usd')} · turns {st.get('num_turns')} · error {st.get('is_error')}{extra}")
    lines += detail
    lines += ["", "Caveats: one run per arm (no repetition, no confidence interval); the arms share the host CLAUDE.md; hidden tests define correctness only for what they check. Treat differences as hypotheses for the next rung, not results."]
    open(os.path.join(out, "report.md"), "w", encoding="utf-8").write("\n".join(lines) + "\n")
    print("\n".join(lines))


if __name__ == "__main__":
    sys.stdout.reconfigure(encoding="utf-8")
    main(sys.argv[1])

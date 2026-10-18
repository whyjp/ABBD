#!/usr/bin/env python3
"""Cost and time estimate per track, group and phase. Every input is an assumption stated below;
change it here and re-run. Output is a Markdown table for feasibility_and_cost.md.

Measured anchor (docs/benchmark-results-v2, 12 small Go tasks x 2 reps, alias `sonnet`):
  baseline $0.131/task, pstack $0.169/task, abbd $0.160/task; ~27 s of session per task.
Everything else is a guess, labelled as such in the report.
"""
import json

PRICE_RATIO = {"claude-sonnet-5-5": 1.0, "claude-opus-5-5": 2.0}  # $2/$10 vs $4/$20 per MTok, same token use assumed

# Group overhead on group A cost, from v2 means: pstack 2.03/1.57, abbd 1.92/1.57.
# C and D are partial ABBD arms; assumed equal to the full abbd overhead (conservative).
GROUP_MULT = {"A": 1.00, "B": 1.29, "C": 1.22, "D": 1.22, "E": 1.22}

# Unit cost for group A on Sonnet 5.5, (low, mid, high) USD. Guesses scaled from v2's
# ~$0.29 per agent-minute and each benchmark's official timeouts.
TRACKS = {
    "terminal-bench-2.0": {"unit": "task", "cost": (1.0, 2.5, 6.0), "minutes": 12, "concurrency": 4,
                           "pilot": 15, "full": 89},
    "swe-milestone": {"unit": "milestone", "cost": (1.5, 3.0, 6.0), "minutes": 15, "concurrency": 2,
                      "pilot": 23 + 18, "full": 23 + 18 + 12 + 13},
    "slop-code-bench": {"unit": "checkpoint", "cost": (0.5, 1.5, 3.0), "minutes": 6, "concurrency": 4,
                        "pilot": 6 * 8 + 6 + 4, "full": 196},
}
PHASES = {"pilot": 1, "full": 3}  # reps


def estimate(model="claude-sonnet-5-5", groups=("A", "B", "C", "D", "E")):
    rows = []
    for track, t in TRACKS.items():
        for phase, reps in PHASES.items():
            units = t[phase]
            per_group = [tuple(c * GROUP_MULT[g] * PRICE_RATIO[model] for c in t["cost"]) for g in groups]
            low, mid, high = (sum(units * reps * pg[i] for pg in per_group) for i in range(3))
            hours = units * reps * len(groups) * t["minutes"] / 60 / t["concurrency"]
            rows.append({"track": track, "phase": phase, "units": f"{units} {t['unit']}s", "reps": reps,
                         "groups": "".join(groups), "model": model, "usd_low": round(low), "usd_mid": round(mid),
                         "usd_high": round(high), "wall_hours_mid": round(hours, 1)})
    return rows


def markdown(rows):
    cols = list(rows[0])
    out = ["| " + " | ".join(cols) + " |", "|" + "---|" * len(cols)]
    out += ["| " + " | ".join(str(r[c]) for c in cols) + " |" for r in rows]
    return "\n".join(out)


if __name__ == "__main__":
    rows = estimate() + estimate("claude-opus-5-5")
    print(markdown(rows))
    tot = {m: {p: sum(r["usd_mid"] for r in rows if r["model"] == m and r["phase"] == p) for p in PHASES} for m in PRICE_RATIO}
    print("\nmid totals:", json.dumps(tot))

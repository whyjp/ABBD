"""Measure the layout complaints the user keeps repeating, one exact case each.

    python eval/lint/accept.py OUT [--no-hover] [--record]

OUT is a scratch directory. One go test run writes there the per-view table
(ABBD_LINT_FIXTURES), every breach (ABBD_LINT_BREACHES), every traced line
with its turning points (ABBD_ACCEPT_WAYS, TestAcceptWays) and every fixture
page (ABBD_BLUEPRINTS); then, unless --no-hover, shoot.py hovers every page
in Chrome and writes OUT/shots/findings.json. The tables are read even when
the fixture gate fails, so a hillclimb trial is measured either way.

It prints one row per case: case, source, view, measure, value, target and
pass when the value is at or under the target. A case comes from a user
screenshot in D:/abbd-eval/exemplars (user-NN) or from the day's review
(today); its view is a fixture view, by the name TestLintFixtures gives it.
--record writes eval/lint/acceptance.tsv with the values as the main column.
"""
import csv
import json
import os
import subprocess
import sys
from collections import namedtuple
from itertools import combinations

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))

RG = "ripgrep overview"
R72 = "orca MOD-0072 desktop renderer UI"
F73 = "orca MOD-0072 focus MOD-0073 renderer/app-shell"
S98 = "orca MOD-0098 src/"
GREP, REGEX, MATCHER = 'box "grep" (MOD-0006)', 'box "grep-regex" (MOD-0011)', 'box "grep-matcher" (MOD-0008)'
MOBILE, NATIVE = 'chip "← mobile/"', 'chip "← native/"'
HOST = 'box "@orca/process-host" (MOD-0070)'
CONNECTOR = 'chip "@orca-cloud/connector- contract"'
GITX = 'box "internal/gitx" (MOD-0022)'
SAME_CLEAR = "(same column, the straight drop is clear)"


def read(path):
    with open(path, newline="", encoding="utf8") as f:
        return list(csv.DictReader(f, delimiter="\t", quoting=csv.QUOTE_NONE))


class Tables:
    def __init__(self, out, hover):
        self.views = {r["view"]: r for r in read(os.path.join(out, "fix.tsv"))}
        self.breaches = read(os.path.join(out, "breaches.tsv"))
        self.ways = read(os.path.join(out, "ways.tsv"))
        for w in self.ways:
            w["pts"] = [tuple(float(c) for c in p.split(",")) for p in w["points"].split()]
        self.hover = None
        if hover:
            with open(os.path.join(out, "shots", "findings.json"), encoding="utf8") as f:
                self.hover = json.load(f)

    def view(self, name):
        if name not in self.views:
            sys.exit(f"no view {name!r} in the lint table")
        return name


# Each measure takes the tables and the case's view and returns an int.

def breaches(criteria, *names, why=""):
    """Breaches of these criteria on the view whose text names every one of names."""
    def m(t, view):
        return sum(1 for b in t.breaches if b["view"] == t.view(view) and b["criterion"] in criteria.split()
                   and all(n in b["text"] for n in names) and why in b["text"])
    return m


def column(col):
    def m(t, view):
        return int(t.views[t.view(view)][col])
    return m


def ways(t, view, frm, to=None, kind=None):
    got = [w for w in t.ways if w["view"] == t.view(view) and w["from"] == frm
           and (to is None or w["to"] == to) and (kind is None or w["kind"] == kind)]
    if not got:
        sys.exit(f"no traced line from {frm} to {to} on {view}")
    return got


def past_ends(frm, to):
    """Pixels the line runs left or right past the x span of its two ends."""
    def m(t, view):
        n = 0
        for w in ways(t, view, frm, to):
            xs = [p[0] for p in w["pts"]]
            lo, hi = sorted((xs[0], xs[-1]))
            n = max(n, round(max(0, max(xs) - hi) + max(0, lo - min(xs))))
        return n
    return m


def x_travel(frm, to):
    """Total length of the line's sideways stretches."""
    def m(t, view):
        w = ways(t, view, frm, to)[0]["pts"]
        return round(sum(abs(b[0] - a[0]) for a, b in zip(w, w[1:])))
    return m


def stems(frm, kind):
    """Distinct x's at which the place's lines of this kind leave it."""
    def m(t, view):
        return len({w["pts"][0][0] for w in ways(t, view, frm, kind=kind)})
    return m


def beside(frm, within=16):
    """Pixels two vertical stretches of the place's lines, at different x's no
    more than within apart, run beside each other."""
    def m(t, view):
        runs = set()
        for w in ways(t, view, frm):
            for a, b in zip(w["pts"], w["pts"][1:]):
                if a[0] == b[0]:
                    runs.add((a[0], min(a[1], b[1]), max(a[1], b[1])))
        n = 0
        for (x1, a1, b1), (x2, a2, b2) in combinations(sorted(runs), 2):
            if 0 < abs(x1 - x2) <= within:
                n += max(0, min(b1, b2) - max(a1, a2))
        return round(n)
    return m


def hover(key):
    """Findings of this kind shoot.py records over every fixture page it hovers."""
    def m(t, view):
        if t.hover is None:
            return None
        return sum(len(r[key]) for r in t.hover)
    return m


Case = namedtuple("Case", "id source view measure fn target note")

CASES = [
    Case("rg-regex-drop", "today; user-16, user-17", RG, "B1 breaches naming grep and grep-regex",
         breaches("B1", GREP, REGEX), 0, "rows 67-70: the hairpin is gone, but grep-regex stands in grep's column and the line still turns twice"),
    Case("rg-regex-hairpin", "today; user-16, user-17", RG, "B3 and B4 breaches naming grep and grep-regex",
         breaches("B3 B4", GREP, REGEX), 0, "guard: rows 69-70 took the ride past pcre2 cargo feature's drop and back"),
    Case("rg-doubled", "user-18", RG, "B4 breaches", breaches("B4"), 0, "guard: two lanes of one line read as one doubled line"),
    Case("rg-matcher-out", "today", RG, "px grep -> grep-matcher runs past its ends' x span",
         past_ends(GREP, MATCHER), 0, "row 71: it passes at x=769 to reach 337 from 428"),
    Case("rg-matcher-out-ripgrep", "today", RG, "px ripgrep -> grep-matcher runs past its ends' x span",
         past_ends('box "ripgrep" (MOD-0001)', MATCHER), 0, "the same bus as grep's line, from 578"),
    Case("rg-matcher-back", "today", RG, "B3 breaches naming grep-matcher",
         breaches("B3", MATCHER), 0, "grep-matcher's arrivals ride around a row and back"),
    Case("r72-straight", "today", R72, "B1 breaches, same column with a clear straight drop",
         breaches("B1", why=SAME_CLEAR), 0, "a line that could drop straight turns"),
    Case("r72-back", "today", R72, "B3 breaches", breaches("B3"), 0, "app-shell's lines ride 90px past their turn"),
    Case("r72-jogs", "today", R72, "jogs", column("jogs"), 0, "stretches under 24px between turns"),
    Case("r72-steps", "today", R72, "steps", column("steps"), 0, "jogs plus turns past 2 per line"),
    Case("f73-one-stem", "today", F73, "distinct x's app-shell's depends_on lines leave it at",
         stems('box "app-shell" (MOD-0073)', "depends_on"), 1, "5 children in one row: x3/1/1 stubs at 493, 516, 586; one stem that forks"),
    Case("s98-mobile", "user-11, user-12, user-15", S98, "B1, B2 and B3 breaches naming chip ← mobile/",
         breaches("B1 B2 B3", MOBILE), 0, "the mobile/ entry's lines stair down through the middle"),
    Case("s98-mobile-beside", "user-13, user-14", S98, "px two stems from chip ← mobile/ run beside each other within 16px",
         beside(MOBILE), 0, "the x2 and x5 stems run as two parallel verticals; one stem, or apart"),
    Case("s98-native", "user-11, user-12", S98, "B1, B2 and B3 breaches naming chip ← native/",
         breaches("B1 B2 B3", NATIVE), 0, "guard: the native/ entry now drops straight into desktop main process"),
    Case("s98-host-jogs", "user-15", S98, "B2 breaches naming @orca/process-host",
         breaches("B2", HOST), 0, "process-host's arrivals step in small jogs above it"),
    Case("s98-connector", "user-02, user-03 (MOD-0100, not a fixture)", S98,
         "B1, B2 and B3 breaches naming chip @orca-cloud/connector- contract",
         breaches("B1 B2 B3", CONNECTOR), 0, "MOD-0100's stairs down to the frame's bottom chips; MOD-0098 has the same chips"),
    Case("n97-exit", "user-04", "orca MOD-0097 native/", "px windows-agent-hook -> chip → src/ runs sideways",
         x_travel('box "windows-agent-hook" (MOD-0034)', 'chip "→ src/"'), 0,
         "guard: the exit ran left under process-footprint-macos, reading as its line"),
    Case("self-gitx", "user-05", "self overview", "B1, B2 and B3 breaches naming internal/gitx",
         breaches("B1 B2 B3", GITX), 0, "the knot of steps between internal/extract, internal/store and internal/gitx"),
    Case("hover-cut", "today", "every shot page", "shoot.py cut", hover("cut"), 0, "guard: a lit route stops partway"),
    Case("hover-stray", "today", "every shot page", "shoot.py stray", hover("stray"), 0, "guard: a lit box no lit line reaches"),
    Case("hover-mixed", "today", "every shot page", "shoot.py mixed", hover("mixed"), 0, "guard: one lit route of two colours"),
    Case("hover-overlap", "today", "every shot page", "shoot.py overlap", hover("overlap"), 0,
         "two colours along one track, one painting over the other"),
]


def run(out, hover_on):
    os.makedirs(out, exist_ok=True)
    pages = os.path.join(out, "pages")
    env = dict(os.environ, ABBD_LINT_FIXTURES=os.path.join(out, "fix.tsv"), ABBD_LINT_BREACHES=os.path.join(out, "breaches.tsv"),
               ABBD_ACCEPT_WAYS=os.path.join(out, "ways.tsv"), ABBD_BLUEPRINTS=pages)
    go = subprocess.run(["go", "test", "./internal/screen", "-count=1", "-run", "^(TestLintFixtures|TestAcceptWays|TestEveryRealDrawingIsLegible)$"],
                        cwd=ROOT, env=env, capture_output=True, text=True)
    if go.returncode:
        print(f"go test failed (tables still read):\n{go.stdout[-2000:]}", file=sys.stderr)
    if hover_on:
        shoot = os.path.join(ROOT, "eval", "skill-polish", "tools", "shoot.py")
        repos = [f"{r}={os.path.join(pages, r)}" for r in sorted(os.listdir(pages))]
        subprocess.run([sys.executable, shoot, os.path.join(out, "shots"), *repos, "--pages", "all", "--hover-shots", "0"],
                       check=True, stdout=subprocess.DEVNULL)


def main():
    args = sys.argv[1:]
    if not args or args[0].startswith("-"):
        sys.exit(__doc__)
    sys.stdout.reconfigure(encoding="utf8")
    out, hover_on, record = args[0], "--no-hover" not in args, "--record" in args
    run(out, hover_on)
    t = Tables(out, hover_on)
    rows, fails = [], 0
    print("case\tsource\tview\tmeasure\tvalue\ttarget\tresult")
    for c in CASES:
        v = c.fn(t, c.view)
        result = "skip" if v is None else "pass" if v <= c.target else "fail"
        fails += result == "fail"
        print(f"{c.id}\t{c.source}\t{c.view}\t{c.measure}\t{'' if v is None else v}\t<= {c.target}\t{result}")
        rows.append([c.id, c.source, c.view, c.measure, "" if v is None else str(v), f"<= {c.target}", c.note])
    print(f"\n{fails} of {len(CASES)} cases fail")
    if record:
        with open(os.path.join(HERE, "acceptance.tsv"), "w", newline="", encoding="utf8") as f:
            f.write("case\tsource\tview\tmeasure\tmain\ttarget\tnote\n")
            f.writelines("\t".join(r) + "\n" for r in rows)


if __name__ == "__main__":
    main()

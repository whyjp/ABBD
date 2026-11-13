"""Measure the layout complaints the user keeps repeating, one exact case each.

    python eval/lint/accept.py OUT [--no-hover] [--main DIR] [--record]

OUT is a scratch directory. One go test run writes there the per-view table
(ABBD_LINT_FIXTURES), every traced line with its turning points and its two
ends' boxes (ABBD_ACCEPT_WAYS, TestAcceptWays) and every fixture page
(ABBD_BLUEPRINTS); then, unless --no-hover, shoot.py hovers every page in
Chrome and writes OUT/shots/findings.json. The tables are read even when the
fixture gate fails, so a hillclimb trial is measured either way.

Every case measures the traced geometry itself, never the lint's breaches,
so a change to a lint criterion cannot move a case.

It prints one row per case: case, source, view, measure, value, main's
value, target and pass when the value is at or under the target, and exits 1 when a
case fails. A case whose view, page or line is not drawn fails with that
reason in its result, and the other cases are still measured. When go test
writes no tables, as when the build fails, it stops with go's output before
measuring anything. A case comes from a user
screenshot in D:/abbd-eval/exemplars (user-NN) or from the day's review
(today); its view is a fixture view, by the name TestLintFixtures gives it,
or every fixture view for a total.
--main DIR measures the checkout at DIR too, into OUT/main, with this
script's cases, and prints its values in a main column. --record writes
eval/lint/acceptance.tsv with the values as the recorded column, the commit
they were measured on (git describe --dirty), and, with --main, DIR's values
and commit; a case that skips keeps the values it had.

The arrowhead cases read the drawn map off the view's rendered page: each
route's way and its #t- chevrons, so a head at either end of a mutual line
counts. The zone title cases read the overview pages, and this repository's
own state, rendered into OUT/self-pages (TestAcceptSelfPages), since only it
stores the stack whose numbered zones the fixtures lack; a checkout without
that test skips the case.

The busy cases measure how a drawing reads at rest, before a hover tells
its lines apart, as lint's parallel_ink, segments, lines and fork_levels
columns do from the same geometry: ink of two drawn paths running side by
side, turns per line and the tracks one fork splits off. Their targets
come from the user's drawings user-07 and user-08, counted by eye.
"""
import csv
import html
import json
import os
import re
import subprocess
import sys
from collections import namedtuple
from itertools import combinations

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))

RG = "ripgrep overview"
RG1 = "ripgrep overview focus MOD-0001 ripgrep"
RG6 = "ripgrep overview focus MOD-0006 grep"
R72 = "orca MOD-0072 desktop renderer UI"
F73 = "orca MOD-0072 focus MOD-0073 renderer/app-shell"
S98 = "orca MOD-0098 src/"
GREP, REGEX, MATCHER = 'box "grep" (MOD-0006)', 'box "grep-regex" (MOD-0011)', 'box "grep-matcher" (MOD-0008)'
MOBILE, NATIVE = 'chip "← mobile/"', 'chip "← native/"'
HOST = 'box "@orca/process-host" (MOD-0070)'
CONNECTOR = 'chip "@orca-cloud/connector- contract"'
GITX = 'box "internal/gitx" (MOD-0022)'
APP_SHELL = 'box "app-shell" (MOD-0073)'
M81 = "orca MOD-0081 shared types and schemas"
RPC = 'box "shared/rpc-contract" (MOD-0090)'


class Unmeasured(Exception):
    """A case's view, page or line is not there, so that case fails with this
    reason and the others are still measured."""


def read(path):
    with open(path, newline="", encoding="utf8") as f:
        return list(csv.DictReader(f, delimiter="\t", quoting=csv.QUOTE_NONE))


class Tables:
    def __init__(self, out, hover):
        self.views = {r["view"]: r for r in read(os.path.join(out, "fix.tsv"))}
        self.ways = read(os.path.join(out, "ways.tsv"))
        for w in self.ways:
            w["pts"] = [tuple(float(c) for c in p.split(",")) for p in w["points"].split()]
            w["rects"] = [tuple(float(c) for c in w[k].split(",")) if w[k] else None for k in ("from_rect", "to_rect")]
        self.out, self.pages = out, os.path.join(out, "pages")
        self.hover = None
        if hover:
            with open(os.path.join(out, "shots", "findings.json"), encoding="utf8") as f:
                self.hover = json.load(f)

    def view(self, name):
        if name not in self.views:
            raise Unmeasured(f"no view {name!r} in the lint table")
        return name


# Each measure takes the tables and the case's view and returns an int.

def stretches(w):
    return list(zip(w["pts"], w["pts"][1:]))


def length(a, b):
    return abs(b[0] - a[0]) + abs(b[1] - a[1])


def turns(w):
    return len(w["pts"]) - 2


def jogs(w):
    """Stretches between two turns shorter than 24px."""
    return sum(1 for a, b in stretches(w)[1:-1] if length(a, b) < 24)


def heading(a, b):
    return (b[0] > a[0]) - (b[0] < a[0]), (b[1] > a[1]) - (b[1] < a[1])


def backs(w):
    """Stretches that run against the stretch two before them."""
    s = stretches(w)
    return sum(1 for i in range(2, len(s)) if heading(*s[i]) == tuple(-c for c in heading(*s[i - 2])))


def doubles(w):
    """Pairs of the line's stretches that run side by side within 12px for 24px or more."""
    s, n = stretches(w), 0
    for i, j in combinations(range(len(s)), 2):
        (a1, b1), (a2, b2) = s[i], s[j]
        for ax in (0, 1):
            if j > i + 1 and a1[ax] == b1[ax] and a2[ax] == b2[ax] and abs(a1[ax] - a2[ax]) <= 12:
                o = 1 - ax
                n += min(max(a1[o], b1[o]), max(a2[o], b2[o])) - max(min(a1[o], b1[o]), min(a2[o], b2[o])) >= 24
    return n


def shares_column(w):
    """The two ends' boxes overlap in x, so the line could drop with no corner."""
    a, b = w["rects"]
    return a is not None and b is not None and max(a[0], b[0]) < min(a[0] + a[2], b[0] + b[2])


def excess(w):
    """Turns past 0 for ends in one column, past 2 for ends in two."""
    return max(0, turns(w) - (0 if shares_column(w) else 2))


def knots(w):
    return excess(w) + jogs(w) + backs(w)


def steps(w):
    return jogs(w) + max(0, turns(w) - 2)


def on(fn, *names):
    """Sum fn over the view's lines that have every one of names at an end."""
    def m(t, view):
        got = [w for w in t.ways if w["view"] == t.view(view) and all(n in (w["from"], w["to"]) for n in names)]
        if names and not got:
            raise Unmeasured(f"no traced line at {' and '.join(names)} on {view}")
        return sum(fn(w) for w in got)
    return m


def ways(t, view, frm, to=None, kind=None):
    got = [w for w in t.ways if w["view"] == t.view(view) and w["from"] == frm
           and (to is None or w["to"] == to) and (kind is None or w["kind"] == kind)]
    if not got:
        raise Unmeasured(f"no traced line from {frm} to {to} on {view}")
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


def x_travel(frm, to, *more):
    """Total length of the sideways stretches of the line from frm to to, and
    of the line from each place in more to to."""
    def m(t, view):
        return round(sum(abs(b[0] - a[0]) for f in (frm, *more) for w in [ways(t, view, f, to)[0]["pts"]]
                         for a, b in zip(w, w[1:])))
    return m


def x_slack(frm, *tos):
    """Pixels the lines from frm to each of tos run sideways past the x span
    between their two ends."""
    def m(t, view):
        return round(sum(sum(abs(b[0] - a[0]) for a, b in zip(w, w[1:])) - abs(w[-1][0] - w[0][0])
                         for to in tos for w in [ways(t, view, frm, to)[0]["pts"]]))
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


def tracks(frm, kind):
    """Distinct y's of the flat stretch the place's lines of this kind turn onto."""
    def m(t, view):
        return len({w["pts"][1][1] for w in ways(t, view, frm, kind=kind) if turns(w) > 0})
    return m


def tees(t, view):
    """Points where a line's straight drop crosses, at no turn of the other
    line, a flat stretch of another line leaving the same place."""
    got = [w for w in t.ways if w["view"] == t.view(view)]
    seen = set()
    for a in got:
        if len(a["pts"]) != 2 or a["pts"][0][0] != a["pts"][1][0]:
            continue
        x, (y0, y1) = a["pts"][0][0], sorted(p[1] for p in a["pts"])
        for b in got:
            if b is a or b["from"] != a["from"]:
                continue
            for p, q in zip(b["pts"], b["pts"][1:]):
                if p[1] == q[1] and y0 < p[1] < y1 and min(p[0], q[0]) < x < max(p[0], q[0]):
                    seen.add((x, p[1]))
    return len(seen)


WIRE = re.compile(r'<path class="w ([^" ]+)[^"]*" d="([^"]*)"([^>]*)>(?:<title>([^<]*)</title>)?')


def ink(d):
    """The straight stretches of a path's data."""
    out, at = [], None
    for c, x, y in re.findall(r"([ML])\s*(-?[\d.]+)[ ,](-?[\d.]+)", d):
        p = (float(x), float(y))
        if c == "L" and at:
            out.append((at, p))
        at = p
    return out


def along(s, t):
    """How far two straight stretches run on one another."""
    (a, b), (c, d) = s, t
    for ax in (0, 1):
        if a[ax] == b[ax] == c[ax] == d[ax]:
            o = 1 - ax
            return max(0, min(max(a[o], b[o]), max(c[o], d[o])) - max(min(a[o], b[o]), min(c[o], d[o])))
    return 0


def listing(path):
    if not os.path.isdir(path):
        raise Unmeasured(f"no pages in {path}")
    return sorted(os.listdir(path))


def page_text(path):
    try:
        with open(path, encoding="utf8") as f:
            return f.read()
    except FileNotFoundError:
        raise Unmeasured(f"no page {path}") from None


def section(page, tag, path):
    if tag not in page:
        raise Unmeasured(f"no {tag} in {path}")
    return page.index(tag)


def doubled(fixture, unit):
    """Pairs of wires of one kind, not of one bundle (data-net), whose ink
    runs on one another, where neither names every line of the other, so
    the one drawn on top hides the other's lines from the pointer."""
    def m(t, view):
        t.view(view)
        path = os.path.join(t.pages, fixture, unit + ".html")
        page = page_text(path)
        at = section(page, f'id="m-{unit}"', path)
        ws = []
        for kind, d, rest, title in WIRE.findall(page[at:page.index("</section>", at)]):
            net = re.search(r'data-net="(\d+)"', rest)
            ws.append((kind, net and net[1], ink(d), set(html.unescape(title).split("\n"))))
        return sum(1 for a, b in combinations(ws, 2)
                   if a[0] == b[0] and (a[1] is None or a[1] != b[1]) and not (a[3] <= b[3] or b[3] <= a[3])
                   and any(along(s, q) > 0 for s in a[2] for q in b[2]))
    return m


EVERY = "every fixture view"
DRAWN = {}


def page_map(t, view):
    """The drawn map of a fixture view, read off its rendered page: overview is
    section overview, "U name" is m-U and "A focus B name" is f-A--B."""
    t.view(view)
    fixture, rest = view.split(" ", 1)
    words = rest.split()
    sid = "overview" if rest == "overview" else f"f-{words[0]}--{words[2]}" if words[1:2] == ["focus"] else f"m-{words[0]}"
    key = (t.pages, fixture, sid)
    if key not in DRAWN:
        tag = f'id="{sid}"'
        for name in listing(os.path.join(t.pages, fixture)):
            with open(os.path.join(t.pages, fixture, name), encoding="utf8") as f:
                page = f.read()
            if tag in page:
                at = page.index("<svg", page.index(tag))
                DRAWN[key] = drawn(page[at:page.index("</svg>", at)])
                break
        else:
            raise Unmeasured(f"no section {sid} for {view!r} in {t.pages}")
    return DRAWN[key]


Drawn = namedtuple("Drawn", "routes wires dots badges texts chips")
CHEVRON_DEPTH, CHEVRON_HALF = 7.2, 3.6
NUM = r"(-?[\d.]+)"


def tokens(cls, p):
    return frozenset(c for c in cls.split() if re.fullmatch(p + r"\d+", c))


def drawn(svg):
    """Routes as (r ids, e ids, turning points, heads), a head being the tip and
    unit heading of a #t- chevron; wires as (r ids, stretches); junction dots
    as (e ids, centre, radius); hop badges as (r ids, centre, radius); label,
    count and zone title text as x,y,w,h boxes, and chips as (p ids, box), a
    chip's p id naming the place a route's e id names."""
    routes = []
    for cls, body in re.findall(r'<g class="route ([^"]*)"[^>]*>(.*?)</g>', svg):
        d = re.search(r'<path class="way" d="([^"]*)"', body)[1]
        pts = [(float(x), float(y)) for x, y in re.findall(NUM + r"[ ,]" + NUM, d)]
        heads = [((float(x), float(y)), (0.0, 1.0 if r == "90" else -1.0))
                 for x, y, r in re.findall(r'<use href="#t-\w+" transform="translate\(' + NUM + " " + NUM + r"\) rotate\(" + NUM + r"\)", body)]
        routes.append((tokens(cls, "r"), tokens(cls, "e"), pts, heads))
    wires = [(tokens(cls, "r"), ink(d)) for cls, d in re.findall(r'<path class="(w [^"]*)" d="([^"]*)"', svg)]
    disc = '<circle cx="' + NUM + '" cy="' + NUM + '" r="' + NUM + '"'
    dots = [(tokens(cls, "e"), (float(x), float(y)), float(r))
            for cls, x, y, r in re.findall(r'<circle class="(junction[^"]*)"[^>]*cx="' + NUM + '" cy="' + NUM + '" r="' + NUM + '"', svg)]
    badges = [(tokens(cls, "r"), (float(x), float(y)), float(r)) for cls, x, y, r in re.findall(r'<g class="badge ([^"]*)">' + disc, svg)]
    texts = []
    for attrs, words in re.findall(r'<text class="(?:lab|count|zt)[^"]*"([^>]*)>([^<]*)<', svg):
        x, y = float(re.search(r'x="' + NUM, attrs)[1]), float(re.search(r'y="' + NUM, attrs)[1])
        w = sum(11 if ord(ch) > 0x2e80 else 6.6 for ch in html.unescape(words))
        texts.append((x - w / 2 if 'text-anchor="middle"' in attrs else x, y - 8.8, w, 11))
    chips = [(tokens(cls, "p"), tuple(float(v) for v in r)) for cls, *r in re.findall(
        r'<g class="(port[^"]*)"[^>]*>(?:<title>[^<]*</title>)?<rect x="' + NUM + '" y="' + NUM + '" width="' + NUM + '" height="' + NUM + '"', svg)]
    return Drawn(routes, wires, dots, badges, texts, chips)


def views_of(t, view):
    return [v for v in t.views if v in {w["view"] for w in t.ways}] if view == EVERY else [view]


def straight_ins(t, view):
    """{tip: run} for every drawn arrowhead: the length of the straight stretch
    that ends in the tip, the shortest when routes share the head."""
    runs = {}
    for _, _, pts, heads in page_map(t, view).routes:
        for tip, _ in heads:
            run = length(*(pts[-2:] if pts[-1] == tip else pts[:2]))
            runs[tip] = min(run, runs.get(tip, run))
    return runs


def bent_heads(t, view):
    """Arrowheads whose line turns closer to the tip than the chevron's depth
    plus 12px."""
    return sum(1 for v in views_of(t, view) for run in straight_ins(t, v).values() if run < CHEVRON_DEPTH + 12)


def seg_point(a, b, p):
    (ax, ay), (bx, by), (px, py) = a, b, p
    dx, dy = bx - ax, by - ay
    k = 0 if dx == dy == 0 else max(0, min(1, ((px - ax) * dx + (py - ay) * dy) / (dx * dx + dy * dy)))
    return ((ax + k * dx - px) ** 2 + (ay + k * dy - py) ** 2) ** 0.5


def seg_seg(s, q):
    (a, b), (c, d) = s, q

    def side(p, u, v):
        return (u[0] - p[0]) * (v[1] - p[1]) - (u[1] - p[1]) * (v[0] - p[0])
    if side(a, b, c) * side(a, b, d) < 0 and side(c, d, a) * side(c, d, b) < 0:
        return 0
    return min(seg_point(c, d, a), seg_point(c, d, b), seg_point(a, b, c), seg_point(a, b, d))


def seg_rect(s, r):
    x, y, w, h = r
    if any(x <= p[0] <= x + w and y <= p[1] <= y + h for p in s):
        return 0
    corners = [(x, y), (x + w, y), (x + w, y + h), (x, y + h)]
    return min(seg_seg(s, e) for e in zip(corners, corners[1:] + corners[:1]))


def chevron(tip, heading):
    """A drawn chevron as its tip (a zero-length stretch) and its base."""
    (x, y), (hx, hy) = tip, heading
    bx, by = x - hx * CHEVRON_DEPTH, y - hy * CHEVRON_DEPTH
    return [((x, y), (x, y)), ((bx - hy * CHEVRON_HALF, by - hx * CHEVRON_HALF), (bx + hy * CHEVRON_HALF, by + hx * CHEVRON_HALF))]


def head_gaps(t, view):
    """(tip, gap) for every drawn arrowhead: the least distance from its tip or
    base to ink that is not its own route's line or a chip at either end of
    its route, the chip it points into known by its place, not its distance.
    Lines and other heads count by their centre line, junction dots of other
    nets and every hop badge by their disc, labels, counts and zone titles by
    a text box estimated at 11px type (11px a CJK glyph, 6.6px any other),
    chips by their rect."""
    d = page_map(t, view)
    out, seen = [], set()
    for rs, es, _, heads in d.routes:
        for tip, hd in heads:
            if tip in seen:
                continue
            seen.add(tip)
            me = chevron(tip, hd)
            gaps = [seg_seg(m, s) for rid, segs in d.wires if not rid & rs for s in segs for m in me]
            gaps += [seg_seg(m, s) for rs2, _, _, hs in d.routes if not rs2 & rs
                     for tip2, hd2 in hs if tip2 != tip for s in chevron(tip2, hd2) for m in me]
            gaps += [max(0, seg_point(*m, c) - r) for e2, c, r in d.dots if not es <= e2 for m in me]
            gaps += [max(0, seg_point(*m, c) - r) for _, c, r in d.badges for m in me]
            gaps += [seg_rect(m, r) for r in d.texts for m in me]
            own = {"p" + e[1:] for e in es}
            gaps += [seg_rect(m, r) for ps, r in d.chips if not ps & own for m in me]
            out.append((tip, min(gaps, default=99)))
    return out


def crowded_heads(t, view):
    """Arrowheads with other ink closer than 6px."""
    return sum(1 for v in views_of(t, view) for _, gap in head_gaps(t, v) if gap < 6)


def crossed_badges(t, view):
    """Hop badges that another route's line passes within 2px of."""
    n = 0
    for v in views_of(t, view):
        d = page_map(t, v)
        n += sum(any(seg_point(*s, c) < r + 2 for rid, segs in d.wires if not rid & rs for s in segs) for rs, c, r in d.badges)
    return n


def crossing(s, q):
    """The point where a flat and an upright stretch cross inside both."""
    for (a, b), (c, d) in ((s, q), (q, s)):
        if a[1] == b[1] and c[0] == d[0] and min(a[0], b[0]) < c[0] < max(a[0], b[0]) and min(c[1], d[1]) < a[1] < max(c[1], d[1]):
            return c[0], a[1]
    return None


def fork_tangles(t, view):
    """Points where two traced lines that leave different places cross, each
    line an arrival of a fork: a place that lines from two or more places
    reach."""
    n = 0
    for v in views_of(t, view):
        got = [w for w in t.ways if w["view"] == v]
        froms = {}
        for w in got:
            froms.setdefault(w["to"], set()).add(w["from"])
        fork = [w for w in got if len(froms[w["to"]]) > 1]
        n += len({p for a, b in combinations(fork, 2) if a["from"] != b["from"]
                  for s in stretches(a) for q in stretches(b) for p in [crossing(s, q)] if p})
    return n


def edge_slacks(got):
    """{(line, end): slack} over a view's lines, for each end standing on a
    box's top or bottom edge whose box overlaps the far end's in x: how far
    the end stands from the nearest x of its edge over the far end's span,
    clear by slotGap of the edge's other ends, where the line's sideways run
    heads that way, so leaving or reaching there would have spared it that
    run and the turns and crossings on it.
    Lines sharing one end, a bus's stub or drop, move together: they count
    only where their spans meet, and then each the group's slack."""
    groups = {}
    for n, w in enumerate(got):
        for k, (p, r, far, nxt) in enumerate(((w["pts"][0], w["rects"][0], w["rects"][1], w["pts"][1:]),
                                              (w["pts"][-1], w["rects"][1], w["rects"][0], w["pts"][-2::-1]))):
            if r is None or far is None or p[1] not in (r[1], r[1] + r[3]):
                continue
            lo, hi = max(r[0], far[0]), min(r[0] + r[2], far[0] + far[2])
            if lo > hi:
                continue
            side = next((q[0] - p[0] for q in nxt if q[0] != p[0]), 0)
            groups.setdefault((r, p[1]), {}).setdefault(p[0], []).append(((n, k), lo, hi, side))
    out = {}
    for edge in groups.values():
        for x, members in edge.items():
            lo, hi = max(m[1] for m in members), min(m[2] for m in members)
            slack = 0
            if lo <= hi:
                free = [z for z in range(int(lo), int(hi) + 1) if all(abs(z - y) >= 12 for y in edge if y != x)] or [x]
                best = min(free, key=lambda z: abs(z - x))
                if abs(best - x) > 12 and all(m[3] * (best - x) > 0 for m in members):
                    slack = round(abs(best - x))
            for m in members:
                out[m[0]] = slack
    return out


def port_slack(frm=None, to=None):
    """Line ends that leave or reach their box more than slotGap from the
    nearest free x over the other end's span (edge_slacks): px of the one
    line's start, or a count of ends over the view."""
    def m(t, view):
        n = 0
        for v in views_of(t, view):
            got = [w for w in t.ways if w["view"] == v]
            for (i, k), s in edge_slacks(got).items():
                if not frm:
                    n += s > 0
                elif (got[i]["from"], got[i]["to"]) == (frm, to) and k == 0:
                    n += s
        return n
    return m


ZONE_TITLE = re.compile(r'<text class="zt"[^>]*x="' + NUM + '" y="' + NUM + r'"[^>]*>([^<]*)<')


def zone_width(words):
    """A zone title's set width as zone.go's zoneWidth reckons it: 10px small
    capitals spaced 2px apart."""
    w = 0
    for ch in words.upper():
        w += 11 if ord(ch) >= 0x1100 else 8 if ch.isupper() or ch in "MW@" else 7
    return w + 2 * len(words)


def inked_titles(svg):
    """Zone titles whose glyph box, cap height above the baseline to 1px
    under it, meets any stretch of line."""
    segs = [s for _, d in re.findall(r'<path class="(w [^"]*)" d="([^"]*)"', svg) for s in ink(d)]
    boxes = [(float(x), float(y) - 8, zone_width(html.unescape(w)), 9) for x, y, w in ZONE_TITLE.findall(svg)]
    return sum(1 for r in boxes if any(seg_rect(s, r) == 0 for s in segs))


def overview_svg(path):
    page = page_text(path)
    at = page.index("<svg", section(page, 'id="overview"', path))
    return page[at:page.index("</svg>", at)]


SELF_STATE = "self overview, this repository's state"


def titles_meet_ink(t, view):
    """Zone titles on overviews that meet line ink: the overview of this
    repository's own state, whose stored stack numbers its zones, or every
    fixture's overview."""
    if view == SELF_STATE:
        page = os.path.join(t.out, "self-pages", "index.html")
        return inked_titles(overview_svg(page)) if os.path.exists(page) else None
    return sum(inked_titles(overview_svg(os.path.join(t.pages, f, "index.html"))) for f in listing(t.pages))


BESIDE = 10


def drawn_of(t, view):
    """The drawn map of a fixture view, or of this repository's own overview
    (SELF_STATE), None when the checkout writes no such page."""
    if view != SELF_STATE:
        return page_map(t, view)
    page = os.path.join(t.out, "self-pages", "index.html")
    if not os.path.exists(page):
        return None
    if page not in DRAWN:
        DRAWN[page] = drawn(overview_svg(page))
    return DRAWN[page]


def pieces(segs):
    """A path's straight pieces: its stretches, each run of them one way
    merged."""
    out = []
    for a, b in segs:
        if a == b:
            continue
        if out and out[-1][1] == a and heading(*out[-1]) == heading(a, b):
            out[-1] = (out[-1][0], b)
        else:
            out.append((a, b))
    return out


def side_by_side(paths):
    """Px that pieces of two different paths run along one another, flat or
    upright, no more than BESIDE apart, each pair of pieces once."""
    n = 0
    for ax in (1, 0):
        o = 1 - ax
        rs = sorted((a[ax], min(a[o], b[o]), max(a[o], b[o]), i) for i, ps in enumerate(paths) for a, b in ps if a[ax] == b[ax])
        for k, (at, lo, hi, i) in enumerate(rs):
            for at2, lo2, hi2, j in rs[k + 1:]:
                if at2 - at > BESIDE:
                    break
                if i != j:
                    n += max(0, min(hi, hi2) - max(lo, lo2))
    return n


def busy(fn):
    """fn over the pieces of every drawn path.w of the view, or of every
    fixture view."""
    def m(t, view):
        got = [drawn_of(t, v) for v in views_of(t, view)]
        if None in got:
            return None
        return round(sum(fn([pieces(segs) for _, segs in d.wires]) for d in got))
    return m


def parallel_ink(t, view):
    """Px of drawn ink that runs beside or on another drawn path, the same
    way, within BESIDE: lines a reader sees doubled that a hand drawing
    merges into one, or keeps apart."""
    return busy(side_by_side)(t, view)


def segments(t, view):
    """Straight pieces of drawn ink."""
    return busy(lambda paths: sum(map(len, paths)))(t, view)


def bent(pts):
    """How often a polyline turns."""
    s = [heading(a, b) for a, b in zip(pts, pts[1:]) if a != b]
    return sum(1 for h, g in zip(s, s[1:]) if h != g)


def bends_per_line(t, view):
    """Traced turns per line, in hundredths; on this repository's own
    overview, the turns of each route the page lights."""
    if view == SELF_STATE:
        d = drawn_of(t, view)
        if d is None:
            return None
        lines = [bent(pts) for _, _, pts, _ in d.routes]
    else:
        lines = [bent(w["pts"]) for v in views_of(t, view) for w in t.ways if w["view"] == v]
    return round(100 * sum(lines) / max(1, len(lines)))


def flat_near(pts, last):
    """The y of the flat stretch nearest the polyline's last point, or its first."""
    s = list(zip(pts, pts[1:]))
    for a, b in reversed(s) if last else s:
        if a[1] == b[1] and a[0] != b[0]:
            return a[1]
    return None


def rows_of(rects):
    """{rect: row} as busy.go bands boxes and chips: a row is a run of them,
    by top, whose heights overlap."""
    row, n, bottom = {}, 0, float("-inf")
    for r in sorted(rects, key=lambda r: r[1]):
        if r[1] >= bottom:
            n += 1
        bottom = max(bottom, r[1] + r[3])
        row[r] = n
    return row


def fork_levels(t, view):
    """Over every fork, the flat tracks its branches turn onto past the
    first: a fork is the lines of one kind between one place (by name and
    rect, as chips share names) and two or more places of one row, boxes
    and chips banded together, the track being the flat stretch nearest
    that row."""
    n = 0
    for v in views_of(t, view):
        got = [w for w in t.ways if w["view"] == v]
        row = rows_of({r for w in got for r in w["rects"] if r})
        forks = {}
        for w in got:
            for out in (True, False):
                (place, at), (far, r) = [*zip((w["from"], w["to"]), w["rects"])][::1 if out else -1]
                if r is None:
                    continue
                f = forks.setdefault((place, at, w["kind"], row[r], out), (set(), set()))
                f[0].add(far)
                y = flat_near(w["pts"], out)
                if y is not None:
                    f[1].add(y)
        n += sum(len(ys) - 1 for fars, ys in forks.values() if len(fars) > 1 and len(ys) > 1)
    return n


def crossings(*names):
    """Points where two traced lines cross, flat over upright, inside both;
    with names, only crossings of a line from names[0] to one of the rest."""
    def m(t, view):
        got = [w for w in t.ways if w["view"] == t.view(view)]
        mine = [w for to in names[1:] for w in ways(t, view, names[0], to)] if names else got
        return len({p for a in mine for b in got if b is not a and (a["from"], a["to"]) != (b["from"], b["to"])
                    for s in stretches(a) for q in stretches(b) for p in [crossing(s, q)] if p})
    return m


def turns_of(frm, *tos):
    def m(t, view):
        return sum(turns(w) for to in tos for w in ways(t, view, frm, to))
    return m


def hover(key):
    """Findings of this kind shoot.py records over every fixture page it hovers."""
    def m(t, view):
        if t.hover is None:
            return None
        return sum(len(r[key]) for r in t.hover)
    return m


EYE = "user 2026-11-06 (eye review of main)"
PARALLEL = "px of ink beside or on another drawn path within 10px, the same way"
BENDS = "traced turns per line, in hundredths"
FORKS = "flat tracks past the first that one fork's branches turn onto"
RIPGREP, SEARCHER = 'box "ripgrep" (MOD-0001)', 'box "grep-searcher" (MOD-0012)'
EXEMPLAR_BENDS = "target 1.00, between user-08's about 0.56 (9 turns, 16 lines) and user-07's about 1.45 (32, 22), counted by eye"
Q_PS = "\"수직수평-나란히 배치되는 선을 가급적 병합하도록한 지침이 많이 누락되어 표현되다보니 선이 너무 많습니다\""
Q_RG_LEFT = "\"ripgrep -> grep-searcher / grep-matcher 로 가는 선은 좌측으로 우회했다면 깔끔하게 표현되었을 것\""
Q_R72 = "\"renderer/lib 이나 중앙으로 집중되는 선들이 복잡하게 얽혀있습니다 ... 심각한 수준 ... 불필요한 꺾임 또한 크게 발생\""
Q_S41 = "\"불필요한 꺾임이 많이 보입니다 불필요한 교차도 발생\""
Q_SELF = "\"수평수직 병합이 덜 되어 복잡, 꺾임이 많음\""

Case = namedtuple("Case", "id source view measure fn target note")

CASES = [
    Case("rg-regex-drop", "today; user-16, user-17", RG6, "turns past 0 in one column, 2 in two, of grep -> grep-regex",
         on(excess, GREP, REGEX), 0, "rows 67-70 of the overview: the hairpin is gone, but grep-regex stands in grep's column"
         " and the line still turns twice; the density cut draws the line only in focus views since b95ffaf8"),
    Case("rg-regex-hairpin", "today; user-16, user-17", RG6, "turn-backs and doubled stretches of grep -> grep-regex",
         on(lambda w: backs(w) + doubles(w), GREP, REGEX), 0, "guard: rows 69-70 of the overview took the ride past"
         " pcre2 cargo feature's drop and back; the density cut draws the line only in focus views since b95ffaf8"),
    Case("rg-doubled", "user-18", RG, "stretches of one line side by side within 12px for 24px",
         on(doubles), 0, "guard: two lanes of one line read as one doubled line"),
    Case("rg-matcher-out", "today", RG, "px grep -> grep-matcher runs past its ends' x span",
         past_ends(GREP, MATCHER), 0, "row 71: it passes at x=769 to reach 337 from 428"),
    Case("rg-matcher-out-ripgrep", "today", RG1, "px ripgrep -> grep-matcher runs past its ends' x span",
         past_ends('box "ripgrep" (MOD-0001)', MATCHER), 0,
         "the same bus as grep's line, from 578, on the overview; its density cut draws the line only here since b95ffaf8"),
    Case("rg-matcher-back", "today", RG, "turn-backs of lines at grep-matcher",
         on(backs, MATCHER), 0, "grep-matcher's arrivals ride around a row and back"),
    Case("r72-straight", "today", R72, "lines between boxes in one column that turn",
         on(lambda w: shares_column(w) and turns(w) > 0), 0, "a line that could drop straight turns"),
    Case("r72-back", "today", R72, "turn-backs", on(backs), 0, "app-shell's lines ride 90px past their turn"),
    Case("r72-jogs", "today", R72, "jogs", on(jogs), 0, "stretches under 24px between turns"),
    Case("r72-steps", "today", R72, "steps", on(steps), 0, "jogs plus turns past 2 per line"),
    Case("f73-one-stem", "today", F73, "distinct x's app-shell's depends_on lines leave it at",
         stems(APP_SHELL, "depends_on"), 1, "5 children in one row: x3/1/1 stubs at 493, 516, 586; one stem that forks"),
    Case("f73-one-track", "today", F73, "distinct y's app-shell's depends_on lines turn onto",
         tracks(APP_SHELL, "depends_on"), 1, "the fork's branches leave its stem onto one track"),
    Case("f73-beside", "today", F73, "px two stems from app-shell run beside each other within 16px",
         beside(APP_SHELL), 0, "guard: no stub runs beside the stem through its track"),
    Case("s98-mobile", "user-11, user-12, user-15", S98, "excess turns, jogs and turn-backs of lines at chip ← mobile/",
         on(knots, MOBILE), 0, "the mobile/ entry's lines stair down through the middle"),
    Case("s98-mobile-beside", "user-13, user-14", S98, "px two stems from chip ← mobile/ run beside each other within 16px",
         beside(MOBILE), 0, "the x2 and x5 stems run as two parallel verticals; one stem, or apart"),
    Case("s98-doubled", "review-112", S98, "pairs of wires of one kind, not one bundle, drawn on one another",
         doubled("orca", "MOD-0098"), 0, "wires 37 and 39 both draw M626 526 L626 558, so the pointer never reaches 37"),
    Case("s98-native", "user-11, user-12", S98, "excess turns, jogs and turn-backs of lines at chip ← native/",
         on(knots, NATIVE), 0, "guard: the native/ entry now drops straight into desktop main process"),
    Case("s98-host-jogs", "user-15", S98, "jogs of lines at @orca/process-host",
         on(jogs, HOST), 0, "process-host's arrivals step in small jogs above it"),
    Case("s98-connector", "user-02, user-03 (MOD-0100, not a fixture)", S98,
         "excess turns, jogs and turn-backs of lines at chip @orca-cloud/connector- contract",
         on(knots, CONNECTOR), 0, "MOD-0100's stairs down to the frame's bottom chips; MOD-0098 has the same chips"),
    Case("n97-exit", "user-04", "orca MOD-0097 native/", "px windows-agent-hook -> chip → src/ runs sideways",
         x_travel('box "windows-agent-hook" (MOD-0034)', 'chip "→ src/"'), 0,
         "guard: the exit ran left under process-footprint-macos, reading as its line"),
    Case("m81-rpc-sideways", "review-122", M81, "px chip Orca runtime and chip mobile/ -> shared/rpc-contract run sideways",
         x_travel('chip "Orca runtime (RPC host)"', RPC, 'chip "mobile/"'), 220,
         "#122 drew mobile/'s line from 1000 left to rpc-contract at 370 (630px), Orca runtime's from 502 (132px); before it, 220 in all"),
    Case("m81-connector-turns", "review-122", M81, "turns of desktop copy of connector contract -> chip @orca-cloud/connector- contract",
         on(turns, 'box "desktop copy of connector contract" (IF-0025)', CONNECTOR), 0,
         "#122 leaves the drop at 782 and turns it twice to reach the chip at 962; before it, it dropped straight at 106"),
    Case("self-gitx", "user-05", "self overview", "excess turns, jogs and turn-backs of lines at internal/gitx",
         on(knots, GITX), 0, "the knot of steps between internal/extract, internal/store and internal/gitx"),
    Case("p01-tee", "today", "pstack MOD-0001 setup and help",
         "points where a line's straight drop crosses the track of another line leaving its box",
         tees, 0, "/poteto-help -> 10-recipes drops 420,106 to 420,142 across its box's own bus to 01-setup at 420,130"),
    Case("self-head-bend", "user-19", "self overview",
         "arrowheads whose line turns under the chevron's 7.2px depth plus 12px before the tip",
         bent_heads, 0, "/abbd -> /abbd-setup turns at 45,100 and runs 8px up into the head at 45,92 under plugins/abbd/skills/"),
    Case("head-bend", "user-19", EVERY, "arrowheads whose line turns under the chevron's 7.2px depth plus 12px before the tip",
         bent_heads, 0, "the same rule over every view of every fixture"),
    Case("s41-hwm-steps", "user-20", "self MOD-0041 internal/store", "jogs plus turns past 2 of ← identity -> ID high-water mark",
         on(steps, 'chip "← identity"', 'box "ID high-water mark" (DATA-0018)'), 0,
         "369,38 down to 78, left to 332, down to 106, a 16px jog right to the junction at 348, down into the box: 4 turns, 1 jog"),
    Case("f01-fork-tangle", "user 2026-11-06 (FEAT-0001, no shot)", "self FEAT-0001 /abbd",
         "points where arrivals of forks, leaving different places, cross", fork_tangles, 0,
         "record relation's bus at y=374 and diff's at 382 interleave: after work's feed crosses 569,374, relation's drop crosses 690,382"),
    Case("fork-tangle", "user 2026-11-06 (FEAT-0001, no shot)", EVERY, "points where arrivals of forks, leaving different places, cross",
         fork_tangles, 0, "the same count over every view of every fixture"),
    Case("f01-head-room", "user-21", "self FEAT-0001 /abbd", "arrowheads under 6px from other ink",
         crowded_heads, 0, "the hop head at 265,274 sits 0.8px under badge 2 at 265,258; badge 1, and ×2 counts beside two chip heads"),
    Case("head-room", "user-21", EVERY, "arrowheads under 6px from other ink", crowded_heads, 0,
         "the same count over every view of every fixture"),
    Case("f01-badge-crossed", "user-21", "self FEAT-0001 /abbd", "hop badges another route's line passes within 2px of",
         crossed_badges, 0, "← abbd CLI -> after work's dotted bus at y=258 runs through badge 2's centre"),
    Case("f01-port", "user-22", "self FEAT-0001 /abbd",
         "px /abbd read the code -> chip → pstack leaves its box short of the nearest free x over the chip",
         port_slack('box "/abbd read the code through pstack" (FEAT-0005)', 'chip "→ pstack (external)"'), 0,
         "it leaves at 336, the left of the box's bottom, and crosses to the chip at 462; the box spans 250..422 over the chip's 381..531"),
    Case("port-slack", "user-22", EVERY, "line ends more than 12px short of the nearest free x of their edge over the far end",
         port_slack(), 0, "the same rule over both ends of every line of every fixture view"),
    Case("self-title-meets-ink", "today (zones, #112)", SELF_STATE, "zone titles whose glyph box meets line ink",
         titles_meet_ink, 0, "buses at y=220, 228 and 236 cross the room above internal/screen, so 2 · blueprint rendering at 683,242 sits on the one at 236"),
    Case("title-meets-ink", "today (zones, #112)", "every fixture overview", "zone titles whose glyph box meets line ink",
         titles_meet_ink, 0, "the same count over every fixture's overview"),
    Case("ps-parallel", EYE, "pstack overview", PARALLEL, parallel_ink, 7 * 61,
         Q_PS + "; lines 12 and 21 run 24px on one track, 17 and 41 40px; target 7px a line, user-07's, over 61 lines (user-08: 0)"),
    Case("ps-bends", EYE, "pstack overview", BENDS, bends_per_line, 100, Q_PS + "; " + EXEMPLAR_BENDS),
    Case("rg-left-turns", EYE, RG1, "turns of ripgrep -> grep-searcher and grep-matcher",
         turns_of(RIPGREP, SEARCHER, MATCHER), 4,
         Q_RG_LEFT + "; on the overview each left at 578, joined grep's trunk at 428 between the rows and turned 4 times;"
         " 2 each; the density cut draws both lines only here since b95ffaf8"),
    Case("rg-left-crossings", EYE, RG1, "crossings of ripgrep -> grep-searcher and grep-matcher",
         crossings(RIPGREP, SEARCHER, MATCHER), 0, Q_RG_LEFT + "; the overview's trunk at 428 ran through the middle of the drawing"),
    Case("rg-left-overview-turns", EYE, RG, "turns of ripgrep -> grep-searcher", turns_of(RIPGREP, SEARCHER), 2,
         Q_RG_LEFT + "; guard on the overview the user looked at, which since b95ffaf8 draws only this of the two, straight down at 978"),
    Case("rg-left-overview-crossings", EYE, RG, "crossings of ripgrep -> grep-searcher", crossings(RIPGREP, SEARCHER), 1,
         Q_RG_LEFT + "; guard: today only ripgrep -> grep-printer's track at y=228 crosses it; the flagged route via 388 crossed 3"),
    Case("rg-left-sideways", EYE, RG1, "px ripgrep -> grep-searcher and grep-matcher run sideways past the x span between their ends",
         x_slack(RIPGREP, SEARCHER, MATCHER), 0,
         "guard: on the overview the 505 they ran was their ends' x span; the way round must not add sideways for its fewer turns"),
    Case("rg-regex-steps", EYE, RG, "steps of grep-regex -> grep-matcher", on(steps, REGEX, MATCHER), 0,
         "\"grep-regex -> grep-matcher 로 가는 선의 계단이 불필요하게 발생\"; 483,340 down to 372, left to 448, down to 508, left to 376, down: 4 turns"),
    Case("s98-parallel", EYE, S98, PARALLEL, parallel_ink, 7 * 37,
         "\"수평-수직 선이 인접하게 진행할 때 병합하다 갈라지는 표현이 필요\"; target 7px a line over 37 lines"),
    Case("r72-bends", EYE, R72, BENDS, bends_per_line, 100, Q_R72 + "; " + EXEMPLAR_BENDS),
    Case("r72-parallel", EYE, R72, PARALLEL, parallel_ink, 7 * 43, Q_R72 + "; target 7px a line over 43 lines"),
    Case("s41-bends", EYE, "self MOD-0041 internal/store", BENDS, bends_per_line, 100, Q_S41 + "; " + EXEMPLAR_BENDS),
    Case("s41-crossings", EYE, "self MOD-0041 internal/store", "points where two traced lines cross", crossings(), 2,
         Q_S41 + "; target 0.2 a line, the exemplars' (user-08 about 1 in 16 lines, user-07 about 8 in 22), over 11 lines"),
    Case("f01-fork-levels", EYE, "self FEAT-0001 /abbd", FORKS, fork_levels, 0,
         "\"포크로 내려가 갈라지는 선이 포크의 수평이 일정하지 못해 지저분해 보입니다\"; the forks into the bottom chips split at y=362, 378 and 386"),
    Case("self-parallel", EYE, "self overview", PARALLEL, parallel_ink, 7 * 37, Q_SELF + "; target 7px a line over 37 lines"),
    Case("self-bends", EYE, "self overview", BENDS, bends_per_line, 100, Q_SELF + "; " + EXEMPLAR_BENDS),
    Case("repo-parallel", EYE, SELF_STATE, PARALLEL, parallel_ink, 7 * 39,
         Q_SELF + " (.abbd/blueprint/index.html); target 7px a line over its 39 routes"),
    Case("repo-bends", EYE, SELF_STATE, "turns per lit route, in hundredths", bends_per_line, 100,
         Q_SELF + " (.abbd/blueprint/index.html); the page's routes, as it stores no trace; " + EXEMPLAR_BENDS),
    Case("parallel-ink", EYE, EVERY, PARALLEL, parallel_ink, 0, "measured, not a goal of its own yet: the same count over every fixture view"),
    Case("fork-levels", EYE, EVERY, FORKS, fork_levels, 0, "the same count over every fixture view"),
    Case("hover-cut", "today", "every shot page", "shoot.py cut", hover("cut"), 0, "guard: a lit route stops partway"),
    Case("hover-stray", "today", "every shot page", "shoot.py stray", hover("stray"), 0, "guard: a lit box no lit line reaches"),
    Case("hover-mixed", "today", "every shot page", "shoot.py mixed", hover("mixed"), 0, "guard: one lit route of two colours"),
    Case("hover-overlap", "today", "every shot page", "shoot.py overlap", hover("overlap"), 0,
         "two colours along one track, one painting over the other"),
]


def run(out, hover_on, root):
    os.makedirs(out, exist_ok=True)
    pages = os.path.join(out, "pages")
    tables = [os.path.join(out, "fix.tsv"), os.path.join(out, "ways.tsv")]
    for f in tables:
        if os.path.exists(f):
            os.remove(f)
    env = dict(os.environ, ABBD_LINT_FIXTURES=tables[0], ABBD_ACCEPT_WAYS=tables[1], ABBD_BLUEPRINTS=pages,
               ABBD_ACCEPT_SELF=os.path.join(out, "self-pages"))
    go = subprocess.run(["go", "test", "./internal/screen", "-count=1", "-run",
                         "^(TestLintFixtures|TestAcceptWays|TestEveryRealDrawingIsLegible|TestAcceptSelfPages)$"],
                        cwd=root, env=env, capture_output=True, text=True)
    tail = (go.stdout + go.stderr)[-2000:]
    if not all(os.path.exists(f) for f in tables):
        sys.exit(f"go test in {root} wrote no tables, so nothing was measured (exit {go.returncode}):\n{tail}")
    if go.returncode:
        print(f"go test in {root} failed (tables still read):\n{tail}", file=sys.stderr)
    if hover_on:
        shoot = os.path.join(ROOT, "eval", "skill-polish", "tools", "shoot.py")
        repos = [f"{r}={os.path.join(pages, r)}" for r in sorted(os.listdir(pages))]
        subprocess.run([sys.executable, shoot, os.path.join(out, "shots"), *repos, "--pages", "all", "--hover-shots", "0"],
                       check=True, stdout=subprocess.DEVNULL)
    t = Tables(out, hover_on)
    return [measure(c, t) for c in CASES]


def measure(c, t):
    """The case's value, None when it skips, or the reason it could not be measured."""
    try:
        v = c.fn(t, c.view)
    except Unmeasured as e:
        return str(e)
    return None if v is None else int(v)


def describe(root):
    return subprocess.run(["git", "describe", "--always", "--dirty"], cwd=root, capture_output=True, text=True, check=True).stdout.strip()


def main():
    args = sys.argv[1:]
    if not args or args[0].startswith("-"):
        sys.exit(__doc__)
    sys.stdout.reconfigure(encoding="utf8")
    out, hover_on, record = args[0], "--no-hover" not in args, "--record" in args
    base = args[args.index("--main") + 1] if "--main" in args else None
    values = run(out, hover_on, ROOT)
    mains = run(os.path.join(out, "main"), hover_on, base) if base else [None] * len(CASES)
    results = ["skip" if v is None else f"fail: {v}" if isinstance(v, str) else "pass" if v <= c.target else "fail"
               for c, v in zip(CASES, values)]
    show = lambda v: "" if v is None or isinstance(v, str) else str(v)
    print("case\tsource\tview\tmeasure\tvalue\tmain\ttarget\tresult")
    for c, v, m, r in zip(CASES, values, mains, results):
        print(f"{c.id}\t{c.source}\t{c.view}\t{c.measure}\t{show(v)}\t{show(m)}\t<= {c.target}\t{r}")
    skipped = results.count("skip")
    failed = sum(r.startswith("fail") for r in results)
    print(f"\n{failed} of {len(results) - skipped} measured cases fail" + (f", {skipped} skipped" if skipped else ""))
    if record:
        path = os.path.join(HERE, "acceptance.tsv")
        old = {r["case"]: r for r in read(path)} if os.path.exists(path) else {}
        at, main_at = describe(ROOT), describe(base) if base else ""
        with open(path, "w", newline="", encoding="utf8") as f:
            f.write("case\tsource\tview\tmeasure\trecorded\tcommit\tmain\tmain_commit\ttarget\tnote\n")
            for c, v, m in zip(CASES, values, mains):
                was = old.get(c.id, {})
                rec = [show(v), at] if show(v) else [was.get("recorded", ""), was.get("commit", "")]
                mn = [show(m), main_at] if show(m) else [was.get("main", ""), was.get("main_commit", "")]
                f.write("\t".join([c.id, c.source, c.view, c.measure, *rec, *mn, f"<= {c.target}", c.note]) + "\n")
    sys.exit(1 if failed else 0)


if __name__ == "__main__":
    main()

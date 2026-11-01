"""Hand-laid reference drawing of this repository's overview (testdata/self).

Every coordinate below was chosen by hand, the way a person lays a diagram
out in draw.io: same boxes, same relations, same rules. The checker in
internal/screen/trace_test.go measures it as the target row.

Run: python eval/render-r1/reference_self.py > eval/render-r1/reference-self.svg
"""
import sys

W, H = 1232, 680
out = []


def box(id, name, x, y, w=140, h=56, cls="design", chips=()):
    s = f'<g class="box {cls}" data-id="{id}"><title>{id} {name}</title><rect x="{x}" y="{y}" width="{w}" height="{h}" rx="6"/>'
    s += f'<text class="name" x="{x + w // 2}" y="{y + h // 2 + 5}" text-anchor="middle">{name}</text></g>'
    out.append(s)
    for cx, cy, cw, label, exit in chips:
        cl = "exit" if exit else "entry"
        out.append(f'<g class="port design {cl}"><title>{label}</title><rect x="{cx}" y="{cy}" width="{cw}" height="15" rx="3"/><text class="chip" x="{cx + 4}" y="{cy + 11}">{label}</text></g>')


def line(pts, kind, title, joins=False, back=False, both=False):
    cls = "w " + kind + (" joins" if joins else "") + (" back" if back else "") + (" both" if both else "")
    d = "M" + " L".join(f"{x} {y}" for x, y in pts)
    out.append(f'<path class="{cls}" d="{d}"><title>{title}</title></path>')


def dot(x, y):
    out.append(f'<circle class="junction" cx="{x}" cy="{y}" r="3"/>')


def count(x, y, n):
    out.append(f'<text class="count" x="{x}" y="{y}">×{n}</text>')


# Rows: the skills, the CLI they drive, what the CLI runs, the stores, the base.
r0, r1, r2, r3, r4 = 40, 170, 310, 450, 590
B = 56
box("FEAT-0001", "/abbd", 290, r0, 160)
box("FEAT-0002", "/abbd-setup", 690, r0, 160)
box("MOD-0055", "pstack (external)", 150, r1, 164)
box("MOD-0001", "abbd CLI", 380, r1, 480, chips=[(500, r1 + 3, 70, "계약 11개", True), (740, r1 + 3, 70, "계약 13개", True), (408, r1 + 38, 80, "sync state", True)])
box("MOD-0034", "internal/screen", 30, r2, 172)
box("MOD-0026", "internal/identity", 230, r2, 172)
box("MOD-0009", "internal/bootstrap", 430, r2, 172)
box("MOD-0013", "internal/evidence", 630, r2, 172)
box("MOD-0054", "internal/group", 830, r2, 172)
box("MOD-0047", "internal/topology", 1030, r2, 172)
box("MOD-0041", "internal/store", 210, r3, 190, chips=[(218, r3 + 3, 70, "계약 4개", True)])
box("MOD-0017", "internal/extract", 440, r3, 150)
box("MOD-0022", "internal/gitx", 380, r4, 160)
box("MOD-0030", "internal/model", 740, r4, 160, chips=[(748, r4 + 3, 80, "relation key", True)])
box("CFG-0001", "plugin manifest", 1040, r4, 150, 40, cls="loose design")

# The two skills use each other: one line, a head at each end.
line([(450, r0 + 28), (690, r0 + 28)], "mutual", "/abbd-setup ↔ /abbd", both=True)
# Each skill drives the CLI straight down; the CLI reads /abbd back, dashed.
line([(420, r0 + B), (420, r1)], "contract", "/abbd → abbd CLI")
line([(720, r0 + B), (720, r1)], "contract", "/abbd-setup → abbd CLI")
line([(400, r1), (400, r0 + B)], "bypass", "abbd CLI → /abbd", back=True)
# Both skills use pstack: /abbd straight down, /abbd-setup merges into it.
line([(300, r0 + B), (300, r1)], "bypass", "/abbd → pstack")
line([(705, r0 + B), (705, 130), (300, 130)], "bypass", "/abbd-setup → pstack", joins=True)
dot(300, 130)

# The CLI runs six modules: one stub, one rail, one drop each.
rail = 268
cols = [116, 316, 516, 716, 916, 1116]
line([(716, r1 + B), (716, rail)], "bypass", "abbd CLI → six modules", joins=True)
line([(116, rail), (1116, rail)], "bypass", "rail", joins=True)
for x in cols:
    line([(x, rail), (x, r2)], "bypass", "drop")
for x in cols:
    if x != 716:
        dot(x, rail)
dot(716, rail)
count(722, 246, 7)
# ... and its trunk to gitx and model, down the gap after identity.
tr = 416
line([(tr, rail), (tr, 530)], "bypass", "abbd CLI → gitx, model", joins=True)
dot(tr, rail)

# Screen and identity read sync state back from the CLI: dashed, merged.
line([(392, r2), (392, r1 + B)], "contract", "internal/identity → abbd CLI", back=True)
line([(40, r2), (40, 250), (392, 250)], "contract", "internal/screen → abbd CLI", joins=True, back=True)
dot(392, 250)

# The CLI's own contracts go down one trunk in the gap after evidence.
ct = 816
line([(ct, r1 + B), (ct, r4)], "contract", "abbd CLI → internal/model")
line([(ct, 410), (300, 410)], "contract", "abbd CLI → store, extract", joins=True)
dot(ct, 410)
line([(560, 410), (560, r3)], "contract", "abbd CLI → internal/extract")
dot(560, 410)
# Store is reached by identity, the CLI and screen: one arrival.
line([(300, r2 + B), (300, r3)], "contract", "internal/identity → internal/store")
dot(300, 410)
line([(170, r2 + B), (170, 578), (ct, 578)], "contract", "internal/screen → store, model", joins=True)
line([(170, 422), (300, 422)], "contract", "internal/screen → internal/store", joins=True)
dot(170, 422)
dot(300, 422)
count(306, 446, 3)
line([(1116, r2 + B), (1116, 578), (ct, 578)], "contract", "internal/topology → internal/model", joins=True)
dot(ct, 578)

# Bootstrap reads extract straight down; identity merges into the CLI's
# trunk, which branches to extract on the way to the base.
line([(530, r2 + B), (530, r3)], "bypass", "internal/bootstrap → internal/extract")
line([(380, r2 + B), (380, 380), (tr, 380)], "bypass", "internal/identity → trunk", joins=True)
dot(tr, 380)
line([(tr, 425), (480, 425), (480, r3)], "bypass", "trunk → internal/extract")
dot(tr, 425)
# Everything that uses both gitx and model shares one bus that splits at the end.
bus = 530
line([(40, r2 + B), (40, bus)], "bypass", "internal/screen → gitx, model", joins=True)
line([(330, r3 + B), (330, bus)], "bypass", "internal/store → gitx, model", joins=True)
line([(520, r3 + B), (520, bus)], "bypass", "internal/extract → gitx, model", joins=True)
line([(716, r2 + B), (716, bus)], "bypass", "internal/evidence → gitx, model", joins=True)
line([(40, bus), (780, bus)], "bypass", "bus", joins=True)
line([(460, bus), (460, r4)], "bypass", "→ internal/gitx")
line([(780, bus), (780, r4)], "bypass", "→ internal/model")
for x in (330, tr, 460, 520, 716):
    dot(x, bus)
count(466, 552, 6)
count(786, 550, 8)
# Model only: bootstrap and group join model's own drop, after the split.
line([(596, r2 + B), (596, 566), (780, 566)], "bypass", "internal/bootstrap → internal/model", joins=True)
line([(880, r2 + B), (880, 566), (780, 566)], "bypass", "internal/group → internal/model", joins=True)
dot(780, 566)
# Store reads identity back: dashed.
line([(390, r3), (390, 440), (396, 440), (396, r2 + B)], "bypass", "internal/store → internal/identity", back=True)

style = """<style>
svg { background: #161614; } text { fill: #ecebe6; font: 13px system-ui, sans-serif; } text.name { font-weight: 650; }
.box rect { fill: #201f1c; stroke: #ecebe6; stroke-width: 1.6; } .box.loose rect { stroke-dasharray: 3 3; stroke: #a29e93; }
.port rect { stroke-width: 1.2; } .port.exit rect { fill: #7fb0e0; stroke: #7fb0e0; } .port.exit text { fill: #161614; font-weight: 600; }
.port.entry rect { fill: #201f1c; stroke: #7fb0e0; } .port.entry text { fill: #7fb0e0; } text.chip { font-size: 11px; }
path.w { fill: none; stroke-width: 1.5; } path.contract { stroke: #7fb0e0; marker-end: url(#m-contract); }
path.bypass, path.mutual { stroke: #fb923c; stroke-width: 2; marker-end: url(#m-bypass); } path.w.both { marker-start: url(#m-bypass); }
path.w.joins { marker-end: none; } path.w.back { stroke-dasharray: 6 4; } circle.junction { fill: #ecebe6; }
text.count { font-size: 11px; font-weight: 700; fill: #a29e93; }
</style>"""
marks = "".join(
    f'<marker id="m-{k}" viewBox="0 0 10 10" refX="9" refY="5" markerUnits="userSpaceOnUse" markerWidth="9" markerHeight="9" orient="auto-start-reverse"><path d="M0 0 L10 5 L0 10 z" style="fill: {c}"/></marker>'
    for k, c in (("contract", "#7fb0e0"), ("bypass", "#fb923c"))
)
sys.stdout.reconfigure(encoding="utf-8", newline="\n")
print(f'<svg xmlns="http://www.w3.org/2000/svg" width="{W}" height="{H}" viewBox="0 0 {W} {H}">{style}<defs>{marks}</defs>')
print("\n".join([s for s in out if s.startswith("<path")] + [s for s in out if not s.startswith("<path")]))
print("</svg>")

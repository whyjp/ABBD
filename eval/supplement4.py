"""Merge the semantic-track workers' reports into one ABBD supplement for Orca.

  python -I eval/supplement4.py <orca checkout> <script extraction.json> <sem dir> <out supplement.json> <out check.json>

Each worker wrote <slice>.json with units, relations and purposes keyed by script unit keys. This merges them,
turns purposes into supplement units (key, kind, name and one anchor of the script unit, plus purpose), drops
relations the script already reports, and checks every evidence quote against the file the way ABBD does
(whitespace-insensitive, starting within three lines of `lines`). Relations or units whose quotes all miss are
left out and listed in the check file, so setup only ever sees evidence that is really in the tree.
"""
import glob
import json
import os
import re
import sys

KINDS_PROVIDE = {"MOD", "FEAT"}
CONTRACTS = {"IF", "EVT", "DATA", "CFG"}
VIAS = {"import", "trait", "callback", "macro", "config", "build-feature", "data", "convention", "call"}


def squash(s):
    return re.sub(r"\s+", "", s)


def quote_ok(root, ev):
    path = os.path.join(root, ev["path"])
    if not os.path.isfile(path):
        return False
    lines = open(path, encoding="utf-8", errors="replace").read().split("\n")
    q = squash(ev.get("quote", ""))
    if not q:
        return False
    span = ev.get("lines", "")
    m = re.match(r"(\d+)(?:-(\d+))?$", span or "")
    starts = range(len(lines)) if not m else range(max(0, int(m.group(1)) - 4), min(len(lines), int(m.group(1)) + 3))
    for i in starts:
        if squash("".join(lines[i:i + 40])).startswith(q) or q in squash("".join(lines[i:i + 3])):
            return True
    return False


def main():
    root, script, semdir, out, check = sys.argv[1:6]
    sx = json.load(open(script, encoding="utf-8"))
    sunits = {u["key"]: u for u in sx["units"]}
    srels = {(r["from"], r["kind"], r["to"]) for r in sx["relations"]}
    units, rels, purposes, log = {}, {}, {}, {"dropped_units": [], "dropped_relations": [], "fixed": []}
    for path in sorted(glob.glob(os.path.join(semdir, "*.json"))):
        if os.path.basename(path) in ("supplement.json", "check.json"):
            continue
        slice_ = os.path.basename(path)[:-5]
        d = json.load(open(path, encoding="utf-8"))
        for k, v in (d.get("purposes") or {}).items():
            if k in sunits and k not in purposes:
                purposes[k] = v
        for u in d.get("units", []):
            if u["key"] in sunits:
                if u.get("responsibility") and u["key"] not in purposes:
                    purposes[u["key"]] = u["responsibility"]
                continue
            ev = [e for e in u.get("evidence", []) if quote_ok(root, e)]
            if not ev:
                log["dropped_units"].append({"slice": slice_, "key": u["key"], "why": "no live quote"})
                continue
            nu = {k: u[k] for k in ("key", "kind", "name", "parent", "signature", "responsibility") if u.get(k)}
            if nu["kind"] not in CONTRACTS:
                nu.pop("signature", None)
            nu["evidence"] = ev
            nu["anchors"] = [{"path": e["path"]} for e in ev][:1]
            units.setdefault(u["key"], nu)
        for r in d.get("relations", []):
            rels.setdefault((r["from"], r["kind"], r["to"]), (slice_, r))
    kind = {k: u["kind"] for k, u in sunits.items()}
    kind.update({k: u["kind"] for k, u in units.items()})
    for k, u in list(units.items()):
        p = u.get("parent")
        if p not in sunits and p and p.split("/")[0] in sunits:
            u["parent"] = p.split("/")[0]
            log["fixed"].append({"key": k, "parent": p, "now": u["parent"]})
            p = u["parent"]
        if p not in sunits or sunits[p]["kind"] != "MOD":
            log["dropped_units"].append({"key": k, "why": "parent %r is not a script module" % p})
            del units[k]
            kind.pop(k, None)
    out_rels = []
    for (a, k, b), (slice_, r) in sorted(rels.items()):
        why = None
        a0, b0 = a, b
        a = a if a in kind or "/" not in a else a.split("/")[0]
        b = b if b in kind or "/" not in b else b.split("/")[0]
        if (a, b) != (a0, b0):
            log["fixed"].append({"rel": "%s %s %s" % (a0, k, b0), "now": "%s %s %s" % (a, k, b)})
        if (a, k, b) in srels:
            why = "script already reports it"
        elif a not in kind or b not in kind:
            why = "unknown key"
        elif a == b:
            why = "self relation"
        elif k == "provides" and (kind[a] not in KINDS_PROVIDE or kind[b] not in CONTRACTS):
            why = "provides must go from a MOD or FEAT to a contract"
        elif k == "consumes" and kind[b] not in CONTRACTS:
            why = "consumes must end at a contract"
        elif k not in ("depends_on", "consumes", "provides"):
            why = "unknown kind"
        elif r.get("via") not in VIAS:
            why = "via %r" % r.get("via")
        ev = [e for e in r.get("evidence", []) if quote_ok(root, e)]
        if not why and not ev:
            why = "no live quote"
        if why:
            log["dropped_relations"].append({"slice": slice_, "rel": "%s %s %s" % (a, k, b), "why": why})
            continue
        out_rels.append({"from": a, "kind": k, "to": b, "via": r["via"], "evidence": ev})
    # ABBD parses the supplement as a document of its own: every parent and relation end must be a unit in it,
    # so script units that are referenced but got no purpose are repeated as bare stubs.
    referenced = {u.get("parent") for u in units.values()} | {r["from"] for r in out_rels} | {r["to"] for r in out_rels}
    sup_units = []
    for k in sorted((set(purposes) | referenced) & set(sunits)):
        su = sunits[k]
        stub = {"key": k, "kind": su["kind"], "name": su["name"], "anchors": su["anchors"]}
        if k in purposes:
            stub["purpose"] = purposes[k]
        sup_units.append(stub)
    sup_units += [units[k] for k in sorted(units)]
    json.dump({"units": sup_units, "relations": out_rels}, open(out, "w", encoding="utf-8", newline="\n"), indent=1, ensure_ascii=False)
    log["counts"] = {"purposes": len(purposes), "new_units": len(units), "relations_in": len(rels), "relations_out": len(out_rels)}
    json.dump(log, open(check, "w", encoding="utf-8", newline="\n"), indent=1, ensure_ascii=False)
    print(json.dumps(log["counts"]), "dropped units", len(log["dropped_units"]), "dropped relations", len(log["dropped_relations"]))


main()

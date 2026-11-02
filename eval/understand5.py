"""Store pstack how's Overview and Key Concepts for Orca's main modules with `abbd understand`.

  python -I eval/understand5.py <orca checkout> <abbd exe> <sem dir>

For each <slice>-how.md the semantic workers wrote, take its Overview and Key Concepts sections verbatim and
hand them to `abbd understand <MOD-ID> -`, where the ID is the unit whose name the script gave that module.
"""
import json
import os
import re
import subprocess
import sys

NAMES = {"desktop-main": "desktop main process", "desktop-runtime": "Orca runtime (RPC host)",
         "renderer": "desktop renderer UI", "mobile": "mobile app (Expo)", "cloud-relay": "@orca-cloud/relay"}


def sections(md):
    keep, out = False, []
    for line in md.splitlines():
        m = re.match(r"#+\s*(.+?)\s*$", line)
        if m:
            keep = m.group(1).lower() in ("overview", "key concepts")
        if keep:
            out.append(line)
    return "\n".join(out).strip() + "\n"


def main():
    root, abbd, sem = sys.argv[1:4]
    graph = json.load(open(os.path.join(root, ".abbd", "blueprint", "graph.json"), encoding="utf-8"))
    ids = {u["Name"]: u["ID"] for u in graph["units"] if not u.get("Parent")}
    for slice_, name in NAMES.items():
        text = sections(open(os.path.join(sem, slice_ + "-how.md"), encoding="utf-8").read())
        p = subprocess.run([abbd, "understand", ids[name], "-"], cwd=root, input=text, capture_output=True,
                           text=True, encoding="utf-8")
        print(slice_, ids[name], p.returncode, (p.stdout + p.stderr).strip()[:200], len(text), "chars")


main()


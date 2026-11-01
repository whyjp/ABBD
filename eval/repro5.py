"""Rerun round 4's setup frictions (E1, E2, E12) and round 5's anchor collision on abbd v3, in a two-module repo.

  python -I eval/repro5.py <repro repo> <abbd exe>

The repo's extractor prints MOD app (anchors app, app/main.py, app/sub) and MOD app/sub (parent app; anchors app/sub,
app/sub/x.py). Each case writes one supplement, runs `abbd setup` and prints the exit code and the last line.
"""
import json
import os
import subprocess
import sys

CASES = {
    "E1 new unit whose parent is a script unit the supplement does not restate": {"units": [
        {"key": "charges", "kind": "DATA", "name": "charges table", "parent": "app/sub",
         "evidence": [{"path": "app/sub/x.py", "lines": "1", "quote": "INSERT INTO charges"}]}], "relations": []},
    "E2 restate a script unit with one anchor its parent also holds": {"units": [
        {"key": "app/sub", "kind": "MOD", "name": "app/sub", "anchors": [{"path": "app/sub"}],
         "responsibility": "Writes charges."}], "relations": []},
    "E12 signature on a non-contract unit": {"units": [
        {"key": "app/sub", "kind": "MOD", "name": "app/sub", "anchors": [{"path": "app/sub"}], "signature": "x"}],
        "relations": []},
    "E13 reader-found inside MOD quoting a file under its parent's anchor": {"units": [
        {"key": "app", "kind": "MOD", "name": "app", "anchors": [{"path": "app/main.py"}]},
        {"key": "app/serve", "kind": "MOD", "name": "serve", "parent": "app",
         "evidence": [{"path": "app/main.py", "lines": "2", "quote": "return 'GET /x'"}]}], "relations": []},
    "E14 reader-found contract keeps its signature": {"units": [
        {"key": "app", "kind": "MOD", "name": "app", "anchors": [{"path": "app/main.py"}]},
        {"key": "api", "kind": "IF", "name": "http api", "parent": "app", "signature": "GET /x",
         "evidence": [{"path": "app/main.py", "lines": "2", "quote": "return 'GET /x'"}]}], "relations": []},
}


def main():
    repo, abbd = sys.argv[1:3]
    for name, supp in CASES.items():
        path = os.path.join(repo, ".abbd", "extractor", "supplement.json")
        json.dump(supp, open(path, "w", encoding="utf-8"), indent=1)
        p = subprocess.run([abbd, "setup", "--supplement", ".abbd/extractor/supplement.json", "--", "python", "-I",
                            ".abbd/extractor/extract.py"], cwd=repo, capture_output=True, text=True, encoding="utf-8")
        lines = (p.stdout + p.stderr).strip().splitlines()
        print("%s\n  exit %d: %s" % (name, p.returncode, " / ".join(lines[-3:])))
        if "E14" in name and p.returncode == 0:
            g = json.load(open(os.path.join(repo, ".abbd", "blueprint", "graph.json"), encoding="utf-8"))
            print("  stored signature:", [u["Signature"] for u in g["units"] if u["Name"] == "http api"])


main()


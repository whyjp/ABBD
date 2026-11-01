"""Measure the path metrics (internal/screen/trace_test.go) on a renderer.

    python eval/render-r1/measure.py REV OUT.tsv

checks REV out into a temporary worktree, gives its drawings the ids and
relations the checker reads (data-id on boxes and frame ports, and each
view's requested relations) when it lacks them, copies this branch's checker in, and
writes the table of every fixture view to OUT.tsv. With ABBD_TRACE_STORE set
to a repository's root, the measures of that repository's views go to
OUT.tsv.store.txt. With REV set to the working tree (".") it measures the
working tree as it is.
"""
import os
import shutil
import subprocess
import sys
import tempfile

here = os.path.dirname(os.path.abspath(__file__))
repo = os.path.dirname(os.path.dirname(here))
rev, out = sys.argv[1], os.path.abspath(sys.argv[2])


def edit(root, path, pairs):
    p = os.path.join(root, path)
    s = open(p, encoding="utf8").read()
    for a, b in pairs:
        if b in s:
            continue
        assert a in s, (path, a)
        s = s.replace(a, b, 1)
    open(p, "w", encoding="utf8", newline="\n").write(s)


def plumb(root):
    """Give an older renderer the ids and relations the checker reads; geometry is untouched."""
    edit(root, "internal/screen/page.html", [
        ('<g class="box {{.Class}}">', '<g class="box {{.Class}}" data-id="{{.ID}}">'),
        ('<g class="port {{.Class}} {{if .Exit}}exit{{else}}entry{{end}}">',
         '<g class="port {{.Class}} {{if .Exit}}exit{{else}}entry{{end}}"{{with .Keys}} data-id="{{join . " "}}"{{end}}>'),
    ])
    edit(root, "internal/screen/overview.go", [
        ("\tFrame *rect `json:\",omitempty\"`\n}", "\tFrame *rect `json:\",omitempty\"`\n\tRelations [][2]string `json:\"-\"`\n}"),
        ("\t// Rels are the stored relations the chip stands for.\n\tRels []string `json:\",omitempty\"`\n}",
         "\t// Rels are the stored relations the chip stands for.\n\tRels []string `json:\",omitempty\"`\n\tKeys []string `json:\"-\"`\n}"),
        ("\td := Drawing{", "\td := Drawing{Relations: wanted(reqs), "),
    ])
    edit(root, "internal/screen/inside.go", [
        ("pt.Contract, pt.Exit, pt.Class, pt.Title = string(p.id), p.exit, p.cls, p.title\n",
         "pt.Contract, pt.Exit, pt.Class, pt.Title, pt.Keys = string(p.id), p.exit, p.cls, p.title, []string{\"port|\" + k}\n"),
        ("\t\t\tq.Contract, q.Href = \"\", \"\"\n", "\t\t\tq.Contract, q.Href = \"\", \"\"\n\t\t\tq.Keys = append(q.Keys, pt.Keys...)\n"),
        ("\td := Drawing{", "\td := Drawing{Relations: wanted(reqs), "),
    ])
    with open(os.path.join(root, "internal/screen/route.go"), "a", encoding="utf8", newline="\n") as f:
        f.write(WANTED)


WANTED = """
func wanted(reqs []wireReq) [][2]string {
	id := func(e end) string {
		if e.node != "" {
			return e.node
		}
		return e.key
	}
	var out [][2]string
	for _, r := range reqs {
		out = append(out, [2]string{id(r.from), id(r.to)})
		if r.kind == "mutual" {
			out = append(out, [2]string{id(r.to), id(r.from)})
		}
	}
	return out
}
"""


def run(root):
    env = dict(os.environ, ABBD_METRICS=out)
    subprocess.run(["go", "test", "./internal/screen/", "-run", "TestTraceEveryRealDrawing", "-count=1"], cwd=root, env=env)
    if env.get("ABBD_TRACE_STORE"):
        r = subprocess.run(["go", "test", "./internal/screen/", "-run", "TestTraceStore", "-count=1", "-v"], cwd=root, env=env, capture_output=True, text=True, encoding="utf8")
        lines = [l.split(": ", 1)[1] for l in r.stdout.splitlines() if "trace_test.go" in l]
        open(out + ".store.txt", "w", encoding="utf8", newline="\n").write("\n".join(lines) + "\n")


if rev == ".":
    run(repo)
    sys.exit(0)
tmp = tempfile.mkdtemp(prefix="abbd-measure-")
wt = os.path.join(tmp, "wt")
subprocess.run(["git", "worktree", "add", "--detach", wt, rev], cwd=repo, check=True)
try:
    if "data-id" not in open(os.path.join(wt, "internal/screen/page.html"), encoding="utf8").read():
        plumb(wt)
    for f in ("trace_test.go", "legible_test.go"):
        shutil.copy(os.path.join(repo, "internal/screen", f), os.path.join(wt, "internal/screen", f))
    for f in os.listdir(os.path.join(wt, "internal/screen")):
        if f.endswith("_test.go") and f not in ("trace_test.go", "legible_test.go", "fixtures_test.go"):
            os.remove(os.path.join(wt, "internal/screen", f))
    fx = os.path.join(wt, "internal/screen/fixtures_test.go")
    s = open(fx, encoding="utf8").read()
    s = s.replace("func TestEveryRealDrawingIsLegible(", "func skipLegible(").replace("func TestEveryRelationIsInSomeView(", "func skipRelations(")
    open(fx, "w", encoding="utf8", newline="\n").write(s)
    run(wt)
finally:
    subprocess.run(["git", "worktree", "remove", "--force", wt], cwd=repo)
    shutil.rmtree(tmp, ignore_errors=True)

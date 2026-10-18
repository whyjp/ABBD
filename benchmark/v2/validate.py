#!/usr/bin/env python3
"""Validate benchmark v2 against its reference patches.

1. For each task: every hidden test function of that task fails before its patch, and after it
   every hidden test of that task and all earlier tasks passes; `go test ./...` and `go vet ./...`
   pass on the reference tree.
2. Flakiness: on the final tree every task's hidden tests pass with -count=N.
3. Rule ablations: each mutant keeps the rule where it was stated but drops it on the paths added
   later, the way an agent that forgot the rule would. The rule tests of the later tasks must
   fail and the feature tests must keep passing, which shows the rule tests are what separates
   the arms.
"""
import argparse
import os
import re
import shutil
import subprocess
import sys
import tempfile

BENCH = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, BENCH)
import run  # noqa: E402

RULE_TEST = re.compile(r"^TestHidden(T\d+)Rule(\w+)$")
NEW_HANDLERS = "func (s *Server) handleImport"


def edit(repo, rel, fn):
    p = os.path.join(repo, rel)
    src = open(p, encoding="utf-8").read()
    out = fn(src)
    assert out != src, f"ablation did not change {rel}"
    open(p, "w", encoding="utf-8", newline="\n").write(out)


def in_new_handlers(fn):
    def apply(src):
        head, sep, tail = src.partition(NEW_HANDLERS)
        assert sep, "api.go lost handleImport"
        return head + sep + fn(tail)
    return apply


def ablate_routes(repo):
    edit(repo, "internal/api/api.go", lambda s: s.replace(
        "\ts.routes = append(s.routes, pattern)", "\tif len(s.routes) < 4 {\n\t\ts.routes = append(s.routes, pattern)\n\t}"))


def ablate_no_test_source(repo):
    edit(repo, "internal/storage/storage.go", lambda s: s.replace(
        "\tif h.Source == heartbeat.LoadTestSource {\n\t\treturn nil\n\t}\n", ""))
    edit(repo, "internal/heartbeat/consumer.go", lambda s: s.replace(
        "\tif !c.seen.firstSeen(h.ID) {", "\tif h.Source == LoadTestSource {\n\t\treturn nil\n\t}\n\tif !c.seen.firstSeen(h.ID) {"))


def ablate_case_insensitive(repo):
    edit(repo, "internal/storage/storage.go", lambda s: s.replace(
        "h, ok := w.rows[heartbeat.Key(id)]", "h, ok := w.rows[id]").replace("key := heartbeat.Key(id)", "key := id"))


def ablate_json_errors(repo):
    edit(repo, "internal/api/api.go", in_new_handlers(
        lambda s: re.sub(r"writeError\(w, ([^,]+), (.+)\)\n", r"http.Error(w, \2, \1)\n", s)))


def ablate_body_limit(repo):
    def fn(src):
        src = src.replace("\tdata, ok := readBody(w, r)\n\tif !ok {\n\t\treturn\n\t}\n", "\tdata, _ := io.ReadAll(r.Body)\n")
        return src
    edit(repo, "internal/api/api.go", in_new_handlers(fn))


ABLATIONS = {
    "RoutesListed": ablate_routes,
    "NoTestSource": ablate_no_test_source,
    "CaseInsensitiveIDs": ablate_case_insensitive,
    "JSONErrors": ablate_json_errors,
    "BodyLimit": ablate_body_limit,
}


def fmt(tests):
    return " ".join(f"{n.removeprefix('TestHidden')}={'PASS' if ok else 'FAIL'}" for n, ok in tests.items())


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--count", type=int, default=5, help="repetitions for the flakiness pass")
    args = ap.parse_args()
    suite = run.load_suite("v2")
    tasks = [t["id"] for t in suite["spec"]["tasks"]]
    work = tempfile.mkdtemp(prefix="abbd-v2-validate-")
    repo, env = run.prepare("reference", work, suite, None)
    problems = []

    print("## fail before, pass after")
    for k, tid in enumerate(tasks):
        before = run.run_hidden(repo, suite["hidden"], tid, env)
        print(f"{tid} before: {fmt(before['tests'])}")
        problems += [f"{tid}: {n} passes before its patch" for n, ok in before["tests"].items() if ok]
        r = run.git(repo, "apply", "--whitespace=nowarn", os.path.join(suite["reference"], tid + ".patch"), check=False)
        if r.returncode != 0:
            raise SystemExit(f"{tid}.patch does not apply: {r.stderr}")
        run.git(repo, "add", "-A")
        run.git(repo, "commit", "-qm", tid)
        hidden, own_ok, vet_ok = run.score(repo, suite["hidden"], tasks[: k + 1], env)
        print(f"{tid} after:  {fmt(hidden[tid]['tests'])}")
        earlier = [t for t in tasks[:k] if not hidden[t]["passed"]]
        print(f"{tid} earlier tasks still green: {not earlier} · go test ./... {own_ok} · go vet ./... {vet_ok}")
        problems += [f"{tid}: {n} fails after its patch" for n, ok in hidden[tid]["tests"].items() if not ok]
        problems += [f"{tid}: earlier {t} regressed" for t in earlier]
        problems += [f"{tid}: go test ./... red"] * (not own_ok) + [f"{tid}: go vet ./... red"] * (not vet_ok)

    print(f"\n## flakiness (-count={args.count}, final tree)")
    for tid in tasks:
        res = run.run_hidden(repo, suite["hidden"], tid, env, count=args.count)
        print(f"{tid}: {sum(res['tests'].values())}/{len(res['tests'])} stable")
        problems += [f"{tid}: flaky or failing {n}" for n, ok in res["tests"].items() if not ok]

    print("\n## rule ablations (rule kept where stated, dropped on later paths)")
    for rule, ablate in ABLATIONS.items():
        ablate(repo)
        failing = []
        for tid in tasks:
            failing += [n for n, ok in run.run_hidden(repo, suite["hidden"], tid, env)["tests"].items() if not ok]
        run.git(repo, "checkout", "--", ".")
        caught = [n for n in failing if (m := RULE_TEST.match(n)) and m.group(2) == rule]
        stray = [n for n in failing if n not in caught]
        print(f"{rule}: fails {', '.join(n.removeprefix('TestHidden') for n in caught) or 'nothing'}"
              f"{' · STRAY ' + ', '.join(stray) if stray else ''}")
        problems += [f"ablation {rule}: no rule test caught it"] * (not caught) + [f"ablation {rule}: non-rule test failed {n}" for n in stray]

    shutil.rmtree(work, ignore_errors=True)
    print("\n## verdict")
    print("\n".join(problems) or "all checks pass")
    sys.exit(1 if problems else 0)


if __name__ == "__main__":
    sys.stdout.reconfigure(encoding="utf-8")
    main()

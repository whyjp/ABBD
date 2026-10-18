#!/usr/bin/env bash
# Does `.abbd/` in a snapshot change any official number? Scores the same reference solution twice, without
# and with a populated `.abbd/` (abbd init + one abbd run), and diffs the evaluator's correctness report,
# the per-snapshot quality metrics, and the pinned scb-check report. No model calls.
#   ABBD_BIN=<abbd-linux> bash abbd_scoring_check.sh [problem] [checkpoint]
set -uo pipefail
: "${ABBD_BIN:?}"
P=${1:-file_backup} C=${2:-1}
WORK=${WORK:-$HOME/bench/scb}
ENV_CFG=${ENV_CFG:-$WORK/configs/environment-abbd.yaml}
export PATH=$HOME/.local/bin:$PATH SCBENCH_PROBLEMS_PATH=$WORK/scb-problems
OUT=$WORK/abbd-check/$P-$C
rm -rf "$OUT" && mkdir -p "$OUT"
for v in plain abbd; do cp -r "$SCBENCH_PROBLEMS_PATH/$P/solutions/checkpoint_$C" "$OUT/$v"; done
(cd "$OUT/abbd" && "$ABBD_BIN" init > "$OUT/abbd-init.log" 2>&1 && "$ABBD_BIN" run "dry-run feature request" > "$OUT/abbd-run.log" 2>&1)
echo "abbd files: $(find "$OUT/abbd/.abbd" -type f | wc -l), extensions: $(find "$OUT/abbd/.abbd" -type f | sed 's/.*\.//' | sort | uniq -c | tr -s ' ' | tr '\n' ',')"
cd "$WORK/slop-code-bench"
for v in plain abbd; do
  uv run -q slop-code eval-snapshot "$OUT/$v" -o "$OUT/eval-$v" -p "$P" -c "$C" -e "$ENV_CFG" > "$OUT/eval-$v.log" 2>&1
  uvx -q scb-check==0.1.3 check --report --include-all "$OUT/$v" > "$OUT/scb-check-$v.json" 2> "$OUT/scb-check-$v.err"
done
uv run -q python -I - "$OUT" <<'EOF'
import json, sys
from pathlib import Path
out = Path(sys.argv[1])
def load(p):
    try:
        text = p.read_text()
    except OSError:
        return {"missing": p.name}
    try:
        return json.loads(text)
    except ValueError:
        return [json.loads(line) for line in text.splitlines() if line.strip()]
def strip(x):
    drop = {"duration", "timestamp", "started", "completed", "elapsed", "path", "snapshot", "save_dir", "submission_path", "created"}
    if isinstance(x, dict):
        return {k: strip(v) for k, v in x.items() if k not in drop}
    if isinstance(x, list) and all(isinstance(v, str) for v in x):
        return sorted(x)  # file lists come out in filesystem order
    if isinstance(x, list):
        return [strip(v) for v in x]
    return x
pairs = {"scb-check 0.1.3 report": "scb-check-{}.json"}
for qp in sorted((out / "eval-plain").rglob("*.json*")):
    rel = qp.relative_to(out / "eval-plain").as_posix()
    pairs[f"eval output {rel}"] = "eval-{}/" + rel
def diffs(a, b, at=""):
    if isinstance(a, dict) and isinstance(b, dict):
        return [d for k in sorted(set(a) | set(b)) for d in diffs(a.get(k), b.get(k), f"{at}.{k}")]
    if isinstance(a, list) and isinstance(b, list) and len(a) == len(b):
        return [d for i, (x, y) in enumerate(zip(a, b)) for d in diffs(x, y, f"{at}[{i}]")]
    return [] if a == b else [f"{at}: {str(a)[:80]} -> {str(b)[:80]}"]
for name, pat in pairs.items():
    found = diffs(strip(load(out / pat.format("plain"))), strip(load(out / pat.format("abbd"))))
    print(f"{'DIFF' if found else 'SAME'}  {name}")
    for d in found[:8]:
        print(f"      {d}")
EOF

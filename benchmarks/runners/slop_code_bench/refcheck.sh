#!/usr/bin/env bash
# No-model evaluator check: score each checkpoint's reference solution, and an empty submission, with the
# official evaluator and the ABBD environment image. Run after setup.sh.
#   bash refcheck.sh file_backup execution_server
set -uo pipefail
WORK=${WORK:-$HOME/bench/scb}
ENV_CFG=${ENV_CFG:-$WORK/configs/environment-abbd.yaml}
export PATH=$HOME/.local/bin:$PATH SCBENCH_PROBLEMS_PATH=$WORK/scb-problems
OUT=$WORK/refcheck
cd "$WORK/slop-code-bench"
for p in "$@"; do
  rm -rf "$OUT/ref/$p" "$OUT/empty/$p"
  for d in "$SCBENCH_PROBLEMS_PATH/$p"/solutions/checkpoint_*; do
    mkdir -p "$OUT/ref/$p/$(basename "$d")"
    cp -r "$d" "$OUT/ref/$p/$(basename "$d")/snapshot"
  done
  s=$(date +%s)
  uv run -q slop-code eval-problem "$OUT/ref/$p" -p "$p" -e "$ENV_CFG" > "$OUT/ref-$p.log" 2>&1
  echo "ref $p exit=$? elapsed=$(( $(date +%s) - s ))s"
  mkdir -p "$OUT/empty/$p/snapshot"
  s=$(date +%s)
  uv run -q slop-code eval-snapshot "$OUT/empty/$p/snapshot" -o "$OUT/empty/$p/eval" -p "$p" -c 1 -e "$ENV_CFG" --json \
    > "$OUT/empty-$p.log" 2>&1
  echo "empty $p checkpoint_1 exit=$? elapsed=$(( $(date +%s) - s ))s"
done

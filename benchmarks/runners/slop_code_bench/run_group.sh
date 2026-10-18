#!/usr/bin/env bash
# Pilot chunk runner: one group, problems in order, one problem at a time (concurrency 1), subscription auth.
#   CCCV=<cccv repo> bash run_group.sh <group> <label> <problem>...
# Each (group, problem) gets its own run dir, so a usage-limit stop resumes only that problem. Per problem:
#   complete       -> skipped
#   limit_stopped  -> slop-code run --resume <dir> (workspace and .abbd/ come back from the last good snapshot)
#   agent_error    -> kept as is and never resumed (it is a real result)
#   partial        -> resumed (an interrupted run with no error)
#   not_started    -> fresh run
# After each problem the status line from classify_run.py is appended to <label>/status.jsonl. The loop stops
# at the first limit_stopped problem, so the rest of the chunk waits for the reset instead of failing fast.
# dryrun.sh calls this same script with AGENT_DIR/ENV_CFG pointing at its fake-claude configs and
# DRYRUN_TOKEN set, in which case ~/bench/.env is not read.
set -uo pipefail
: "${CCCV:?}"
G=$1 LABEL=$2; shift 2
WORK=${WORK:-$HOME/bench/scb}
AGENT_DIR=${AGENT_DIR:-$CCCV/benchmarks/adapters/for_my_harness/scb}
ENV_CFG=${ENV_CFG:-$WORK/configs/environment-abbd.yaml}
RUNNER=$CCCV/benchmarks/runners/slop_code_bench
export PATH=$HOME/.local/bin:$PATH SCBENCH_PROBLEMS_PATH=$WORK/scb-problems
if [ -n "${DRYRUN_TOKEN:-}" ]; then export CLAUDE_CODE_OAUTH_TOKEN=$DRYRUN_TOKEN; else set -a; . "$HOME/bench/.env"; set +a; fi
[ -n "${CLAUDE_CODE_OAUTH_TOKEN:-}" ] || { echo "CLAUDE_CODE_OAUTH_TOKEN missing in ~/bench/.env" >&2; exit 2; }
unset ANTHROPIC_API_KEY ANTHROPIC_AUTH_TOKEN
OUT=$WORK/outputs/$LABEL
mkdir -p "$OUT"
cd "$WORK/slop-code-bench"
for P in "$@"; do
  DIR=$OUT/group-$G/$P
  STATE=$(uv run -q python -I "$RUNNER/classify_run.py" "$DIR" "$P" | sed -n 's/.*"status": "\([a-z_]*\)".*/\1/p')
  echo "$(date -Is) group=$G problem=$P before=$STATE"
  case $STATE in
    complete|agent_error) continue ;;
    limit_stopped|partial) uv run slop-code --quiet run --resume "$DIR" --num-workers 1 --no-live-progress ;;
    *) uv run slop-code --quiet run \
         --agent "$AGENT_DIR/agent-group-$G.yaml" \
         --environment "$ENV_CFG" \
         --prompt configs/prompts/just-solve.jinja \
         --model claude_code_oauth/sonnet-5.5 \
         --problem "$P" --num-workers 1 --no-live-progress \
         thinking=none save_dir="$OUT/group-$G" save_template="$P" ;;
  esac
  LINE=$(uv run -q python -I "$RUNNER/classify_run.py" "$DIR" "$P")
  echo "$LINE" | tee -a "$OUT/status.jsonl"
  case $LINE in *'"status": "limit_stopped"'*) echo "usage limit hit; rerun this command after the reset" >&2; exit 3 ;; esac
done

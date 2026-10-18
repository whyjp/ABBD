#!/usr/bin/env bash
# Cross-harness TB2 run: 15 pilot tasks x REPS reps x ARMS, sonnet 5.5, Claude Code 2.1.302, subscription auth.
# See cross_harness.md. Run prepare_cross_harness.sh first. Reps are the outer loop so drift and usage
# limits hit every arm alike. A finished job (result.json present) is skipped, so a rerun resumes.
set -u
export PATH="$HOME/.local/bin:/usr/local/bin:/usr/bin:/bin"
set -a; . "$HOME/bench/.env"; set +a          # CLAUDE_CODE_OAUTH_TOKEN; never echoed
export CLAUDE_FORCE_OAUTH=1
REPO=/mnt/c/Users/cxx/orca/projects/cccv
PUB=/mnt/c/Users/cxx/orca/projects/abbd-bench-pub
XH_SRC=${XH_SRC:-$HOME/bench/xh-src}
TAG=${TAG:-4ec437c}
export PYTHONPATH=$REPO
TASKS=$(python3 -c "import json;m=json.load(open('$REPO/benchmarks/manifests/frozen_task_manifest.json'));print(' '.join('-i '+t['id'] for t in m['terminal_bench_2']['pilot']))")
COMMON="-d terminal-bench@2.0 -m anthropic/claude-sonnet-5-5 --ak version=2.1.302 -n ${N:-1} -o $HOME/bench/jobs -q"
MOD=benchmarks.adapters.for_my_harness.harbor_agent
LOG=$HOME/bench/xh-tb2-$TAG.log
for REP in ${REPS:-1 2 3}; do
  for ARM in ${ARMS:-A B E superpowers gstack}; do
    JOB=xh-tb2-$ARM-r$REP-$TAG
    [ -f "$HOME/bench/jobs/$JOB/result.json" ] && { echo "$(date -Is) skip $JOB (done)" >> "$LOG"; continue; }
    unset ABBD_PLUGIN ABBD_GROUP ABBD_BIN GSTACK_SRC GSTACK_BUN
    case $ARM in
      A) AGENT="-a claude-code" ;;
      B) export ABBD_PLUGIN=/mnt/c/Users/cxx/.claude/plugins/cache/pstack-claude/pstack/0.9.93; AGENT="-a $MOD:PstackClaudeCode" ;;
      E) export ABBD_GROUP=E ABBD_PLUGIN=$PUB/arms/E ABBD_BIN=$PUB/abbd-linux; AGENT="-a $MOD:AbbdClaudeCode" ;;
      superpowers) export ABBD_PLUGIN=$XH_SRC/superpowers-bb92a77; AGENT="-a $MOD:SuperpowersClaudeCode" ;;
      gstack) export GSTACK_SRC=$XH_SRC/gstack-3a64ab8 GSTACK_BUN=$XH_SRC/bun-1.4.2/bun; AGENT="-a $MOD:GstackClaudeCode" ;;
      *) echo "unknown arm $ARM" >&2; exit 2 ;;
    esac
    echo "$(date -Is) start $JOB" >> "$LOG"
    harbor run $COMMON $TASKS $AGENT --job-name "$JOB" >> "$LOG" 2>&1
    lim=$(grep -lsiE 'usage limit|rate_limit_error|"api_error_status":429|reached your .*limit' "$HOME"/bench/jobs/$JOB/*/agent/claude-code.txt | wc -l)
    echo "$(date -Is) end $JOB limit_hits=$lim" >> "$LOG"
    if [ "$lim" -gt 0 ]; then echo "$(date -Is) STOP: usage limit in $JOB; rerun after reset (those trials = limit_stopped)" >> "$LOG"; exit 3; fi
  done
done
echo "$(date -Is) all done" >> "$LOG"

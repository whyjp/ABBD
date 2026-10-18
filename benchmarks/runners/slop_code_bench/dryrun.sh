#!/usr/bin/env bash
# End-to-end dry run of the real runner, agent configs and image with a fake `claude` (no model calls).
#   CCCV=<cccv repo> bash dryrun.sh <group> <label> [problem]      env: FAKE_LIMIT_AT=<checkpoint>
# Output lands in ~/bench/scb/outputs/<label>/ like a real run; use a label that starts with dryrun-.
# The agent config is the real agent-group-<G>.yaml with only the final claude binary swapped for the fake.
# Auth is a dummy CLAUDE_CODE_OAUTH_TOKEN; the real token in ~/bench/.env is never read here.
set -euo pipefail
: "${CCCV:?}"
G=$1 LABEL=$2 PROBLEM=${3:-file_backup}
WORK=${WORK:-$HOME/bench/scb}
SCB=$CCCV/benchmarks/adapters/for_my_harness/scb
export PATH=$HOME/.local/bin:$PATH SCBENCH_PROBLEMS_PATH=$WORK/scb-problems
TAG=$(sed -n 's#^  image: abbd-bench/scb-python3.12:##p' "$WORK/configs/environment-abbd.yaml")
DRY=$WORK/dryrun && mkdir -p "$DRY"

CTX=$DRY/ctx-$PROBLEM
rm -rf "$CTX" && mkdir -p "$CTX/solutions/specs"
sed 's/\r$//' "$SCB/dryrun/Dockerfile" > "$CTX/Dockerfile"
sed 's/\r$//' "$SCB/dryrun/fake_claude.py" > "$CTX/fake_claude.py"
cp -r "$SCBENCH_PROBLEMS_PATH/$PROBLEM"/solutions/checkpoint_* "$CTX/solutions/"
cp "$SCBENCH_PROBLEMS_PATH/$PROBLEM"/checkpoint_*.md "$CTX/solutions/specs/"
# slop-code reuses its derived images by env name, so the name carries a hash of this context too.
DTAG=$TAG-$(cd "$CTX" && find . -type f -print0 | sort -z | xargs -0 sha256sum | sha256sum | cut -c1-8)
docker build -q --build-arg BASE="abbd-bench/scb-python3.12:$TAG" -t "abbd-bench/scb-python3.12-dryrun:$DTAG" "$CTX" >/dev/null
sed -e "s#^name: .*#name: python3.12-abbd-dryrun-$DTAG#" \
    -e "s#^  image: .*#  image: abbd-bench/scb-python3.12-dryrun:$DTAG#" \
    "$WORK/configs/environment-abbd.yaml" > "$DRY/environment-dryrun-$PROBLEM.yaml"

cd "$WORK/slop-code-bench"
uv run -q python -I - "$SCB/agent-group-$G.yaml" "$DRY/agent-group-$G.yaml" "$PROBLEM" <<'EOF'
import sys, os, yaml
src, dst, problem = sys.argv[1:]
cfg = yaml.safe_load(open(src))
env = dict(cfg.get("env") or {})
env["FAKE_SOLUTIONS"] = "/opt/fake/solutions"
for key in ("FAKE_LIMIT_AT", "FAKE_LIMIT_UNTIL"):
    if os.environ.get(key):
        env[key] = os.environ[key]
if cfg["binary"] == "claude":
    cfg["binary"] = "/opt/fake/claude"
else:
    env["ABBD_CLAUDE_BIN"] = "/opt/fake/claude"
cfg["env"] = env
yaml.safe_dump(cfg, open(dst, "w"), sort_keys=False)
EOF

s=$(date +%s)
AGENT_DIR=$DRY ENV_CFG="$DRY/environment-dryrun-$PROBLEM.yaml" DRYRUN_TOKEN=dryrun-not-a-token \
  bash <(sed 's/\r$//' "$CCCV/benchmarks/runners/slop_code_bench/run_group.sh") "$G" "$LABEL" "$PROBLEM" \
  && rc=0 || rc=$?
echo "dryrun group=$G problem=$PROBLEM run_group_exit=$rc elapsed=$(( $(date +%s) - s ))s"

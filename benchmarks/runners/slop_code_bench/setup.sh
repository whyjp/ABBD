#!/usr/bin/env bash
# Idempotent setup for Track C on a Linux Docker host (WSL2 here). No model calls.
#   CCCV=<cccv repo> ABBD_BIN=<abbd-linux> ARMS=<dir with C D E> PSTACK=<pstack plugin dir> bash setup.sh
# Prints the rendered environment config path and the image tag on the last line.
set -euo pipefail
: "${CCCV:?}" "${ABBD_BIN:?}" "${ARMS:?}" "${PSTACK:?}"
WORK=${WORK:-$HOME/bench/scb}
RUNNER_COMMIT=31ceea3add480edb33431e70475c4c70597e6b31
PROBLEMS_COMMIT=9cd9ca3a51c3d3e2a99d2488a25baf73a2204451
SCB=$CCCV/benchmarks/adapters/for_my_harness/scb
export PATH=$HOME/.local/bin:$PATH
mkdir -p "$WORK/configs"
cd "$WORK"

pin() {
  [ -d "$2" ] || git clone -q "$1" "$2"
  git -C "$2" fetch -q origin "$3" 2>/dev/null || true
  git -C "$2" checkout -q "$3"
  [ "$(git -C "$2" rev-parse HEAD)" = "$3" ] || { echo "$2 not at $3" >&2; exit 1; }
}
pin https://github.com/SprocketLab/slop-code-bench slop-code-bench $RUNNER_COMMIT
pin https://github.com/gabeorlanski/scb-problems scb-problems $PROBLEMS_COMMIT
(cd slop-code-bench && uv sync -q)
install -m 644 "$SCB/models/sonnet-5.5.yaml" slop-code-bench/configs/models/sonnet-5.5.yaml

uv run -q --project slop-code-bench python -I "$CCCV/benchmarks/runners/slop_code_bench/check_problems.py" \
  scb-problems "$CCCV/benchmarks/manifests/frozen_task_manifest.json"

STAGE=$WORK/stage
rm -rf "$STAGE" && mkdir -p "$STAGE/plugins"
install -m 755 "$ABBD_BIN" "$STAGE/abbd-linux"
sed 's/\r$//' "$SCB/claude-abbd" > "$STAGE/claude-abbd"
sed 's/\r$//' "$SCB/Dockerfile" > "$STAGE/Dockerfile"
for g in C D E; do cp -r "$ARMS/$g" "$STAGE/plugins/$g"; done
cp -r "$PSTACK" "$STAGE/plugins/pstack"
rm -rf "$STAGE/plugins/pstack/.in_use"
find "$STAGE/plugins" -name '*.sh' -exec sed -i 's/\r$//' {} +
TAG=$(cd "$STAGE" && find . -type f -print0 | sort -z | xargs -0 sha256sum | sha256sum | cut -c1-12)
docker image inspect "abbd-bench/scb-python3.12:$TAG" >/dev/null 2>&1 \
  || docker build -q -t "abbd-bench/scb-python3.12:$TAG" "$STAGE"
sed "s/@TAG@/$TAG/g" "$SCB/environment-abbd-python3.12.yaml" > "$WORK/configs/environment-abbd.yaml"
echo "env=$WORK/configs/environment-abbd.yaml tag=$TAG"

#!/usr/bin/env bash
# Stage the pinned harness sources for the cross-harness TB2 arms (see cross_harness.md).
# Runs on the WSL/Linux Docker host. Idempotent. No model calls.
#   XH_SRC (default ~/bench/xh-src) receives:
#     superpowers-bb92a77/   plugin dir (git archive of obra/superpowers @ pin, no .git)
#     gstack-3a64ab8/        depth-1 checkout of garrytan/gstack @ pin (.git kept: ./setup runs git ls-files)
#     bun-1.4.2/bun          Bun binary, archive sha256 checked against gstack's own bin/gstack-bun-version.sh
set -euo pipefail
XH_SRC=${XH_SRC:-$HOME/bench/xh-src}
SP_SHA=bb92a77741419a4ab5f06e711a283343f1ada0c3   # superpowers v7.0.0
GS_SHA=3a64ab8a1f6bfa613546a99522f144ac724771b5   # gstack v1.91.139.0
BUN_VER=1.4.2                                      # GSTACK_BUN_TESTED at the gstack pin
mkdir -p "$XH_SRC"

sp="$XH_SRC/superpowers-${SP_SHA:0:7}"
if [ ! -f "$sp/.claude-plugin/plugin.json" ]; then
  tmp=$(mktemp -d); git -C "$tmp" init -q; git -C "$tmp" fetch -q --depth 1 https://github.com/obra/superpowers.git "$SP_SHA"
  mkdir -p "$sp"; git -C "$tmp" archive FETCH_HEAD | tar -x -C "$sp"; rm -rf "$tmp"
fi

gs="$XH_SRC/gstack-${GS_SHA:0:7}"
if [ ! -x "$gs/setup" ]; then
  rm -rf "$gs"; git init -q "$gs"; git -C "$gs" fetch -q --depth 1 https://github.com/garrytan/gstack.git "$GS_SHA"
  git -C "$gs" checkout -q FETCH_HEAD
fi
[ "$(git -C "$gs" rev-parse HEAD)" = "$GS_SHA" ] || { echo "gstack checkout is not at $GS_SHA" >&2; exit 1; }
[ -z "$(git -C "$gs" status --porcelain)" ] || { echo "gstack checkout is dirty (a setup ran in it?): $gs" >&2; exit 1; }

bun_dir="$XH_SRC/bun-$BUN_VER"
if [ ! -x "$bun_dir/bun" ]; then
  arch=linux-x64; grep -q avx2 /proc/cpuinfo || arch=linux-x64-baseline
  want=$(bash -c ". '$gs/bin/gstack-bun-version.sh'; gstack_bun_archive_sha256 $arch $BUN_VER")
  tmp=$(mktemp -d); curl -fsSL -o "$tmp/bun.zip" "https://github.com/oven-sh/bun/releases/download/bun-v$BUN_VER/bun-$arch.zip"
  echo "$want  $tmp/bun.zip" | sha256sum -c -
  python3 -I -c "import sys,zipfile; zipfile.ZipFile(sys.argv[1]).extractall(sys.argv[2])" "$tmp/bun.zip" "$tmp"
  mkdir -p "$bun_dir"; install -m 755 "$tmp/bun-$arch/bun" "$bun_dir/bun"; rm -rf "$tmp"
fi

echo "superpowers: $sp"; claude plugin validate "$sp/.claude-plugin/plugin.json"
echo "gstack:      $gs ($(cat "$gs/VERSION"))"
echo "bun:         $bun_dir/bun ($("$bun_dir/bun" --version))"

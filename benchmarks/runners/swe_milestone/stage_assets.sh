#!/bin/sh
# Copy the abbd binary and the B to E plugin dirs onto the WSL ext4 disk, where Docker bind-mounts them
# read-only into each agent container. Rerun after rebuilding abbd-linux or the arms; record the sha256.
set -eu
A="$HOME/bench/swe-milestone/assets"
PUB=/mnt/c/Users/cxx/orca/projects/abbd-bench-pub
PSTACK=/mnt/c/Users/cxx/.claude/plugins/cache/pstack-claude/pstack/0.9.93
rm -rf "$A" && mkdir -p "$A/plugins"
cp "$PUB/abbd-linux" "$A/abbd-linux"
cp -r "$PSTACK" "$A/plugins/B"
for g in C D E; do cp -r "$PUB/arms/$g" "$A/plugins/$g"; done
chmod -R u+rwX,go+rX,go-w "$A"
chmod 755 "$A/abbd-linux"
sha256sum "$A/abbd-linux"
for g in B C D E; do echo "$g: $(ls -A "$A/plugins/$g" | tr '\n' ' ')"; done

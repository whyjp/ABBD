#!/bin/sh
# rerender.sh BIN OUT NAME=RUN...: copy each finished run to OUT/NAME and
# sync it with BIN, so a renderer change is drawn on every run at once. The
# runs are left as they are. Prints each copy's sync tail and lint totals.
set -e
bin=$1; out=$2; shift 2
mkdir -p "$out"
for a in "$@"; do
  name=${a%%=*}; run=${a#*=}
  [ -e "$out/$name" ] && { echo "exists: $out/$name" >&2; exit 1; }
  cp -r "$run" "$out/$name"
  (cd "$out/$name" && "$bin" sync > /dev/null && "$bin" lint | tail -n 1 | sed "s|^|$name: |")
done

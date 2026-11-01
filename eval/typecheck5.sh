#!/bin/sh
# Typecheck an Orca tree with the dependencies installed in the base clone (pnpm --ignore-scripts).
#   sh typecheck5.sh <orca tree> <out file>
# Prints one line per tsconfig: name, exit code, seconds, error count.
tree=$1; out=$2
cd "$tree" || exit 2
: > "$out"
for p in node web cli; do
  s=$(date +%s)
  node node_modules/typescript/bin/tsc --noEmit -p config/tsconfig.$p.json --composite false > "$out.$p" 2>&1
  rc=$?
  n=$(grep -c "error TS" "$out.$p")
  echo "$p exit=$rc ${n}errors $(( $(date +%s) - s ))s" >> "$out"
done
cat "$out"

#!/bin/sh
# Set ABBD v3 up on an Orca checkout from the round-5 extractor and the round-4 supplement.
#   sh eval/setup5.sh <orca checkout> <abbd exe> <eval dir> <round-4 semantic dir>
set -e
orca=$1; abbd=$2; ev=$3; sem=$4
mkdir -p "$orca/.abbd/extractor" "$ev/runs-5"
cp "$ev/orca-extractor-v5/extract.py" "$orca/.abbd/extractor/"
cd "$orca"
s=$(date +%s)
python -I .abbd/extractor/extract.py > "$ev/runs-5/script-extraction.json"
echo "extract $(( $(date +%s) - s ))s"
python -I "$ev/supplement4.py" "$orca" "$ev/runs-5/script-extraction.json" "$sem" "$ev/runs-5/supplement-from-round4.json" "$ev/runs-5/semantic-check.json"
python -I "$ev/fixsupp5.py" "$ev/runs-5/script-extraction.json" "$ev/runs-5/supplement-from-round4.json" "$ev/orca-extractor-v5/supplement.json"
cp "$ev/orca-extractor-v5/supplement.json" .abbd/extractor/
s=$(date +%s)
set +e
"$abbd" setup --supplement .abbd/extractor/supplement.json -- python -I .abbd/extractor/extract.py > "$ev/runs-5/setup.txt" 2>&1
rc=$?
echo "setup exit $rc $(( $(date +%s) - s ))s"
tail -15 "$ev/runs-5/setup.txt"

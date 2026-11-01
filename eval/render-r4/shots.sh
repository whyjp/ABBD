#!/bin/sh
# shots.sh BASE TAG: take round 4's screenshots of the pages served at BASE
# (for example http://127.0.0.1:28474/pages, after python -m http.server
# 28474 --bind 127.0.0.1 one folder up) into eval/render-r4/*-TAG.png.
set -e
B=$1; T=$2; O=$(dirname "$0")
python $O/shot.py $B/self.html overview $O/self-$T.png
python $O/shot.py $B/orca.html m-MOD-0042 $O/orca-main-$T.png
python $O/shot.py $B/orca.html f-MOD-0042--MOD-0052 $O/orca-main-focus-ipc-$T.png
python $O/shot.py $B/orca.html f-MOD-0042--MOD-0054 $O/orca-main-focus-persistence-$T.png
python $O/shot.py $B/pstack.html overview $O/pstack-$T.png
python $O/shot.py $B/pstack.html f-overview--MOD-0003 $O/pstack-focus-principles-$T.png

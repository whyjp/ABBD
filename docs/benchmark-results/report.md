# ABBD benchmark pilot · combined

model `sonnet` · budget/task $4.0 · abbd `pstack arm 477df1e / abbd arm 459fd41` · tasks T01, T02, T03, T04, T05, T06, T07, T08, T09, T10

| arm | first-try pass | final pass | regressions | own tests green (steps) | session errors | cost USD | turns | minutes |
|---|---|---|---|---|---|---|---|---|
| pstack | 10/10 | 10/10 | 0 | 10/10 | 0 | 1.46 | 103 | 4.0 |
| abbd | 10/10 | 10/10 | 0 | 10/10 | 0 | 1.3 | 68 | 4.0 |

## pstack

- cumulative hidden passes after each task: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]
- regressions: none
- T01: failing — · own tests green · $0.12918459999999998 · turns 7 · error False
- T02: failing — · own tests green · $0.1544982 · turns 16 · error False
- T03: failing — · own tests green · $0.1201566 · turns 7 · error False
- T04: failing — · own tests green · $0.14941830000000003 · turns 13 · error False
- T05: failing — · own tests green · $0.1642977 · turns 10 · error False
- T06: failing — · own tests green · $0.16414839999999997 · turns 11 · error False
- T07: failing — · own tests green · $0.14763690000000002 · turns 8 · error False
- T08: failing — · own tests green · $0.1438895 · turns 10 · error False
- T09: failing — · own tests green · $0.1398289 · turns 11 · error False
- T10: failing — · own tests green · $0.14942769999999997 · turns 10 · error False

## abbd

- cumulative hidden passes after each task: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]
- regressions: none
- T01: failing — · own tests green · $0.1638807 · turns 14 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 1 active None
- T02: failing — · own tests green · $0.1503002 · turns 6 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 3 active None
- T03: failing — · own tests green · $0.1156924 · turns 5 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 5 active None
- T04: failing — · own tests green · $0.1092871 · turns 7 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 6 active None
- T05: failing — · own tests green · $0.1341916 · turns 7 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 8 active None
- T06: failing — · own tests green · $0.1249932 · turns 5 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 9 active None
- T07: failing — · own tests green · $0.1179392 · turns 5 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 10 active None
- T08: failing — · own tests green · $0.1222302 · turns 5 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 11 active None
- T09: failing — · own tests green · $0.1079951 · turns 5 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 12 active None
- T10: failing — · own tests green · $0.1493062 · turns 9 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 13 active None

Caveats: one run per arm (no repetition, no confidence interval); the arms share the host CLAUDE.md; hidden tests define correctness only for what they check. Treat differences as hypotheses for the next rung, not results.

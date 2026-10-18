# ABBD benchmark v2 · combined

model `sonnet` · budget/task $4.0 · abbd `86fe734` · reps 2 · tasks T01, T02, T03, T04, T05, T06, T07, T08, T09, T10, T11, T12

Each cell is the mean over reps, with [min, max] when there is more than one rep. Feature first-try counts a task's feature tests at its own step; rule first-try counts a task's rule tests at its own step (did the new path honor rules stated earlier); rule cumulative sums rule passes over every step's applicable rule tests; regressions count test functions that passed at one step and failed at the next.

| arm | runs | feature first-try | feature final | rule first-try | rule cumulative | rule final | regressions | own tests green | session errors | cost USD | turns | minutes |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| baseline | 2 | 100% [100%, 100%] | 100% [100%, 100%] | 98% [95%, 100%] | 96% [93%, 100%] | 98% [95%, 100%] | 0 [0, 0] | 12 [12, 12]/12 | 0 [0, 0] | 1.57 [1.57, 1.58] | 103 [101, 105] | 5.5 [5.4, 5.5] |
| pstack | 2 | 100% [100%, 100%] | 100% [100%, 100%] | 98% [95%, 100%] | 96% [93%, 100%] | 98% [95%, 100%] | 0 [0, 0] | 12 [12, 12]/12 | 0 [0, 0] | 2.03 [1.98, 2.07] | 122 [122, 123] | 5.8 [5.5, 6.0] |
| superpowers | 2 | 100% [100%, 100%] | 100% [100%, 100%] | 100% [100%, 100%] | 100% [100%, 100%] | 100% [100%, 100%] | 0 [0, 0] | 12 [12, 12]/12 | 0 [0, 0] | 1.86 [1.82, 1.90] | 124 [120, 128] | 5.6 [5.6, 5.6] |
| gstack | 2 | 100% [100%, 100%] | 100% [100%, 100%] | 100% [100%, 100%] | 100% [100%, 100%] | 100% [100%, 100%] | 0 [0, 0] | 12 [12, 12]/12 | 0 [0, 0] | 1.75 [1.72, 1.78] | 120 [115, 125] | 5.6 [5.5, 5.7] |
| abbd | 2 | 100% [100%, 100%] | 100% [100%, 100%] | 100% [100%, 100%] | 100% [100%, 100%] | 100% [100%, 100%] | 0 [0, 0] | 12 [12, 12]/12 | 0 [0, 0] | 1.92 [1.91, 1.93] | 86 [84, 88] | 6.5 [6.4, 6.6] |

## Rules (final step: rule tests passed / applicable)

| rule | stated in | baseline | pstack | superpowers | gstack | abbd |
|---|---|---|---|---|---|---|
| JSONErrors | T04 | 100% [100%, 100%] of 6 | 100% [100%, 100%] of 6 | 100% [100%, 100%] of 6 | 100% [100%, 100%] of 6 | 100% [100%, 100%] of 6 |
| NoTestSource | T02, conflict T11 | 100% [100%, 100%] of 3 | 100% [100%, 100%] of 3 | 100% [100%, 100%] of 3 | 100% [100%, 100%] of 3 | 100% [100%, 100%] of 3 |
| CaseInsensitiveIDs | T03 | 100% [100%, 100%] of 5 | 100% [100%, 100%] of 5 | 100% [100%, 100%] of 5 | 100% [100%, 100%] of 5 | 100% [100%, 100%] of 5 |
| BodyLimit | T05 | 75% [50%, 100%] of 2 | 75% [50%, 100%] of 2 | 100% [100%, 100%] of 2 | 100% [100%, 100%] of 2 | 100% [100%, 100%] of 2 |
| RoutesListed | T01 | 100% [100%, 100%] of 5 | 100% [100%, 100%] of 5 | 100% [100%, 100%] of 5 | 100% [100%, 100%] of 5 | 100% [100%, 100%] of 5 |

- **RoutesListed**: Every route the server serves appears in GET /routes as "METHOD /path".
- **NoTestSource**: Heartbeats with source "test" are never stored, by any path.
- **CaseInsensitiveIDs**: Heartbeat ids are case-insensitive everywhere: "ABC" and "abc" are the same heartbeat.
- **JSONErrors**: Every HTTP error response is application/json {"error": "..."} with the right status.
- **BodyLimit**: Request bodies over 1024 bytes are rejected with 413 on every endpoint that accepts a body.

## baseline-r1

- rule tests passing after each task: 0/0, 0/0, 0/0, 0/0, 1/1, 1/1, 6/6, 9/9, 11/11, 14/14, 19/19, 21/21
- regressions: none
- T01: failing none · own tests green · $0.1348795 · turns 8 · error False
- T02: failing none · own tests green · $0.10958400000000001 · turns 5 · error False
- T03: failing none · own tests green · $0.1696346 · turns 20 · error False
- T04: failing none · own tests green · $0.11631240000000001 · turns 6 · error False
- T05: failing none · own tests green · $0.1044096 · turns 5 · error False
- T06: failing none · own tests green · $0.0802626 · turns 6 · error False
- T07: failing none · own tests green · $0.1192271 · turns 4 · error False
- T08: failing none · own tests green · $0.2190843 · turns 21 · error False
- T09: failing none · own tests green · $0.1079505 · turns 4 · error False
- T10: failing none · own tests green · $0.13110190000000002 · turns 10 · error False
- T11: failing none · own tests green · $0.1352561 · turns 5 · error False
- T12: failing none · own tests green · $0.1415036 · turns 7 · error False

## baseline-r2

- rule tests passing after each task: 0/0, 0/0, 0/0, 0/0, 1/1, 1/1, 5/6, 8/9, 10/11, 13/14, 18/19, 20/21
- regressions: none
- T01: failing none · own tests green · $0.12061820000000001 · turns 7 · error False
- T02: failing none · own tests green · $0.11241330000000001 · turns 6 · error False
- T03: failing none · own tests green · $0.1225656 · turns 10 · error False
- T04: failing none · own tests green · $0.1236856 · turns 8 · error False
- T05: failing none · own tests green · $0.1325724 · turns 10 · error False
- T06: failing none · own tests green · $0.10149150000000001 · turns 8 · error False
- T07: failing T07RuleBodyLimit · own tests green · $0.1420693 · turns 5 · error False
- T08: failing T07RuleBodyLimit · own tests green · $0.2173022 · turns 19 · error False
- T09: failing T07RuleBodyLimit · own tests green · $0.10617720000000001 · turns 4 · error False
- T10: failing T07RuleBodyLimit · own tests green · $0.1378871 · turns 13 · error False
- T11: failing T07RuleBodyLimit · own tests green · $0.1434543 · turns 11 · error False
- T12: failing T07RuleBodyLimit · own tests green · $0.1161667 · turns 4 · error False

## pstack-r1

- rule tests passing after each task: 0/0, 0/0, 0/0, 0/0, 1/1, 1/1, 6/6, 9/9, 11/11, 14/14, 19/19, 21/21
- regressions: none
- T01: failing none · own tests green · $0.1687583 · turns 14 · error False
- T02: failing none · own tests green · $0.1569425 · turns 13 · error False
- T03: failing none · own tests green · $0.19631750000000003 · turns 20 · error False
- T04: failing none · own tests green · $0.1400344 · turns 6 · error False
- T05: failing none · own tests green · $0.1361152 · turns 5 · error False
- T06: failing none · own tests green · $0.1302506 · turns 8 · error False
- T07: failing none · own tests green · $0.16335770000000002 · turns 6 · error False
- T08: failing none · own tests green · $0.21425880000000003 · turns 17 · error False
- T09: failing none · own tests green · $0.15223690000000004 · turns 5 · error False
- T10: failing none · own tests green · $0.17918810000000002 · turns 15 · error False
- T11: failing none · own tests green · $0.164406 · turns 5 · error False
- T12: failing none · own tests green · $0.1797223 · turns 8 · error False

## pstack-r2

- rule tests passing after each task: 0/0, 0/0, 0/0, 0/0, 1/1, 1/1, 5/6, 8/9, 10/11, 13/14, 18/19, 20/21
- regressions: none
- T01: failing none · own tests green · $0.1681861 · turns 10 · error False
- T02: failing none · own tests green · $0.16698930000000003 · turns 15 · error False
- T03: failing none · own tests green · $0.17641549999999998 · turns 16 · error False
- T04: failing none · own tests green · $0.15049099999999999 · turns 7 · error False
- T05: failing none · own tests green · $0.17588470000000003 · turns 9 · error False
- T06: failing none · own tests green · $0.1468979 · turns 13 · error False
- T07: failing T07RuleBodyLimit · own tests green · $0.18640800000000002 · turns 7 · error False
- T08: failing T07RuleBodyLimit · own tests green · $0.2190053 · turns 18 · error False
- T09: failing T07RuleBodyLimit · own tests green · $0.14299289999999998 · turns 4 · error False
- T10: failing T07RuleBodyLimit · own tests green · $0.20494599999999996 · turns 11 · error False
- T11: failing T07RuleBodyLimit · own tests green · $0.1515285 · turns 5 · error False
- T12: failing T07RuleBodyLimit · own tests green · $0.1802863 · turns 8 · error False

## superpowers-r1

- rule tests passing after each task: 0/0, 0/0, 0/0, 0/0, 1/1, 1/1, 6/6, 9/9, 11/11, 14/14, 19/19, 21/21
- regressions: none
- T01: failing none · own tests green · $0.1397044 · turns 11 · error False
- T02: failing none · own tests green · $0.1369948 · turns 7 · error False
- T03: failing none · own tests green · $0.15764469999999997 · turns 17 · error False
- T04: failing none · own tests green · $0.1264048 · turns 6 · error False
- T05: failing none · own tests green · $0.1300427 · turns 9 · error False
- T06: failing none · own tests green · $0.1142784 · turns 10 · error False
- T07: failing none · own tests green · $0.1884403 · turns 14 · error False
- T08: failing none · own tests green · $0.2110108 · turns 15 · error False
- T09: failing none · own tests green · $0.1236959 · turns 5 · error False
- T10: failing none · own tests green · $0.14418770000000003 · turns 11 · error False
- T11: failing none · own tests green · $0.2037103 · turns 16 · error False
- T12: failing none · own tests green · $0.1469959 · turns 7 · error False

## superpowers-r2

- rule tests passing after each task: 0/0, 0/0, 0/0, 0/0, 1/1, 1/1, 6/6, 9/9, 11/11, 14/14, 19/19, 21/21
- regressions: none
- T01: failing none · own tests green · $0.13461700000000001 · turns 8 · error False
- T02: failing none · own tests green · $0.13725040000000002 · turns 12 · error False
- T03: failing none · own tests green · $0.1462108 · turns 6 · error False
- T04: failing none · own tests green · $0.1428587 · turns 10 · error False
- T05: failing none · own tests green · $0.15217219999999998 · turns 9 · error False
- T06: failing none · own tests green · $0.13741779999999998 · turns 13 · error False
- T07: failing none · own tests green · $0.1750506 · turns 12 · error False
- T08: failing none · own tests green · $0.233451 · turns 16 · error False
- T09: failing none · own tests green · $0.1290118 · turns 4 · error False
- T10: failing none · own tests green · $0.1530055 · turns 11 · error False
- T11: failing none · own tests green · $0.1786777 · turns 10 · error False
- T12: failing none · own tests green · $0.17692929999999998 · turns 9 · error False

## gstack-r1

- rule tests passing after each task: 0/0, 0/0, 0/0, 0/0, 1/1, 1/1, 6/6, 9/9, 11/11, 14/14, 19/19, 21/21
- regressions: none
- T01: failing none · own tests green · $0.1498827 · turns 14 · error False
- T02: failing none · own tests green · $0.13151770000000002 · turns 7 · error False
- T03: failing none · own tests green · $0.15499659999999998 · turns 16 · error False
- T04: failing none · own tests green · $0.11722390000000002 · turns 5 · error False
- T05: failing none · own tests green · $0.1480817 · turns 10 · error False
- T06: failing none · own tests green · $0.11044999999999999 · turns 9 · error False
- T07: failing none · own tests green · $0.15288200000000002 · turns 7 · error False
- T08: failing none · own tests green · $0.17180410000000002 · turns 9 · error False
- T09: failing none · own tests green · $0.1161247 · turns 4 · error False
- T10: failing none · own tests green · $0.15637430000000002 · turns 15 · error False
- T11: failing none · own tests green · $0.15672220000000003 · turns 7 · error False
- T12: failing none · own tests green · $0.1581998 · turns 12 · error False

## gstack-r2

- rule tests passing after each task: 0/0, 0/0, 0/0, 0/0, 1/1, 1/1, 6/6, 9/9, 11/11, 14/14, 19/19, 21/21
- regressions: none
- T01: failing none · own tests green · $0.15593880000000002 · turns 16 · error False
- T02: failing none · own tests green · $0.15489069999999996 · turns 9 · error False
- T03: failing none · own tests green · $0.1465563 · turns 12 · error False
- T04: failing none · own tests green · $0.15335330000000003 · turns 10 · error False
- T05: failing none · own tests green · $0.1130203 · turns 5 · error False
- T06: failing none · own tests green · $0.12242120000000001 · turns 10 · error False
- T07: failing none · own tests green · $0.1811889 · turns 14 · error False
- T08: failing none · own tests green · $0.17359270000000002 · turns 16 · error False
- T09: failing none · own tests green · $0.1274541 · turns 5 · error False
- T10: failing none · own tests green · $0.13676359999999999 · turns 11 · error False
- T11: failing none · own tests green · $0.16819540000000002 · turns 11 · error False
- T12: failing none · own tests green · $0.142633 · turns 6 · error False

## abbd-r1

- rule tests passing after each task: 0/0, 0/0, 0/0, 0/0, 1/1, 1/1, 6/6, 9/9, 11/11, 14/14, 19/19, 21/21
- regressions: none
- T01: failing none · own tests green · $0.1405662 · turns 6 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 1 active None
- T02: failing none · own tests green · $0.1487047 · turns 5 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 2 active None
- T03: failing none · own tests green · $0.168611 · turns 6 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 3 active None
- T04: failing none · own tests green · $0.13956680000000002 · turns 6 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 4 active None
- T05: failing none · own tests green · $0.15120240000000001 · turns 8 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 5 active None
- T06: failing none · own tests green · $0.1292168 · turns 9 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 6 active None
- T07: failing none · own tests green · $0.15994450000000002 · turns 6 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 7 active None
- T08: failing none · own tests green · $0.223406 · turns 15 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 8 active None
- T09: failing none · own tests green · $0.1452883 · turns 5 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 9 active None
- T10: failing none · own tests green · $0.16233530000000004 · turns 5 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 10 active None
- T11: failing none · own tests green · $0.1576113 · turns 6 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 11 active None
- T12: failing none · own tests green · $0.17855740000000003 · turns 7 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 12 active None

## abbd-r2

- rule tests passing after each task: 0/0, 0/0, 0/0, 0/0, 1/1, 1/1, 6/6, 9/9, 11/11, 14/14, 19/19, 21/21
- regressions: none
- T01: failing none · own tests green · $0.14375749999999998 · turns 5 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 1 active None
- T02: failing none · own tests green · $0.141606 · turns 7 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 2 active None
- T03: failing none · own tests green · $0.19809940000000004 · turns 10 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 3 active None
- T04: failing none · own tests green · $0.14789140000000003 · turns 9 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 4 active None
- T05: failing none · own tests green · $0.1562755 · turns 5 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 5 active None
- T06: failing none · own tests green · $0.17442880000000002 · turns 11 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 6 active None
- T07: failing none · own tests green · $0.17140560000000005 · turns 6 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 7 active None
- T08: failing none · own tests green · $0.1694101 · turns 8 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 8 active None
- T09: failing none · own tests green · $0.1371528 · turns 5 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 9 active None
- T10: failing none · own tests green · $0.18152770000000001 · turns 11 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 10 active None
- T11: failing none · own tests green · $0.1493825 · turns 4 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 11 active None
- T12: failing none · own tests green · $0.1577228 · turns 7 · error False · blueprint {'feature_map': 'CURRENT', 'architecture_map': 'CURRENT', 'graph': 'CURRENT', 'html': 'CURRENT'} receipts 12 active None

Caveats: 2 run(s) per arm; with fewer than about five reps the [min, max] range is a sanity check, not a confidence interval. The arms share the host CLAUDE.md (--setting-sources does not turn it off). Hidden tests define correctness only for what they check. Treat differences as hypotheses for the next rung, not results.

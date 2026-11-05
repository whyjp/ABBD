# Skill polish runs

An agent ran the ABBD setup skill on three held-out repositories (gin, httpie, storefront) once per skill variant. Base is the skill at a0a792f, it2 is c43960f and it3 is a2611d7. Each it3 run used a binary built from a2611d7, which also holds it2's changes.

- `rubric.md` is what the judge scored against.
- `judge-scores.tsv` holds every blind judge pass (Opus), one for the first round and two or three for each later iteration, each over 27 screenshots: per repository and variant, the overview and the two module pages with the most boxes, each a 1600x1100 dark-mode viewport shot taken at the top of the page. Sets were labelled P, Q and R in a seeded random order per repository, and the judge never saw the key.
- `lint-levels.tsv` is `abbd lint --json` for every run, measured with the a2611d7 binary.
- `decisions.tsv` joins both and records the verdicts. Judge scores are averaged over all judge passes, and a variant is kept by one of two rules, chosen by what it changes:
  - A skill or CLI change can change topology, so it is judged on fresh held-out runs against base and the current kept iteration. It is kept if it beats both in at least 2 of 3 repositories, on judge means, and in the third loses to neither by more than the passes' disagreement there.
  - A renderer-only change cannot change topology, so it is judged paired, against the drawing it changes, on the current kept runs re-rendered with the change's binary. The current kept runs are the held-out runs of the latest iteration kept as the working base, it3's for it5. The criteria the change targets are written in this README before the judges run. It is kept if those criteria rise, and no criterion falls by more than the passes' disagreement in any repository.
  - The paired rule was adopted after it5 failed the fresh-run rule, and it5 is the first change judged under it (see Iteration 5). The rule did not exist when it5's judges ran, so no criterion was named in advance. Labels (c5) is what its change targets, but its keep rests on a rule written with its scores in view.
- `shots/` holds the base, it3, it4 and it5 overviews, and the pages iterations 5 to 7 name.

## Verdict

Neither variant meets the fresh-run rule, the only rule at the time. it2 beats base in 1 of 3 repositories and is rejected.

it3 also beats base in 1 of 3 on the judge mean:

- gin is a win, 3.06 against 2.56 (+0.50).
- storefront is a tie, 3.22 against 3.22. The judge ranks it3 first there only on set of views, 4 against 2, and the fresh-run rule has no such tie-break.
- httpie is a loss, 2.33 against 2.67 (-0.34), with lines down from 2.67 to 1.67.

The evidence is thin. It is one blind judge pass over 3 images a repository, and the margins are about the size of the noise between two runs of one skill. Lint regressed too. gin's hard breaches rose from 1 to 12, and httpie's crossings from 4 to 43. The first `abbd lint --record` refusal that it3 added never fired in these runs.

it3 stays the working base for the next iteration, because its level score (c1_level) rose in all three repositories, to 3.67 to 4.00 from base's 2.33 to 3.33. It is not shown to improve on base.

The `level_advice` column of `lint-levels.tsv` comes from the a2611d7 binary. Its hub test counted lines to a frame's chips and both directions of a mutual line, so it marked most boxes of a small page as hubs. Later level checks count only lines between a view's boxes and can differ from it.

## Iteration 4

it4 (0b83eca) gave the setup skill a part rule a script runs, dropped the overview bound from 12 boxes to 7, and labelled a mixed-phrase line with its most common phrase. Each it4 run used a binary built from 0b83eca.

Two fresh Opus judges each scored the same 27 screenshots (base, it3 and it4) in one pass, under different blind seeds (4101 and 7919). Rows marked `@4a` and `@4b` in `judge-scores.tsv` are the passes and `@4` their average. `lint-levels.tsv` adds it4 and re-measures it3 with the 0b83eca binary (`it3k4-*`), so levels compare on one binary. That binary counts grouped overview lines, so its `ov_lines` are lower than the a2611d7 numbers for the same state.

Averaged judge means, with the two passes in brackets:

| repo | base | it3 | it4 |
|---|---|---|---|
| gin | 2.50 (2.44, 2.56) | 2.77 (2.61, 2.94) | 2.22 (2.28, 2.17) |
| httpie | 2.67 (2.50, 2.83) | 2.19 (2.28, 2.11) | 2.61 (2.44, 2.78) |
| storefront | 2.86 (2.78, 2.94) | 3.19 (3.28, 3.11) | 3.58 (3.72, 3.44) |

The passes differ by 0.22 on a set mean on average and by 0.34 at most. Both passes rank the three sets the same way on gin and storefront, and swap base and it4 on httpie.

it4 is rejected and reverted in f89ca26. It beats base and it3 on storefront only (+0.72 and +0.39). On httpie it ties base (-0.06, inside the pass disagreement). On gin it loses to base by 0.28 and to it3 by 0.55, more than either pass disagreement on gin. There the part rule merged gin's root package so 6 of 7 module pages hold fewer than 2 boxes, and the Context page draws nothing.

The same base and it3 screenshots scored lower this round than in the single pass above (storefront base 3.22 to 2.86, gin it3 3.06 to 2.77). A gap of about 0.3 to 0.4 between rounds is judge noise, which makes the earlier one-pass verdicts weaker still.

## Iteration 5

it5 (fa9a75c) tests iteration 4's phrase rule alone. A line whose relations carry different phrases says the phrase the most of them carry, with 등, and falls back to the via word only when none carries a phrase. The skill text, the part rule and the overview bound are as on main.

The change is in the renderer only, so it5 was not run as new candidates. The it3 runs were copied and synced with the fa9a75c binary. Every element of every page except label text is byte-identical to it3, and so is every lint total (`it5-*` and `it3k5-*` rows in `lint-levels.tsv`). The screenshots are the same 9 pages as it3's, taken the same way; retaking an it3 page reproduced its old screenshot byte for byte.

On the overviews the via words give way to phrases:

| repo | it3 labels | it5 labels |
|---|---|---|
| gin | 1 phrase with 등, 11 via words | 6 phrases with 등, 5 via words |
| httpie | 2 phrases | 2 phrases, 7 phrases with 등 |
| storefront | 2 phrases, 8 via words | 2 phrases, 4 phrases with 등 |

Three fresh Opus judges scored base, it3 and it5 in one pass each, under blind seeds 5303, 6211 and 8803. Rows `@5a`, `@5b` and `@5c` in `judge-scores.tsv` are the passes and `@5` their average. Averaged means, with the passes in brackets:

| repo | base | it3 | it5 |
|---|---|---|---|
| gin | 2.48 (2.33, 2.61, 2.50) | 3.09 (3.00, 3.11, 3.17) | 3.19 (3.06, 3.28, 3.22) |
| httpie | 2.55 (2.44, 2.61, 2.61) | 2.24 (2.22, 2.33, 2.17) | 2.30 (2.28, 2.39, 2.22) |
| storefront | 3.13 (3.11, 3.17, 3.11) | 3.24 (3.28, 3.06, 3.39) | 3.46 (3.50, 3.33, 3.56) |

By set mean, all three passes rank it5, it3, base on gin and base, it5, it3 on httpie. On storefront all three rank it5 first, and passes @5a and @5c rank it3 above base, but pass @5b ranks base (3.17) above it3 (3.06). Two passes differ by 0.11 to 0.15 on a set mean on average, and by 0.33 at most.

it5 is kept under the paired rule. It changes only the renderer, and it draws it3's runs, so it is judged against it3. It raises labels (c5) in all 9 pass-and-repo cells, by 0.44 on gin, 0.33 on httpie and 1.22 on storefront averaged. Lines (c4) and every other criterion except overall (c6) score the same as it3's, and c6 rises by 0.11 on gin and storefront. Its mean gain over it3 is 0.09, 0.06 and 0.22.

Under the fresh-run rule it would have been rejected, and it was first reverted in 79f1779 on that ground. It beats base and it3 on gin and storefront, but loses httpie to base by 0.26, more than the largest pass disagreement there (0.17). That gap is it3's topology, which it5 draws unchanged, so it measures it3 against base, not the label rule. f0d42b1 undid that revert.

## Iteration 6

it6 is a renderer-only change, so it is judged under the paired rule against it5: the it5 runs (`w8/*-it5`) copied to `w9/*-it6` and synced with the it6 binary, the same 9 pages screenshotted the same way.

The criteria it targets, named before any judge runs, are lines (c4) and density (c2). In rounds 3 to 5 every blind judge named the same two defects in all three repositories:

- On the overview, lines merge into shared orange buses that read as joining modules that do not relate, with staircases near the bus and long detours round the right edge. Lint counts the false joins as J3 phantoms: 12 on gin's overview and 7 on httpie's.
- On part pages, the lines to the frame's chips tangle the page while the boxes inside are few: gin's shared runtime draws about 25 lines from 6 chips into 5 boxes, httpie's runtime 13 chips, and httpie's program entry 18 arriving lines.

Each hypothesis is measured on the fixture lint gate and on the held-out re-renders, and logged in `eval/lint/decisions.tsv`. It is kept only if it lowers crossings, phantoms or excess turns on the re-renders without raising hard breaches on the fixtures.

Two hypotheses were kept (rows 55 and 56 of `eval/lint/decisions.tsv`). The before numbers are the it5 runs re-rendered with main's binary (a4390c5), the after numbers the `w9/*-it6` runs, drawn by 2b922df.

- Two-headed lines apart (6c2e890). Every J3 phantom on the gin and httpie overviews ran along two lines between modules that use each other, bundled at one box. Each such line is a start and a head at both ends, so the bundle joined their far ends. Each now runs on buses of its own pair, unless that breaks more of rules 13 and 15, as on pstack's overview, which stays bundled.
- Gathered chips (2b922df). A part view may draw one chip per box and edge for the modules outside that meet that box with no contract, instead of one chip per module, so each box takes one line from the frame. choose keeps that drawing only where it crosses no more than the others.

| view | before: phantoms, crossings, excess turns, weighed | after |
|---|---|---|
| gin overview | 12, 6, 48, 198 | 0, 14, 42, 182 |
| httpie overview | 7, 15, 95, 395 | 0, 26, 90, 386 |
| gin shared runtime | 0, 3, 59, 239 | 0, 1, 26, 105 |
| httpie runtime and shared types | 0, 26, 124, 522 | 0, 0, 57, 228 |
| httpie program entry | 0, 0, 19, 76 | 0, 0, 11, 44 |
| gin, every view | 12, 11, 267, 1079 (hard 12) | 0, 17, 129, 533 (hard 0) |
| httpie, all views | 7, 43, 412, 1691 (hard 10) | 0, 26, 243, 998 (hard 2) |
| storefront, all views | 0, 4, 61, 248 | the same, byte for byte |

The overviews cross more: each two-headed line now crosses the rows on its own track. gin's routing page is no longer dense, so its 3 focus views are gone. On the fixtures, hard breaches stay at 61 and no view gains one. Over the 244 views both drawings hold, the weighed sum falls from 9812 to 8116 and placement from 326 to 252. Five views are no longer dense, so their 30 focus views are gone. Every fixture ceiling went down, none up. `shots/it6-*.png` are the after pages, and `shots/it5-gin-MOD-0027.png` and `shots/it5-httpie-MOD-0003.png` the part pages before.

Three fresh Opus judges scored base, it5 and it6 in one pass each, from the prompts and blind keys `judge-prompt-6a` to `6c` and `review6a-key.json` to `review6c-key.json`, under blind seeds 61, 62 and 63. Rows `@6a`, `@6b` and `@6c` in `judge-scores.tsv` are the passes and `@6` their average. Averaged means, with the passes in brackets:

| repo | base | it5 | it6 |
|---|---|---|---|
| gin | 2.55 (2.33, 2.61, 2.72) | 3.17 (3.33, 3.11, 3.06) | 3.44 (3.72, 3.28, 3.33) |
| httpie | 2.68 (2.61, 2.61, 2.83) | 2.33 (2.50, 2.11, 2.39) | 2.80 (3.11, 2.50, 2.78) |
| storefront | 3.11 (3.22, 3.11, 3.00) | 3.46 (3.61, 3.22, 3.56) | 3.45 (3.56, 3.22, 3.56) |

The named criteria, averaged over the passes:

| repo | c2 density, it5 to it6 | c4 lines, it5 to it6 |
|---|---|---|
| gin | 3.00 to 3.56 | 2.67 to 3.11 |
| httpie | 2.00 to 2.67 | 1.67 to 2.44 |
| storefront | 4.00 to 4.00 | 3.33 to 3.33 |

it6 is kept under the paired rule. c2 and c4 rise on gin and httpie in every pass, and hold on storefront, whose drawings it6 leaves byte for byte as they were. All three passes rank it6 first on gin. On httpie every pass puts it6 above it5, and two of them put base first, so it6's mean gain over base there (+0.12) is inside the passes' disagreement. Every other criterion holds or rises on gin and httpie.

The one criterion that falls is labels (c5) on storefront, from 3.11 to 3.00, inside the cell's pass disagreement (0 to -0.33). It does not come from it6. `shots5` were drawn by the kit5 binary, which predates PR #100's fix round, and that round's label fallback put more generic 임포트 labels on storefront's overview, which two judges named. it5's and it6's storefront drawings differ only by that fallback.

Defects the passes still name:

- httpie's overview is a hairball that runs past the 1600px viewport.
- storefront's part pages draw no outside chips, so the ui kit and the shopify client pages show nothing of what meets them, and its overview runs one orange bus through unrelated pages.
- gin's overview lines climb into request context, and generic 임포트 labels remain on httpie and storefront.

A review of PR #101 then found the gathered wires' tooltips named only the first module they gather, the gathered chips lost their links, and the chip's 24-rune name cap let Hangul names wrap. 7111bdd fixes all three. On the held-out runs that changes tooltips and links only, so the judged pages are unchanged.

## Iteration 7

it7 changes the skill and the CLI, so it is judged under the fresh-run rule, on new candidate runs against base and it6. The renderer is as on main.

Two defects the judges named share one cause in the units. A module's page draws its parts and what meets them, so a relation that meets the module itself has no box there. On the it6 runs storefront's shopify client page draws no chip, though 8 modules use it: they all import its own `index.ts`, which sits in no part. gin it4's Context page was an empty frame: the module held 7 files and no parts. A probe over the kept runs found the same in every repository: 1 to 6 module pages with parts per run whose own code carries relations (6 on gin, 1 on httpie, 4 on storefront), and 1 to 4 top-level modules of 4 files or more with no parts, such as gin's engine and httpie's cli and manager.

- Hypothesis 1. A check that names both, which the first `abbd lint --record` waits for, makes the candidate end each coupling at a part and cut each module of 4 files or more into parts. `abbd lint` now lists them under `unseen` in its level lines: the relations that meet a module with parts itself, with the files that sit in no part, and a top-level module of 4 files or more with no parts.
- Hypothesis 2. The setup skill gets a part rule a script can run on any language, with the key, the merge order and the count defined: subdirectories first, then a file's receiver type or class, or in component code its default or first export with `use`, `Provider`, `Context` and `Props` taken off, then its name stem. A module's import file is its API part. One-file parts merge, first by name, into the part that uses the most of their names, and merging stops at two parts. Step 3 says to end each coupling at the part whose file holds its code at each end.
- Hypothesis 3. A view of 4 drawn relations or more of which fewer than half carry a phrase is listed under `unseen` too. On the it6 runs that names httpie's overview (45 of 83 without one) and its runtime page (53 of 56), and 5 gin pages.
- The skill also says how to find the shared module, by the count `abbd lint` uses for hubs, and which top-level modules a web app has.

The criteria these target, named before any judge runs, are set of views, density (c2) and labels (c5) on the module pages, and labels on the overview. Lint on the fresh runs should list no `unseen` line, or name each one kept with `--keep-level`.

Each it7 candidate ran the setup skill of b5ab71e with a binary built from it, on a fresh clone of each repository (`w10/*`). All three ended with `abbd lint` and `abbd status --check` at 0 and no `unseen` line, and none kept a level with `--keep-level`. The it7 rows of `lint-levels.tsv` are `abbd lint --json` on those runs with the b5ab71e binary.

Three fresh Opus judges scored base (`shots0`), it6 (`shots6`) and it7 (`shots7`) in one pass each, from the prompts and blind keys `judge-prompt-7a` to `7c` and `review7a-key.json` to `review7c-key.json`, under blind seeds 71, 72 and 73. The it7 pages are each run's overview and its two module pages with the most boxes. Rows `@7a`, `@7b` and `@7c` in `judge-scores.tsv` are the passes and `@7` their average. Averaged means, with the passes in brackets:

| repo | base | it6 | it7 |
|---|---|---|---|
| gin | 2.59 (2.61, 2.56, 2.61) | 3.44 (3.50, 3.44, 3.39) | 3.04 (3.00, 3.11, 3.00) |
| httpie | 2.65 (2.89, 2.61, 2.44) | 2.85 (2.89, 2.83, 2.83) | 2.57 (2.56, 2.72, 2.44) |
| storefront | 3.21 (3.28, 3.17, 3.17) | 3.55 (3.61, 3.44, 3.61) | 3.52 (3.50, 3.50, 3.56) |

The named criteria, averaged over the passes, as set means over all three pages (the judges scored c2 and c5 per set, so module pages and the overview are not split):

| repo | set of views, it6 to it7 | c2 density | c5 labels |
|---|---|---|---|
| gin | 4.00 to 3.00 | 3.78 to 3.33 | 3.67 to 3.22 |
| httpie | 3.00 to 2.33 | 2.89 to 2.45 | 2.89 to 2.78 |
| storefront | 3.67 to 4.00 | 4.22 to 3.78 | 3.00 to 3.44 |

it7 is rejected under the fresh-run rule and reverted on PR #102. It beats it6 in none of the three repositories. Every pass puts it6 above it7 on gin (by 0.33 to 0.50) and on httpie (by 0.11 to 0.39). On storefront two passes put it6 ahead and one puts it7 ahead, a tie inside the passes' disagreement (-0.03 averaged). It beats base on gin (+0.45) and storefront (+0.31) and loses httpie to base (-0.08). Of the named criteria only storefront's set of views and labels rose.

What the judges named on it7:

- gin promotes bytesconv, fs and json codec to the top level beside the subsystems, and its server engine page is a knot of lines, with test, version and utils drawn as parts. 3 of its 10 module pages hold fewer than 2 boxes.
- httpie's overview is a hairball cut off at the fold, with lines labelled `Environment 등`, and its shared foundation page is a grab bag of 8 parts with odd names.
- storefront promotes its product grid to a top-level module beside the route areas, and has the best phrases of the three sets. Its part pages now draw their chips. The shopify client page draws its 8 users, which it6's drew none of.

What the runs taught. The check stands as a finding; the part rule it was paired with does not.

- The `unseen` check found every defect the judges named of its kind on the old runs: storefront's chipless shopify client page, gin it4's empty Context page, and the modules with no parts. On the fresh runs each candidate cleared it, and storefront's pages gained their chips.
- Clearing it cost more elsewhere. The part rule stops merging at two parts and has no rule for a root package that holds most of the code, as gin's and httpie's do. Both candidates fell back to supplement groups, and the groups promoted helpers to the top level and drew test and version files as parts.

Open items for the next iteration, from the candidates' reports (`notes/it7-candidates.md`):

- A rule for cutting a root package that holds most of a repository's code.
- `abbd record` refuses a group's evidence quote when every file it quotes is already anchored, so gin's and httpie's candidates quoted docs instead.
- The merge rule's names. Merged parts keep the target part's name, so storefront's root files (layout, sitemap, robots, error) land in a part named navbar and `lib/constants` and `lib/utils` in one named footer.
- The name split of CamelCase identifiers breaks `HTTPie` into `htt pie`, as in httpie's `htt pie cookie policy`, and rule 2's key picks private names such as `json dict preserving duplicate keys`.
- Step 11 does not say that the setup takes a second commit, after the first sync, so httpie's status read STALE until its candidate made one.
- Smaller gaps: rule 2's component key reads as JavaScript only, "the receiver most files use" is undefined, an API part swallows what merges into it, a directory part with no entry file has no name fallback, and the route-area rule collides with grouping by directory.

Every candidate's subagent handbacks failed: no worker's report came back to the candidate through the handback. Each worked around it. The storefront candidate had its workers write their results to files in the scratchpad, gin's had them write `.scratch-abbd` files in its clone and deleted them before committing, and httpie's read the workers' results from their transcripts.

`shots/it7-gin-overview.png`, `shots/it7-gin-MOD-0033.png` (server engine), `shots/it7-httpie-overview.png` and `shots/it7-storefront-MOD-0022.png` (shopify client) are judged it7 pages.

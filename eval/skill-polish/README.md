# Skill polish runs

An agent ran the ABBD setup skill on three held-out repositories (gin, httpie, storefront) once per skill variant. Base is the skill at a0a792f, it2 is c43960f and it3 is a2611d7. Each it3 run used a binary built from a2611d7, which also holds it2's changes.

- `rubric.md` is what the judge scored against.
- `judge-scores.tsv` holds one blind judge pass (Opus) over 27 screenshots: per repository and variant, the overview and the two module pages with the most boxes, each a 1600x1100 dark-mode viewport shot taken at the top of the page. Sets were labelled P, Q and R in a seeded random order per repository, and the judge never saw the key.
- `lint-levels.tsv` is `abbd lint --json` for every run, measured with the a2611d7 binary.
- `decisions.tsv` joins both and records the verdicts. A variant is kept only if, on judge means averaged over all judge passes, it beats both base and the current kept iteration in at least 2 of 3 repositories, and in the third loses to neither by more than the passes' disagreement there.
- `shots/` holds the base, it3, it4 and it5 overviews.

## Verdict

Neither variant meets the stated rule. it2 beats base in 1 of 3 repositories and is rejected.

it3 also beats base in 1 of 3 on the judge mean:

- gin is a win, 3.06 against 2.56 (+0.50).
- storefront is a tie, 3.22 against 3.22. The judge ranks it3 first there only on set of views, 4 against 2, and the stated rule has no such tie-break.
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

All three passes rank the sets the same way: it5, it3, base on gin and storefront, and base, it5, it3 on httpie. Two passes differ by 0.11 to 0.15 on a set mean on average, and by 0.33 at most.

it5 is rejected and reverted in 79f1779. It beats base and it3 on gin and storefront. On httpie it loses to base by 0.26, more than the largest pass disagreement there (0.17).

The rule judges a renderer-only change against base's topology, though. it5 draws it3's runs, and it3 already lost httpie to base. Paired against it3, the change raises labels (c5) in all 9 pass-and-repo cells, by 0.44 on gin, 0.33 on httpie and 1.22 on storefront averaged, and leaves lines (c4) unchanged. Its mean gain over it3 is 0.09, 0.06 and 0.22. A rule that compares a drawing change with the drawing it changes would keep it.

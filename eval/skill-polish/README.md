# Skill polish runs

This folder holds how ABBD's setup skill, CLI and renderer are judged on repositories they were never tuned on, and every round judged so far. The goal is a blueprint that reads like a diagram an engineer polished by hand in draw.io, on any repository.

- `rubric.md` is what a judge reads: the criteria and the pairwise format.
- `tools/` holds the levers. Each script's docstring says how to run it. `tools/verdict.py` applies the keep rules below to a round's passes.
- `judge-scores.tsv` holds every absolute judge pass of rounds 3 to 7, `pairwise/` every pairwise pass of the method study, with its key and the prompt its judges read.
- `lint-levels.tsv` is `abbd lint --json` for every run, and `decisions.tsv` joins both with the verdicts of rounds 3 to 7.
- `ref-parts/` holds the reference parts of the six pool repositories, for the parts proxy.
- `shots/` holds the judged pages that iterations 5 to 7 name.

## Protocol

From iteration 8 on, a change goes through three tiers, cheapest first. It moves to the next tier only if it passes the one before. The tiers exist because a fresh-run round costs hours, and through iteration 7 every skill change waited for one before anyone learned it had failed.

These rules were written after the method study below, with its data in view, and before any iteration 8 data. That includes the headline criterion (c6), the 0.10 flip threshold and the unanimity rule of Tier 2. Iteration 8 is the first round judged under them. `tools/verdict.py ROUND_DIR KEPT CAND --tier renderer|proxy|fresh` computes every statistic and rule named here, so a verdict is a command, not a reading.

What a change touches decides where it is decided:

- A renderer-only change decides at Tier 1, on the current runs re-rendered with its binary. It never goes to Tier 2.
- A skill or CLI change never decides at Tier 1. Tier 1 only screens it on proxy drawings, and only Tier 2's fresh runs can keep it.

### Tier 0. Automated, minutes, no judge

Every change, renderer or skill, runs these first.

- **Renderer change.** `tools/rerender.sh BIN OUT NAME=RUN...` copies every finished run in the pool and syncs it with the change's binary. Six runs take 16 seconds. `tools/shoot.py OUT --pages all REPO=OUT/NAME/.abbd/blueprint...` draws every view whole and runs the hover pass, about 5 seconds a run. Compare its findings table and `abbd lint` totals with the same table for the current binary. `tools/pairs.py ... --same-runs --changed-only` then lists only the views whose drawing changed. A change whose hover findings or hard breaches rise with no reason written in advance stops here.
- **Skill change.** Check the decomposition before any drawing is judged. A rule the skill states as a script (a part rule, a grouping step) is run as that script on all six pool repositories. A rule only an agent can apply is run through setup steps 1 to 5, which skip step 6's understanding texts and step 7's look; its cost is not yet measured. These are the change's proxy runs. `abbd lint` level lines and `tools/parts.py ref-parts/REPO.tsv RUN...` then screen for collapse: a run with fewer than half the reference's top-level parts, or a top-level F1 under 0.5, stops here. The F1 does not rank two healthy runs (see the method study), so it never decides a keep.

The pool is every finished run of the current kept iteration on triple A, plus the zero-base runs of triple B (see Rotation). A renderer change is drawn on all of them at once, never on three.

The baseline a change is compared with is the current kept iteration's run of the same repository. No kept iteration has runs on triple B yet, so until one does, triple B's baseline is its `zM` runs (main's skill when they were made), re-rendered with the current binary.

### Tier 1. Pairwise judges, about 15 minutes

Three fresh Opus judges, blind, each in one pass, compare views in pairs (`rubric.md`). `tools/pairs.py OUT KEY SEED NAME=DIR... --pair KEPT,CAND --null KEPT` builds one pack per judge with its own seed, and writes that judge's prompt next to KEY. OUT must be a new folder whose parent holds nothing else. The pack carries its own copy of the rubric, and keys and source shots stay under another root; `pairs.py` refuses to build a pack otherwise. The prompt asks for the tables as the judge's final answer, which the launching agent collects, and tells the judge to send no messages.

**Near-nulls.** Each pack holds, per repository, the baseline's overview against a near copy of itself (`--null`). The copy is moved down one pixel and encoded again, and it and its hover shots get fresh names, so neither bytes nor names give it away. Neither the prompt nor the rubric says such pairs exist. A pass that answers a near-null with A or B on any criterion is rerun with a new judge. A re-render's unchanged views are near-nulls too, but `--changed-only` drops them.

**Rules.**

- A view's verdict on a criterion is the side more than half the passes pick (2 of 3). Otherwise it is a tie.
- **Flip rate.** Over every comparison except the near-nulls, every criterion the pack asks (c1 to c6, and c7 when it holds hover shots), and every pair of passes: the share of pass pairs that give opposite strict answers (one A, the other B). If it is above 0.10, the round is rerun with three new judges before anyone acts on it.
- **Renderer change.** The criteria the change targets are written in this README before the judges run. It is kept if each targeted criterion, counted on its own, wins more views than it loses, criterion 6 wins at least as many views as it loses, and no overview loses criterion 6.
- **Skill or CLI change, the proxy screen.** The drawings are the overviews of Tier 0's proxy runs, one per pool repository (six), synced with the current binary, each against the baseline's overview of the same repository. The change goes to Tier 2 if its overview loses criterion 6 in no more repositories than it wins. Nothing is kept at this step.

### Tier 2. Fresh runs, hours

Only a skill or CLI change that passed Tier 0 and the proxy screen gets fresh setup runs, one per repository, on the triple whose turn it is.

- **Rotation.** The pool is gin, httpie and storefront (triple A) and express, click and hyperfine (triple B). Rounds alternate triples, so no repository decides two rounds in a row. A change kept on one triple is checked on the other at Tier 0 before the next round starts. When a repository has decided four rounds, it is replaced by a new one of the same language family, and its runs stay in the Tier 0 pool.
- **Judging.** The judges compare the candidate's views with the baseline's, overview against overview and the densest pages by rank, plus the hover shots `shoot.py` picks (criterion 7). Rank-matched pages show two different modules, so the overview verdicts carry the decision.
- **Keep rule.** The change is kept if its overview wins criterion 6 unanimously (all three passes) in at least 2 of 3 repositories, loses it in none, and the pages win criterion 6 at least as often as they lose it. A majority win is not enough: on overviews, c6 flipped between passes on 6 of 45 pass pairs (0.13) in the study, as often as absolute c6 (9 of 63, 0.14). A change that wins 2 of 3 overviews by majority but not unanimously gets a fourth repository or three more judges before it is decided.
- **No base comparison.** Through iteration 7 a fresh-run change had to beat both base and the kept iteration. Tier 2 drops the base comparison on purpose. The question a round answers is whether the change should replace the working base, and a third set in every pack spends judge time on a question no keep rule asks. The cost is that no tier checks drift from base.
- **Set of views** is recorded but gates nothing. Its pairwise answers flipped between passes on 22 in 100 pass pairs.

### What the judge sees

- Every view whole, captured by `shoot.py` at its drawn size. Through iteration 7 the shots were a 1600x1100 viewport taken at the top of the page.
- Hover shots of each view: the hub box's hover and each hover the checks flag, up to six.
- The judge never sees lint numbers, findings or which version drew which, and is not told that near-nulls exist.

## Method study (before iteration 8)

Through iteration 7 the round loop was slow. Each skill round cost hours of fresh runs before its first verdict, and three of the four skill rounds (it2, it4, it7) were rejected. Four parts of the method were examined on the data that already existed. The protocol above is what the evidence supports, and where it goes further than the evidence, this section says so.

### Judge noise: absolute scores against pairwise verdicts

The question was whether a judge's 1 to 5 scores drift so much between passes that they swamp real gains, and whether asking a judge which of two drawings of one view reads better is steadier.

The same screenshots were judged many times. `shots0` (base) was scored in 11 absolute passes from rounds 4 to 7, and `shots5` and `shots6` in 6 each, byte for byte the same images (checked by hash against every review folder). Three new blind Opus judges then compared the four judged sets `shots0`, `shots5`, `shots6` and `shots7` in pairs, in packs built by `tools/pairs.py` with seeds 81, 82 and 83 (`pairwise/`). Each pack holds 48 comparisons: 5 variant pairs (base and it5, it5 and it6, base and it6, it6 and it7, base and it7) times 3 repositories times 3 views, plus one overview against a copy of itself per repository, and 15 set comparisons. A and B were drawn at random per comparison. These images were 1600x1100 viewport crops with no hover shots.

`tools/agree.py judge-scores.tsv pairwise` measures both methods on the 15 decision cells (variant pair and repository) per criterion. A pass answers a cell with X, Y or a tie. For absolute scores that is the sign of the difference of the two sets' scores in one pass. For pairwise it is the sign of the wins over the set's three views. Over every pair of passes that answer a cell, it counts strict same (both give the same strict answer) and flips (opposite strict answers). Plain agreement would count two ties as agreeing, and pairwise ties far more often than absolute, so strict same is the figure that compares like with like.

Pairwise has 3 passes on each cell, 45 pass pairs. Absolute pass pairs are taken inside one round, and 6 of the 15 cells were scored in two rounds, so absolute has 21 cell-rounds and 63 pass pairs. Keeping only each cell's last round gives absolute the same 15 cells and 45 pass pairs. Measured:

| criterion | pairwise: strict same, flips | absolute, every round: strict same, flips, p | absolute, last round per cell: strict same, flips, p |
|---|---|---|---|
| c1 level | 0.56, 6/45 | 0.76, 2/63, 0.065 | 0.67, 2/45, 0.27 |
| c2 density | 0.67, 5/45 | 0.73, 4/63, 0.49 | 0.69, 3/45, 0.71 |
| c3 placement | 0.62, 1/45 | 0.24, 5/63, 0.40 | 0.24, 4/45, 0.36 |
| c4 lines | 0.40, 1/45 | 0.35, 7/63, 0.14 | 0.36, 5/45, 0.20 |
| c5 labels | 0.80, 6/45 | 0.79, 2/63, 0.065 | 0.78, 2/45, 0.27 |
| c6 overall | 0.82, 2/45 | 0.65, 9/63, 0.12 | 0.67, 4/45, 0.68 |
| all criteria together | 0.60, 8/45 | 0.81, 5/63, 0.14 | 0.78, 3/45, 0.20 |

p is a two-sided Fisher exact test of the absolute flip count against pairwise's. `agree.py` also prints the first round per cell, which gives the same picture.

What it shows:

- The study shows no measurable difference in stability between the two methods. No criterion's flip counts differ at p < 0.05. Pairwise flips less on c6, c4 and c3 (p 0.12, 0.14 and 0.40 against every round). Absolute flips less on c1 and c5 (p 0.065 each) and on the all-criteria sum. Counted like with like, one round per cell, no p is below 0.19. With 15 cells and 3 passes, a difference of a few flips in 45 cannot be told from chance.
- An earlier draft of this section made c6 the headline because pairwise looked steadier there. That was chosen after seeing the data, and the data do not support it. c6 stays the headline for iteration 8 because it is the question closest to the goal (which drawing an engineer would put in a design doc), not because it was measured steadiest. The 0.10 flip threshold was set the same way, after this study.
- Pairwise is kept for two reasons that do not rest on stability. It can say "no difference", which a 1 to 5 score cannot. And on a re-render it compares one view's two drawings side by side, where absolute scores compare two scorings made apart.
- Every answer on an image against its own copy was `=` (54 of 54), and so was every answer on the three views byte-identical between it5 and it6 (54 of 54). But this study announced its nulls: the prompt told the judges some pairs may be identical, and the rubric said identical drawings are `=` on every criterion. So 54 of 54 shows the judges read that instruction, not that they catch an unmarked null. From iteration 8 on, near-nulls go unannounced. A side was picked 50% of the time among 574 strict answers, so no position bias showed.
- Rank-matched module pages compare two different modules when the topologies differ (gin's routing page against its shared runtime page, for one). The overview is the only view that is the same view in every pair. On overview comparisons alone, passes agree on 0.78 of answers and flip on 0.09 over all criteria. On c6 alone they flip on 6 of 45 pass pairs (0.13), as noisy as absolute c6. That is why Tier 2 asks for unanimous overview wins.
- The judges saw viewport crops and no hover shots. How noisy they are on whole drawings, and on hover shots under criterion 7, is not measured.

Absolute scores are noisy enough to turn a one-pass verdict. The same image set's mean moves between passes by a standard deviation of 0.14 (median over 15 sets, range 0.03 to 0.22). A cell's gap is the difference of two such means, so one pass's gap carries noise of about 0.14 × √2 ≈ 0.2. The median gap between the two variants of a decision cell is 0.31, about 1.5 of those standard deviations. Under a normal approximation (inferred, not measured), one pass gets the sign of a 0.31 gap wrong about one time in sixteen, and of a 0.1 gap about one time in three. The one-pass and two-pass verdicts of rounds 3 and 4 were not safe from that. The 0.3 to 0.4 swings named in iteration 4 are the range over 11 passes, not one pass's error. Noise does not explain all of the slowness, though. Each skill change waited for fresh runs before its first verdict, 3 judged views a repository showed little of each run, and the judges never saw hover.

So the protocol keeps pairwise verdicts for the `=` answer and the side-by-side comparison, takes c6 as the headline by choice, and drops set of views and the all-criteria sum as gates, since their pairwise answers flip most (22 and 18 in 100 pass pairs).

### Whole drawings

Through iteration 7 each judged image was a 1600x1100 viewport at the top of the page. `tools/shoot.py` captures each view's drawing at its drawn size instead, from file URLs in headless Chrome, and reports what the old crop would have cut. On the overview and the two densest pages of each base, it5, it6 and it7 run (36 views; on the base runs `shoot.py`'s box count picks other pages than `shots0` holds) the crop cut only the httpie overview, 11% to 15% of its drawing in it5, it6 and it7. On the 63 views of the z0 rotation runs it cut 2, and on zM none. The crop explains one recurring judge complaint (httpie's overview "cut off at the fold"), not the scores.

To shoot a drawing whole, `shoot.py` widens the viewport to the page's scroll width and grows it to the page's height, capped at 12000px. A drawing past the cap would be cut there, and a page whose layout follows the viewport width may reflow when widened, so the shot shows the widened layout. The tallest measured drawing is 1504px (z0 click, one module page), far from the cap.

Capture is deterministic. Re-rendering the it6 runs with this branch's binary and capturing every view again gave byte-identical images for all 31 views, so `pairs.py --changed-only` dropped every comparison.

`shoot.py` drives the installed Chrome (`--headless=new`) over the DevTools protocol with a small websocket client, and needs nothing installed beyond Python. Its first version used Playwright. Before that path was deleted, both versions shot every page of six runs, the it6 runs of gin, httpie and storefront and the z0 runs of express, click and hyperfine. All 287 drawings and hover shots and all three findings files came out byte for byte the same, and the new path took 30 seconds against Playwright's 41. Aside's clipped screenshot timed out after 31 seconds on the first view tried, where headless Chrome took 1.2 seconds.

### Hover

The user found by hand that hovering a line lights it only partway, so its two end modules cannot be told, and that hovering a module mixes colours along a route. No judge could see that. `shoot.py` hovers every box, chip and line with the real mouse, lets the page's own CSS decide what lights, and checks the lit set geometrically. Measured on the judged runs, as views with at least one finding (the first version's count in brackets where it differs):

| run set | views | with a cut route | with a stray light | with a mixed route | with a cut line hover | with a hidden line |
|---|---|---|---|---|---|---|
| base (`w0`) | 9 | 7 | 2 (0) | 7 | 7 | 2 |
| it6 (`w9`, every page) | 31 | 16 (17) | 3 (0) | 14 (15) | 17 | 2 |
| z0 (every page) | 63 | 15 (17) | 0 | 9 | 17 | 6 |
| zM (every page) | 30 | 14 | 3 (0) | 12 (13) | 15 | 3 |

The review of the first version found four checks weaker than their docstring. The 12px stub radius was searched only one 4px grid cell around a line end; it now searches every cell the radius covers. Overlap counted every point of one line near another, scattered or not; it now takes the longest unbroken run, which removes the base runs' 2 overlaps and zM's 1. Mixed joined two lines that only end at the same box anchor; a join within 8px of a box no longer counts. Stray covered only boxes and chips with lines; it now covers every chip, and a chip that sits on a box is stray when it stays lit while its box dims. That is new on 8 of the 12 overviews measured (none of z0's): the contract chips drawn on a box never dim, so a box hover leaves them bright on dimmed boxes, as the hover shots show. Box hovers that light every line were skipped without a word; they are now checked and listed as `all_lit` (5 views of it6, 12 of z0 and 2 of zM).

On the it6 httpie overview, 44 of the 54 line hovers light a set that leaves an end loose or reaches fewer than two boxes, because a line hover lights the one path segment under the mouse and a route drawn as a shared bus is several segments. A box hover lights a shared bus whole, so a lit route runs on to a dimmed box, and a box's coloured stub joins a grey bus, which the mixed check counts. Each flagged hover is saved as a hover shot for the judges' criterion 7, and the counts sit beside the verdicts. The checks are geometric heuristics with 8 and 12 pixel tolerances; a judge's look at the hover shot decides, the count only says where to look.

### The parts proxy

The hope was a judge-free screen for skill changes: score a run's modules against a reference decomposition and stop a change that lowers the score. A fresh blind Opus agent wrote the reference parts of all six pool repositories from their source trees alone, without seeing any run (`ref-parts/`, each table followed in the agent's report by the cuts it was least sure of). `tools/parts.py` scores a run by B-cubed F1 over files, for the top level and for the top two levels together.

It does not predict the judges. Over the 13 judged topologies (base, it2, it3, it4 and it7, with it5 and it6 sharing it3's), 16 pairs of runs in one repository differ in judged level (c1) by 0.3 or more, and the two-level F1 orders 8 of them the same way, a coin's rate. The rank correlation with c1 is 0.67 on gin, -0.20 on httpie and -0.40 on storefront. On storefront the reference cuts the app by shared libraries and the runs by route areas, which the judges preferred: it3 scores 3.96 on c1 with an F1 of 0.59 against the reference. One engineer's reference is one valid cut among several, and F1 punishes the others.

What it does catch is collapse. The base skill's runs of click and hyperfine put the whole package in 1 top-level module against the reference's 10 and 8 (top F1 0.24), and the base storefront run in 3 against 8, the storefront run the judges scored lowest on level. So the protocol uses it only as a gate on gross failure (fewer than half the reference's top-level parts, or a top F1 under 0.5), never to rank two runs.

### Overfitting

All seven rounds were decided on the same three repositories, so a rule could fit gin, httpie and storefront without being better. The zero-base runs of express, click and hyperfine (`z0` with the base skill, `zM` with main's) make a second triple, and the protocol alternates the two.

## History of rounds 3 to 7

These rounds used absolute 1 to 5 scores on viewport shots, under the rules below.

An agent ran the ABBD setup skill on three held-out repositories (gin, httpie, storefront) once per skill variant. Base is the skill at a0a792f, it2 is c43960f and it3 is a2611d7. Each it3 run used a binary built from a2611d7, which also holds it2's changes.

- The judges scored against `rubric.md` as it stood at 4dd399e: the same criteria 1 to 6, each scored from 1 to 5.
- `judge-scores.tsv` holds every blind judge pass (Opus), one for the first round and two or three for each later iteration, each over 27 screenshots: per repository and variant, the overview and the two module pages with the most boxes, each a 1600x1100 dark-mode viewport shot taken at the top of the page. Sets were labelled P, Q and R in a seeded random order per repository, and the judge never saw the key.
- `lint-levels.tsv` is `abbd lint --json` for every run, measured with the a2611d7 binary.
- `decisions.tsv` joins both and records the verdicts. Judge scores are averaged over all judge passes, and a variant is kept by one of two rules, chosen by what it changes:
  - A skill or CLI change can change topology, so it is judged on fresh held-out runs against base and the current kept iteration. It is kept if it beats both in at least 2 of 3 repositories, on judge means, and in the third loses to neither by more than the passes' disagreement there.
  - A renderer-only change cannot change topology, so it is judged paired, against the drawing it changes, on the current kept runs re-rendered with the change's binary. The current kept runs are the held-out runs of the latest iteration kept as the working base, it3's for it5. The criteria the change targets are written in this README before the judges run. It is kept if those criteria rise, and no criterion falls by more than the passes' disagreement in any repository.
  - The paired rule was adopted after it5 failed the fresh-run rule, and it5 is the first change judged under it (see Iteration 5). The rule did not exist when it5's judges ran, so no criterion was named in advance. Labels (c5) is what its change targets, but its keep rests on a rule written with its scores in view.
- `shots/` holds the base, it3, it4 and it5 overviews, and the pages iterations 5 to 7 name.

### Iterations 2 and 3

Neither variant meets the fresh-run rule, the only rule at the time. it2 beats base in 1 of 3 repositories and is rejected.

it3 also beats base in 1 of 3 on the judge mean:

- gin is a win, 3.06 against 2.56 (+0.50).
- storefront is a tie, 3.22 against 3.22. The judge ranks it3 first there only on set of views, 4 against 2, and the fresh-run rule has no such tie-break.
- httpie is a loss, 2.33 against 2.67 (-0.34), with lines down from 2.67 to 1.67.

The evidence is thin. It is one blind judge pass over 3 images a repository, and the margins are about the size of the noise between two runs of one skill. Lint regressed too. gin's hard breaches rose from 1 to 12, and httpie's crossings from 4 to 43. The first `abbd lint --record` refusal that it3 added never fired in these runs.

it3 stays the working base for the next iteration, because its level score (c1_level) rose in all three repositories, to 3.67 to 4.00 from base's 2.33 to 3.33. It is not shown to improve on base.

The `level_advice` column of `lint-levels.tsv` comes from the a2611d7 binary. Its hub test counted lines to a frame's chips and both directions of a mutual line, so it marked most boxes of a small page as hubs. Later level checks count only lines between a view's boxes and can differ from it.

### Iteration 4

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

### Iteration 5

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

### Iteration 6

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

### Iteration 7

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

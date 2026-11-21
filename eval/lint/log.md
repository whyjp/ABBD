# Drawing measurements and decisions

This log keeps what `docs/blueprint-svg.md` measured and decided on the way to each rule: the numbers before and after a change, what a reader or a judge said, and what was tried and dropped. The rules themselves live in `docs/blueprint-svg.md`, under the same headings and rule titles as here. Each attempt at a renderer change is one row of `eval/lint/decisions.tsv`. Entries here are history. They are not kept current.

## Blueprint drawing rules

`internal/screen/fixtures_test.go` applies the checks to every view of four real states: this repository, ripgrep, Orca and pstack's skill tree.

## Decomposition / Width budget

A budget of 1270px fits that wider chrome, but it draws 21 more jogs and 32 more paths that turn more than twice, so the chrome gave the width instead. It used to stop at 920px.

## Focus views

Orca's desktop main process holds 15 units and draws 78 relations, and the median relation crosses 9 others. pstack's twelve families use each other in 42 of 66 pairs.

## Convergence / One bundle per place and direction

It used to group them by every class a line carries, and hover gives each line classes of its own, so it saw almost no split. Grouped by kind, it finds 139 lines leaving beside a bus in 105 focus views and 25 arriving beside one in 25, and every one of them drops straight from box to box, as this rule asks of a focus view.

## Convergence

The user asked for them to merge: "선 자체가 속성과 성격을 가지는 서브웨이 패턴이라기보다는 하이웨이 위에서 배정된 버스노선 패턴에 가깝습니다. 전체 뷰에서는 노선이 겹치면 선이 머지된 것처럼 보이지만 개별 노선은 개별 운행 구간을 가집니다."

Over the fixtures: crossings 163 to 136, ink crossings 521 to 481, weighed 2546 to 2427, detours 89 to 81, excess turns 1019 to 999; pstack's overview takes another drawing whose placement is one worse. Roads come in two classes, as the user put it: "점선도 근데 출발 직후와 도착 직전엔 원래의 선으로 표현되도록만 분리되면 중간 도로는 공유하는 게 더 합리적인 선택이겠다. 출발-도착 직전-후는 선 및 출발-도착 에셋의 종류가 같은 것만 통합하고 = 지방도로다, 개별 노선이 따로 배선된다. 이후 만나는 지점들만 고속도로처럼 병합되는 거지!" Before this, the dotted store→identity line on this repository's overview held the whole store fan off the CLI's road (#209). On this repository's overview store→identity is hop 4 of the primary flow, so it rides the CLI's road, and the overview keeps no paths abreast and 511px of parallel ink, as before #209. Measured over the fixtures' 244 views, with each line traced along its route: parallel ink fell from 39446px on main to 14667px with roads of one kind and 7106px with the highway, and to 0 on pstack's and ripgrep's overviews. The points where ink passes through ink fell from 977 to 615 and then 521, the checker's crossings from 205 to 163 with the highway, and the fork levels from 96 to 84. The excess turns rose from 981 to 1019 and the weighed sum from 2406 to 2546 against roads of one kind, still under half of main's 5461. The checker's crossings fell from 232 to 199.

Without them Orca's desktop main process drew 22 phantom pairs and `src/` drew 2.

## Legibility / A port sits on the side its lines flow from

On the fixtures it cut the weighed sum over the views both drawings hold from 9812 to 8116 and placement from 326 to 252. Five views are no longer dense, so their 30 focus views are gone.

## Legibility / A box's colour names it, and a line wears the colour of the box it leaves

An accent bar down the left edge stood in for both until the user rejected it as the accent-left-border card of template design, and three blind judges given the user's exemplars and that rejection preferred the border and tint on all 12 views they compared, in all 36 passes, over the bar and over two other ways tried: a thin rule under the name, and a neutral border with the last line in colour. Enforced: across the fixtures no two boxes a drawn line joins share a colour, 0 of 1167 pairs. It used to wear a reserved orange whatever box it left, the overview's main warning, but a view where most lines skip the boundary then read in one colour: pstack's overview drew all 60 of its lines orange, so only hover told them apart. Across the fixtures' views with two or more coloured sources, the share of those sources whose colour shows on a line at rest rose from 69%, 59%, 94% and 51% to 89%, 98%, 96% and 91%, and the views showing at most one line colour fell from 43 to 7. Three blind judges given the exemplars preferred it on all 5 views it changes, and scored how well a reader tells at rest which box each line comes from at 3.53 out of 5, against 2.67 before. Three blind judges given the exemplars and the rule against template design preferred the tinted exit chip to a solid one 3 to 0 on both views it changes, the solid pills reading as candy beside the one solid hub. Before this, every pointed-at box took the accent blue, so the box and the lines it lit disagreed on 322 hovers in 61 of the fixtures' 67 views (`shoot.py`'s hue check); now on none.

## Legibility / A line that passes over another breaks the one beneath

That is more than rule 7's 313 crossings, because rule 7 leaves out crossings near a line's end and between pieces of one bus, and a reader sees both.

## Legibility / A line says what crosses it

Before this, a line whose phrases were each a minority said nothing unless every relation named one via: on this repository's overview the bus of 9 lines out of abbd CLI, most of them calls, and the convention line into /abbd stood bare, and the overview labelled 23 of 39 routes. With the majority via word it labels 27, every view of this repository 255 routes against 245, label repeats stay at 30 and no route changes. Before this, labels at rest read `module text 외 18`, `Compare revisions 외 6` and `unit .md 등` on this repository's overview, and `Compare revisions 외 4` beside `Compare revisions 외 3`. When the one-word rule first came in without the share, 56 of 187 drawn phrase labels on this repository named a phrase under half their line's relations carry, such as `module text` for 2 of 20 lines on one bus, and `Compare revisions` stood three times along one overview track, each just past 200px. The share and the track cut the labelled routes over every view of this repository from 277 to 245, 29 for minority phrases and 3 for words said again on their track, and on the overview from 37 to 23 of 39, while label repeats fall from 31 to 30 and no route changes. Before this, 958 of the 1118 labels at rest over every page of the held-out obs-studio, tauri and vector runs said 호출 or 임포트, words the ink already said. A bare 등 was the rule before this one. On the held-out runs of `eval/skill-polish` and the zero-base runs of click, express and hyperfine, 363 lines named a phrase fewer than half their relations carry, 151 of them labelled, such as `diff --check 등` for 1 of 40 relations, picked only by name. Dropping such a phrase for the via word, the strict share threshold, cut the overviews' phrase labels from 62 to 15, the outcome a blind judge rejected below. The count keeps them at 68 and misstates none. Before a bus counted lines, `외` counted relations while `×` counted lines, so `git query 외 23` stood beside `×9` on this repository's overview, and a relation that two merged requests carried was counted twice, as on a focus line of 17 relations of which 12 were distinct. Before this, click's zero-base overview labelled the bus into its process environment `설정` though all three of its relations carry phrases: no phrase was lost where the lines merge, the first by name was too wide and the via word spoke. Measured after the wide layout of rule 3, summed over views, 716 relations are drawn in this repository's fixture and 323 of them lie on a line with one word, of which 247 are labelled, by 186 labels. Ripgrep labels 29 of 77, Orca 103 of 202 and pstack 463 of 834. The overviews carry 3 labels in this repository's fixture, 1 in Orca's, 8 in pstack's and 1 in ripgrep's. The room kept from a parallel line costs 13 of the 520 labels the fixtures carried without it, 2 of them on pstack's overview, and each such line keeps its word in its tooltip. This repository's own `.abbd` gives 136 of its couplings a phrase, all a reader's notes. A strict majority, the rule before this one, left the overviews on the via word: a blind judge of three held-out repositories found every overview line labelled only `호출` or `임포트` while agents had phrased most of them. Letting the most common phrase speak with 등 raises the overview paths labelled with a phrase from 6 to 12 of 65 in this repository, and on the overviews of the three held-out runs of `eval/skill-polish` (it3 redrawn as it5) from 1 phrase with 등 and 11 via words to 6 phrases with 등 and 5 via words for gin, from 2 phrases to 2 phrases and 7 with 등 for httpie, and from 2 phrases and 8 via words to 2 phrases and 4 with 등 for storefront. The module pages of this repository go from 31 to 46 paths labelled with a phrase, and from 104 to 99 labels in all, since a phrase is longer than a via word and finds room less often. Falling back to the via word when the phrase is too wide adds 2 via-word labels to this repository's module pages 3 to storefront's overview and 2 to its module pages, and changes no label in gin, httpie or the fixtures.

## Lead, zones and dress / Lead

Failing all of those it shrinks from 8px to 6px and tries again, and last of all it sits on the edge of a box its line joins, beside where the line leaves or arrives, off that box's words. Counted on the rendered SVG alone, with labels and counts as ink and a number's own arrowhead left out, the badges too close to other ink fell from 3 of 6 on main to 0, and `TestABadgeKeepsClearOfOtherInk` holds them, this repository's own pages included (user case 21).

## Lead, zones and dress / Zones

On the fixtures no title meets ink, labels and counts included (`TestAZoneTitleKeepsClearOfInk`), where main had one.

The 외부 계약 band: three blind judges preferred banding the self fixture overview's lone manifest box 3 to 0, as explaining a box that otherwise floats unnamed.

## Lead, zones and dress / Dress

Before that limit, the self overview kept 19 of its 50 lines in colour and orca's desktop main 36 of 117. A hub's view that greys accents its hub as a hand-drawn diagram does: three blind judges given the exemplars preferred that on both views it changes for closeness to them, and split on the overall criterion, the self overview for it 3 to 0 and orca's desktop main against it 2 to 1, since colour by box no longer traces a line through a dense page until it is pointed at.

## Lead, zones and dress

The setup skill does not yet ask readers to store flows, so most held-out runs get an inferred lead or none.

## Lead, zones and dress / Card

Three blind judges given the exemplars preferred the views with a card on 7 of the 8 they compared and tied the last, on the overall criterion, and on all 8 for reading hand-drawn and self-explanatory.

## Flow

A provides line is read from its contract to its provider, so its climb to the contract's chip on a frame's top edge, which every inside draws, does not count: by the line's own ends that climb was 93% to 100% of the provides lines on the insides of vector, obs-studio, tauri and zstd (#241). When the count was set, the overviews measured 2 lines and 718px in vector, 2 and 96px in obs-studio, 0 in tauri, 2 and 782px in zstd and 3 and 888px in this repository, 5%, 3%, 0%, 2% and 7% of their ink. So against the dependency axis the overviews already read one way.

A local search that moved one place at a time to where fewer pairs point back was measured and dropped. It cut the crossings from 342 to 301 across the fixtures, but the drawn climbing lines went from 87 to 92, the dashed ink rose, and three views gained phantom pairs. The numbers are in `eval/render-r6/`.

## Flow / Ordering

In Orca's `native/`, `windows-agent-hook` is the one box of twelve with a line down to the frame. It used to stand in the third of four rows, and its line ran down between two boxes of the fourth. Now it stands in the last row and drops straight to its chip. Three views change. Across the fixtures the crossings go from 342 to 340, the paths that turn more than twice from 524 to 521, and the jogs from 486 to 487. The numbers are in `eval/render-r6/`.

## Flow / Ordering three ways

Across the fixtures the crossings go from 340 to 313, the jogs from 487 to 453, the bends from 542 to 534, the paths that turn more than twice from 521 to 517, the phantom pairs from 73 to 69, the braid from 43033px to 39623px, and the dashed ink from 46981px to 44548px. Some views grow only in ink. pstack's `core principles` draws 637px more while it loses 3 crossings, 9 jogs and its 3 phantom pairs, and four other focus views of the mobile app draw 6px to 12px more. Judging each view with its focus views was measured and dropped. It took ripgrep's overview to 8 more jogs and 1 more crossing. Judging the ways on the turns along each line's whole way, buses included, in place of or on top of the turns of each piece of ink, was measured with the wide drawings and dropped. It took the jogs from 433 to 429 and the crossings from 270 to 280, added 1200px of braid, and sent one line of `/abbd-setup` 418px out of its way. That holds for 81 of the 83 extra drawings whose boxes keep an earlier way's rows, and skipping those changes three of Orca's views. The numbers are in `eval/render-r6/`.

## Flow / Room to run straight

In Orca's `src/` the widest row now runs 1113px where it ran 917px, and in desktop main 1308px where it ran 896px. Across the 274 views the crossings go from 313 to 274, the phantom pairs from 69 to 52, the bends from 534 to 454, the paths that turn more than twice from 517 to 462 and the climbing lines from 87 to 74. The jogs go from 453 to 457 (see Known gaps). Desktop main's crossings go from 87 to 76 and its bends from 36 to 10, and `src/`'s crossings from 43 to 23. The mean width of a unit's view goes from 647px to 746px. The numbers are in `eval/render-r6/wide.tsv`.

## Flow / Staircases first

It sent a line of `/abbd-setup` 418px out of its way, sent 5 of the CLI's 20 lines up where rule 13 allows 4, gave pstack's `principles` 3 back lines and a channel of 5 lines, and gave desktop main a pair of lines closer than 5px and a line 608px out of its way. Across the 274 views (`eval/render-r6/jogs-first.tsv` against `wide.tsv`), the steps go from 1673 to 1438, the jogs from 457 to 408, the crossings from 274 to 265 and the paths that turn more than twice from 462 to 417. The bends along the traced paths go from 5168 to 4960. The phantom pairs stay at 52 and the climbing lines at 74. The bends of the strokes go from 454 to 453, the braid from 41798px to 43964px, and the dashed ink from 40850px to 39749px. The metrics table records each view's steps and traced bends (`steps`, `traced_bends`), so `eval/render-r6/compare.py` totals them.

## Flow / Judged as lint reads it

Judged on the weighed sum alone, the placement debt rose from 366 to 376, past the gate. With the placement breaches counted, pstack's `poteto-mode, the front door` took a packed drawing that folded 6 of its chips and read worse. With folded chips weighed first of all, pstack's `setup and help` took a drawing that folds 4 fewer chips but weighs 55 more. Across the 274 views the excess turns go from 3172 to 3102, the weighed sum from 12953 to 12714, the placement debt from 366 to 364, the traced bends from 4960 to 4954 and the paths that turn more than twice from 417 to 411. The crossings go from 265 to 306. The hard breaches stay at 61. Orca's mobile app loses 12 excess turns and its `mobile/components` focus view 18. Orca's takes 2.2s where it took 1.05s. Two changes that leave every page, the lint table and the metrics table byte for byte as they were take it to 1.72s, against 1.04s on main, best of 3 runs of 3 builds each.

## Flow / Chips seated in the layout

Twenty of the 65 views of a unit take a seated drawing. Three variants were measured before the rule gate, with jogs alone first. Before the gate, the new judge without seated tactics took the jogs only to 393, with 300 crossings.

## Flow / Steps before excess turns

So Orca's mobile app took a seated drawing with 51 crossings where main had 28, and 34 jogs where main had 28, though lint's excess turns fell from 215 to 200. pstack's `core principles` went from 8 crossings to 17 and from 67 steps to 71, Orca's `desktop renderer UI` from 17 crossings to 23, and the mobile app's `mobile/terminal` and `mobile/transport` focus views from a weighed 16 and 56 to 28 and 76. Over the 274 views the crossings go from 298 to 262, below main's 265, the steps from 1415 to 1411, the jogs from 385 to 377 and the placement debt from 365 to 363. The excess turns go from 2927 to 2975 and the weighed sum from 12006 to 12162, which keeps 197 of the 245 excess turns this branch saves on main and 791 of the 947 weighed. The traced bends go from 4806 to 4808 and the paths that turn more than twice from 411 to 419. Against main only three views gain a step or a jog or a crossing: the abbd CLI 3 steps and 1 jog, the mobile app 3 steps and 1 jog, and pstack's `verification` 2 steps. The weighed ceilings of the self, orca and pstack fixtures rise from what the lint-first judge recorded, 1878, 4817 and 4783, to 1883, 4933 and 4818, still below main's 2128, 5119 and 5094.

## Flow / One measure for chooser and gate

Orca's mobile app took a drawing 18 cheaper by the chooser and 39 dearer by the gate (`eval/lint/decisions.tsv` rows 102, 104 and 108). Every drawing of the fixtures stays as it was, and the ceilings went from self 1135, ripgrep 199, Orca 3739 and pstack 2075 to 748, 96, 3155 and 1546. Pricing the tees at 9 in both instead redraws Orca's `desktop renderer UI`, whose lines between boxes in one column that turn go from 14 to 26.

## Flow / Straight drops between next rows

Pinning box-to-box lines alone gave back 96 of the excess turns, 16 jogs and 84 traced bends, and broke the weighed ceiling of every fixture. Across the 274 views the excess turns go from 3102 to 2927, the weighed sum from 12714 to 12006, the jogs from 414 to 385 and the traced bends from 4954 to 4806. The crossings go from 306 to 298, and the placement debt from 364 to 365. The `/abbd` inside loses 10 excess turns and Orca's mobile app 8.

## Tracing / A dot where a join could pass for a crossing

Dropping the T dots took the fixtures' dots from 1104 to 191, pstack's overview from 38 to 12 and ripgrep's from 21 to 4. A T keeps its dot where a gap (rule 24) cuts its line closer than gapJoin (12px) to it: the cut leaves a stub that reads as a corner, and the line past the gap seems to start from nothing, as at pstack's MOD-0010 486,206.

## Tracing / No phantom paths

The user chose that on purpose: each line's route still runs from its own start to its own end, pointing at a box or a road lights each line whole in its colour, and graph.json and the stored relations keep every line's own ends, so neither a reader who points nor an agent who reads the files misreads it. On the held-out gin and httpie overviews that took the phantom pairs from 12 and 7 to 0. Measured: every view's phantom pairs are in the metrics table, 69 across the fixtures, down from 191. pstack's overview holds 52 of them. Of the other 17, 14 are in pstack's insides, 2 in Orca's and 1 in this repository's `internal/store`.

## Tracing / A count belongs to one line

Over the fixtures' pages this takes the counts from 499 to 249, with label repeats at 162 and labelled routes from 475 to 477.

## Bends

Orca's `src/` has such a spot at the `native/` chip, whose line to desktop main turns twice in a clear column.

On the 244 views of the fixtures that takes the detours from 159 to 137 and the weighed sum from 8086 to 7713 (`eval/lint/decisions.tsv` rows 67 to 75).

## Bends and relations / J3. The ink joins no two places that no relation joins

A drawing whose lines ride roads and that holds their routes is exempt, as rule 18 says, so the 52 phantom pairs pstack's overview carried as debt went with it. Hover once dropped the routes of a drawing past 48 shapes or 96 routes while its lines stayed on roads, so vector's vector-lib, at 98 lines, owed 131 J3 though its ink matched any other road view's.

## One screen / S. The drawing fits one screen

These were measured on the page at that size. When S was set, 7 of the fixtures' 244 views broke it (pstack's overview and 6 of Orca's module pages), and 2 of the 158 views of six held-out runs (the overviews of httpie and hyperfine). A new ranking moves every box's centre and so every focus view seated by them, and taking one only to save turns let one new relation between two modules redraw ripgrep's overview at 1234x740 in place of the plain 1308x652 that fits.

## What a change is weighed by

The weighed sum once priced each tee at 9 and each excess turn at 4, while the chooser priced them at 1. When the ceiling was set, the drawings stayed as they were and the fixtures measured self 269, ripgrep 49, Orca 864 and pstack 475 excess turns, and this repository 237.

When the criteria were set, the fixtures' 274 views measured as follows.

This repository's own blueprint (`eval/lint/self.txt`) owes one hard breach, and `.abbd/lint-debt.yaml` records it, so `abbd lint` passes here. It did so on main too, drawn there as bus pieces that each ran one way, which the checker, measuring each path, never saw.

`eval/lint/acceptance.tsv` turns each complaint the user repeated, from a screenshot in `D:/abbd-eval/exemplars` or a review, into one exact measure on one fixture view, such as the turns of grep's line to grep-regex on ripgrep's overview, how far grep's line to grep-matcher runs past its ends, or how many stems and tracks app-shell's lines leave it by in its focus view.

When the criteria were set, the fixtures' 274 views measured as follows.

| Total | Fixtures | Gate |
| --- | --- | --- |
| hard breaches | 61, all debt (J3 52, R 9) | the debt |
| J1, J2, O3 | 0, 0, 0 | hard |
| B1, B2 | 1145, 302 | through the excess turns |
| excess turns | 3172 | the target, through the weighed sum and the ceiling, per fixture |
| crossings | 265 | through the weighed sum |
| weighed sum | 12953 | per fixture, self 2128, ripgrep 612, Orca 5119, pstack 5094 |
| P1, P2, H1, H2, O1, O2, F1 | 117, 21, 15, 6, 1, 8, 198 | through the placement debt |
| placement debt | 366 | per fixture, self 50, ripgrep 9, Orca 131, pstack 176, each plus 2% |
| steps | 1438 | reported |
| jogs | 408 | in the excess turns |
| traced bends | 4960 | tie-breaker |
| paths that turn more than twice | 417 | tie-breaker |
| detours (B3, B4) | 159 on the 244 views of the fixtures with the routing before the step rule | the ceiling, per fixture |
| phantom pairs | 52 | J3 |
| climbing lines | 74 | rule 13 |

## Pages

No relation leaves a view, and all 274 views measure as before:

Now they rewrite 60 files, 171523 bytes and 1083 lines. The causes left over on main were the quotes' lines (67700 bytes), the overview's focus views on `index.html` (58456), the drawings in `graph.json` (27144), the mask names (7186), the colours (5313) and the nav (5274, but in 17 of 20 files when a module was added). Adding a module went from 20 files to 5, adding a CLI command from 19 to 5, and moving a quote's lines alone from up to 7 files to none, which the test below holds. Its bytes went from 97647 to 83138, since the focus views it reorders now sit on the pages of their units.

Within one view they can shift: a new dependency on the overview recoloured boxes worth 2453 bytes of `index.html`.

## What the overview draws

The overview used to leave reader's couplings out, and on Orca (evaluation round 5) that erased every real platform boundary: the phone-to-relay WebSocket, the E2EE handshake, the push gateway and the native helper sockets were all found by readers, mostly on units inside a module.

## Static lines rest in the list

The user read the dotted lines as connections that tag along, like an include, and asked that they not be drawn at rest. A census of 1944 declaration-only relations over the 15 evaluation sets and this repository found that 97% of imports and includes do use the target's symbols, by a symbol search outside the store. So the dotted line is named 정적 의존 (호출 근거 없음), not "unused". The store cannot tell a used static dependency from an unused one. Measured with the lines left out at render time, against the drawing of every static line, over the 15 sets: inside lines fell from 2787 to 1999, crossings from 298 to 188, phantoms from 2802 to 1848, and target breaches from 1370 to 944. Hard breaches went from 16 to 15, and the overviews' target breaches from 153 to 149. Gate failures went from 375 to 376. The four fixtures drew the same, since readers stated their static relations. This repository's overview leaves one static pair out, internal/identity to internal/extract, and its doglegs rose from 20 to 21. Hiding by relation instead, deleting the relations from the store, also cost the focus views of every box that drew only static lines. A first cut that kept no line for the lead took the inferred lead off overviews; the lead now keeps a line into every box something uses. Of 184 insides, 72 moved some box, 24 changed the order of a row and 6 changed a row. 81 inside focus views and 11 overview focus views were added for the lines left out, and 32 went where an inside is no longer dense.

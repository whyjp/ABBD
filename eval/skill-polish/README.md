# Skill polish runs

An agent ran the ABBD setup skill on three held-out repositories (gin, httpie, storefront) once per skill variant. Base is the skill at a0a792f, it2 is c43960f and it3 is a2611d7. Each it3 run used a binary built from a2611d7, which also holds it2's changes.

- `rubric.md` is what the judge scored against.
- `judge-scores.tsv` holds one blind judge pass (Opus) over 27 screenshots: per repository and variant, the overview and the two module pages with the most boxes, each a 1600x1100 dark-mode viewport shot taken at the top of the page. Sets were labelled P, Q and R in a seeded random order per repository, and the judge never saw the key.
- `lint-levels.tsv` is `abbd lint --json` for every run, measured with the a2611d7 binary.
- `decisions.tsv` joins both and records the verdicts. A variant is kept when it beats base on the judge in at least two of three repositories and does not lose badly on the third.
- `shots/` holds the base and it3 overviews.

---
unit: DATA-0006
slot: semantics
origin: INFERRED
digest: 931d2a6257644bef
format: 3
evidence:
    - path: internal/screen/screen.go
      lines: "111"
      quote: 'case in.Synced.Merged || in.Tree != in.Synced.Fingerprint:'
    - path: internal/gitx/gitx.go
      lines: "113"
      quote: add := []string{"add", "-A", "--", "."}
    - path: internal/gitx/gitx.go
      lines: "124"
      quote: add = append(add, ":(exclude,literal)"+r)
    - path: internal/gitx/gitx.go
      lines: "141"
      quote: '"update-index", "--add", "--cacheinfo", "160000,"+l.commit+","+prefix+l.path'
code:
    internal/gitx/gitx.go#quote-995a9513: 788b9307578d7627
    internal/gitx/gitx.go#quote-ab1b93ce: c7359a4a3a0f2c0a
    internal/gitx/gitx.go#quote-df97c10f: 5d656d3468993950
    internal/screen/screen.go#quote-48139f5a: c60ba95ebc6da4c9
---

`state.yaml` says which code the stored units were synced from. `fingerprint` is a git tree object of the working tree as `gitx.Snapshot` writes it into a scratch index. It holds every file `git add -A` would stage, untracked files included, with `.abbd/` left out, by an exclude pathspec unless `.gitignore` already ignores it, as a whitelist `.gitignore` does. A submodule counts as the commit the superproject's index records, not the one checked out, so every clone of a commit fingerprints the same tree. A nested repository with no commit yet, which `git add` refuses, counts by its files on disk that neither repository ignores, as an untracked directory does.

The state is **CURRENT** exactly when the fingerprint of the tree now equals the stored one and no merge left two syncs in the file. Any edit to code, staged or not, makes it **STALE** until `abbd sync` runs. Committing changes nothing, because a commit moves HEAD and not the tree. `revision` only names where that tree was first synced, and `blueprint` is the inputs hash the blueprint was rendered from, for the blueprint's own check. Neither decides whether the state is current.

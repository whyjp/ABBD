---
unit: DATA-0006
slot: semantics
origin: INFERRED
digest: fee8fc3631f4e93c
format: 3
evidence:
    - path: internal/screen/screen.go
      lines: "106"
      quote: 'case in.Tree != in.Synced.Fingerprint:'
    - path: internal/gitx/gitx.go
      lines: "109"
      quote: add := []string{"add", "-A", "--", ".", ":(exclude)" + exclude}
    - path: internal/gitx/gitx.go
      lines: "114"
      quote: add = append(add, ":(exclude,literal)"+r)
    - path: internal/gitx/gitx.go
      lines: "130"
      quote: '"update-index", "--add", "--cacheinfo", "160000,"+l.commit+","+prefix+l.path'
code:
    internal/gitx/gitx.go#quote-ab1b93ce: c7359a4a3a0f2c0a
    internal/gitx/gitx.go#quote-d5b42f38: e282009c15b31d31
    internal/gitx/gitx.go#quote-df97c10f: 59719bb401db9166
    internal/screen/screen.go#quote-3c3de2d3: 0301285aafec1aeb
---

`state.yaml` says which code the stored units were synced from. `fingerprint` is a git tree object of the working tree as `gitx.Snapshot` writes it into a scratch index. It holds every file `git add -A` would stage, untracked files included, with `.abbd/` left out. A submodule counts as the commit the superproject's index records, not the one checked out, so every clone of a commit fingerprints the same tree. A nested repository with no commit yet, which `git add` refuses, counts by its files on disk that neither repository ignores, as an untracked directory does.

The state is **CURRENT** exactly when the fingerprint of the tree now equals the stored one. Any edit to code, staged or not, makes it **STALE** until `abbd sync` runs. Committing changes nothing, because a commit moves HEAD and not the tree. `revision` only names where that tree was first synced, and `md_hash` is the hash of the unit files for the blueprint's own check. Neither decides whether the state is current.

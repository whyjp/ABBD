---
unit: DATA-0006
slot: semantics
origin: INFERRED
digest: 4842d1f37c92a3e8
format: 3
evidence:
    - path: internal/screen/screen.go
      lines: "104"
      quote: 'case in.Tree != in.Synced.Fingerprint:'
    - path: internal/gitx/gitx.go
      lines: "99"
      quote: add := []string{"add", "-A", "--", ".", ":(exclude)" + exclude}
    - path: internal/gitx/gitx.go
      lines: "107"
      quote: '"update-index", "--add", "--cacheinfo", "160000,"+l.commit+","+l.path'
code:
    internal/gitx/gitx.go#quote-ad73090a: 373a55d5dcf14346
    internal/gitx/gitx.go#quote-d5b42f38: a1c0325626cc3b33
    internal/screen/screen.go#quote-3c3de2d3: 0301285aafec1aeb
---

`state.yaml` says which code the stored units were synced from. `fingerprint` is a git tree object of the working tree as `gitx.Snapshot` writes it into a scratch index. It holds every file `git add -A` would stage, untracked files included, with `.abbd/` left out. A submodule counts as the commit the superproject's index records, not the one checked out, so every clone of a commit fingerprints the same tree.

The state is **CURRENT** exactly when the fingerprint of the tree now equals the stored one. Any edit to code, staged or not, makes it **STALE** until `abbd sync` runs. Committing changes nothing, because a commit moves HEAD and not the tree. `revision` only names where that tree was first synced, and `md_hash` is the hash of the unit files for the blueprint's own check. Neither decides whether the state is current.

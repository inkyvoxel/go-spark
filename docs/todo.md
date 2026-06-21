# To-do

Working list toward a v1 feature-complete release. Tackle top to bottom.
Each item notes the why and the main files to touch.

## 1. Harden init.sh substring + sed edge cases

`scripts/init.sh` replaces module path, then `Go Spark`, then `go-spark`. If a
user's module path itself contains `go-spark` (e.g.
`github.com/me/go-spark-fork`), the third pass corrupts it. `sed` replacement
also breaks if the module path/name contains `&` or `|`.

- Add a guard/escape or at least a documented note. CI `init-script` job already
  covers the common case.
- Files: `scripts/init.sh`.

## 2. Minor cleanups

- `.gitignore` references `.gospark-init-state`, which nothing creates. Remove
  the line.
- `internal/server/architecture_boundary_test.go` calls `t.Helper()` inside a
  top-level `Test` function (no-op there). Drop it.

## 3. Write the CHANGELOG before tagging v1

`CHANGELOG.md` is still all "Unreleased / Initial scaffold". Fill in a real v1
entry before shipping a forkable release.

- Files: `CHANGELOG.md`.

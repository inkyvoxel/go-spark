# To-do

Working list toward a v1 feature-complete release. Tackle top to bottom.
Each item notes the why and the main files to touch.

## Write the CHANGELOG before tagging v1

`CHANGELOG.md` is still all "Unreleased / Initial scaffold". Fill in a real v1
entry before shipping a forkable release.

- Files: `CHANGELOG.md`.

## Move `DefaultPasswordMinLength` out of `internal/auth`

`config` and `cmd` depend on `auth.DefaultPasswordMinLength`, so `internal/config`
imports `internal/auth` just for one constant — config depending on a feature
package reads backwards. No import cycle today, but a config-owned default (or a
small shared constants home) would be cleaner.

- Files: `internal/auth/auth.go`, `internal/config/`, `cmd/app/main.go`.

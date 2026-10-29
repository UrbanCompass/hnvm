# Contributing

HNVM is a Go binary (`cmd/hnvm`, `internal/`) that acts as `node`, `npm`, `npx`, `pnpm`, `pnpx`,
`yarn`, `bun` and `bunx` depending on the name it is run as, and as the `hnvm` management command
otherwise.

- `make test`: unit tests, offline (`go test -race ./...`)
- `make lint`: `go vet`, `gofmt`, `shellcheck`
- `make build`: builds `dist/hnvm`, which `bin/*` run
- `make parity`: the jest suite in `test/` against `dist/hnvm`

The data files HNVM embeds (`.hnvmrc`, `lib/hnvm/min-safe-node.txt`, `lib/hnvm/distrusted-certs.txt`,
`lib/hnvm/distrusted-placeholder.pem`) are compiled in, so changing them requires a release.

# Library files

The bash implementation in `lib/hnvm/*.sh` stays until the jest tests that source it directly are
ported. The bootstrap shims fall back to it only while `lib/hnvm/bootstrap-version` is the
unreleased placeholder `0.0.0`.

Any library files needed by core hnvm logic should be only placed into `lib/`. The reason for this is so that the same library files can be installed in the same place on disk by our Homebrew formulas. That way, the import references are consistent for both source code/source tests and for end-user homebrew installations of hnvm.

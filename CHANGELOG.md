# Changelog

## 2.0.0 (unreleased)

HNVM is now a single Go binary. The scripts in `bin/` are bootstrap shims that download and run it,
so Homebrew, basher and `install.sh` setups keep working. `$HNVM_PATH` keeps its layout, so 1.x and
2.x can share `~/.hnvm`.

### Added
- The `hnvm` command: `current`, `which`, `config`, `install`, `list`, `uninstall`, `prune`, `exec`,
  `doctor` and `shims`.
- Installs are locked per version and published with an atomic rename, so parallel first runs no
  longer corrupt each other.
- pnpm is installed from the registry tarball and checked against the packument's sha512 integrity.

### Changed
- **Environment variables now override `.hnvmrc` files for every setting**, as documented. Before,
  an `.hnvmrc` value of `HNVM_PATH`, `HNVM_QUIET`, etc. overrode the environment.
- `.hnvmrc` files are parsed as strict `KEY=VALUE` lines. Values may contain spaces and quotes, and
  only `HNVM_*` keys are read; an `.hnvmrc` can no longer set arbitrary environment variables.
- Tools no longer inherit HNVM's internal variables (`node_bin`, `node_ver`, `COMMAND_OUTPUT`, the
  resolved `HNVM_*` settings, ...). pnpm still gets `npm_config_scripts_prepend_node_path` and
  `npm_config_enable_pre_post_scripts`.
- Linux arm64 now gets arm64 node (was x64).
- Semver ranges are resolved without a bootstrap node.
- No runtime dependency on `jq`, `curl`, `tar`, `unzip`, `git` or `defaults`.
- `marie-kondo` no longer needs `jot`, so it works on Linux.

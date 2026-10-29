# HNVM: Hermetic Node Version Manager

Hermetically sealed versions of [node](https://npmjs.org) and [pnpm](https://pnpm.js.org).

Instead of relying on the version of `node`, `npm`, `pnpm`, etc. each individual developer or
machine has installed locally, packages use a system we've named HNVM, which stands for Hermetic
Node Version Manager. Having our `node` binaries be hermetic means each app or package defines what
version of `node` they depend on. That version is installed and used to run whenever the package
runs scripts in `node`, `npm`, `pnpm`, `yarn`, and `bun`. This ensures that everyone's systems
output consistent packages, and upgrades required by one package don't affect another.

## Installation

### [Homebrew](https://brew.sh) (Compass Employees Only)

HNVM is distributed via [Homebrew](https://brew.sh) inside the
[`UrbanCompass/versions` tap](https://github.com/UrbanCompass/homebrew-versions):

```sh
brew tap UrbanCompass/versions

# If you get a permissions error tap'ing, try using the repo's ssh url
brew tap UrbanCompass/versions git@github.com:UrbanCompass/homebrew-versions.git

brew install hnvm
```

### [Basher](https://github.com/basherpm/basher)

You can also use [basher](https://github.com/basherpm/basher) to install hnvm:

```sh
basher install UrbanCompass/hnvm
```

The scripts in `bin/` are small bootstrap shims: they run the `hnvm` binary for the version pinned
in `lib/hnvm/bootstrap-version`, downloading it once from GitHub Releases into
`$HNVM_PATH/.hnvm-bin/` and verifying it against the release's `checksums.txt`. Set
`HNVM_RELEASE_DIST` to download from a mirror instead.

### Release binary

Download `hnvm_<os>_<arch>.tar.gz` from the [releases](https://github.com/UrbanCompass/hnvm/releases),
put `hnvm` somewhere permanent, and create the tool symlinks in a directory on your `$PATH`:

```sh
hnvm shims ~/.local/bin   # creates node, npm, npx, pnpm, pnpx, yarn, bun and bunx
```

### From source

```sh
make build       # builds dist/hnvm, which the scripts in bin/ use
source install.sh
node -v # Uses hnvm

source uninstall.sh
node -v # No longer uses hnvm
```

HNVM is a single static binary with no runtime dependencies (no `jq`, `curl`, `tar` or `unzip`).

## The `hnvm` command

| Command | |
|---|---|
| `hnvm current [tool...]` | The version of each tool here, and which setting decided it (`package.json`, an `.hnvmrc`, the environment, min-safe-node) |
| `hnvm which <tool>` | The file a tool runs |
| `hnvm config [--json]` | Every setting with its value and source, including managed (MDM) values |
| `hnvm install [tool[@version]...]` | Download ahead of time, e.g. to warm a CI cache |
| `hnvm list` | Installed versions and when they were last used |
| `hnvm uninstall <tool@version>...` | Remove versions |
| `hnvm prune [--keep-used-days N] [-n]` | Remove versions not used recently (never the ones configured here) |
| `hnvm exec <tool> [args...]` | Run a tool without relying on the shims being on `PATH` |
| `hnvm doctor [--json] [--offline]` | Check `PATH` shadowing (nvm, volta, brew), `HNVM_PATH`, installs, certificates and reachability; exits non-zero on failure |
| `hnvm shims [dir]` | Create the tool symlinks |

## Usage

HNVM reads the version of `node`/`pnpm`/`yarn`/`bun` set in your `package.json` file' `"engines"`
field. If no version is set, it will default to the current versions set in HNVM's own
`package.json`. Unlike HNVM 1.0, you don't have to find any particular bash script to run HNVM. Just
use the regular `node`, `npm`, `bun`, etc. commands you're used to from anywhere on your computer. If
you run it next to a `package.json` file, it will read the engines field. If not, it'll default to
the global version.

Because `bun` is a standalone native binary, `bun` and `bunx` do not require a hermetic `node` to be
installed — they run directly.

```js
// main.js
console.log('Node version', process.version);
```

```sh
# package.json in this directory has the hnvm version set to 12.10.0

node main.js # Echo's: "Node version v12.10.0
```

## Configuration

HNVM reads its configuration from one of the following places, in order from highest to lowest
priority:
1. Environment variables (`HNVM_NODE=20.1.0 node -v`)
1. The `package.json` file in the current process working directory (`$PWD/package.json`)
2. An `.hnvmrc` file in the `PWD` (`$PWD/.hnvmrc`)
3. An `.hnvmrc` file at the root of your git repo (if running in a git repo)
4. An `.hnvmrc` file in your home directory (`~/.hnvmrc`)
5. The `.hnvmrc` file that ships with the current version of HNVM

The `.hnvmrc` files are all simple `KEY=VALUE` lines prepended with `HNVM_` (`#` starts a comment,
and other keys are ignored):
```sh
HNVM_PATH=/path/to/.hnvm
HNVM_NODE=10.0.0
HNVM_PNPM=3.0.0
HNVM_YARN=1.19.0
HNVM_BUN=1.3.14
```

The full list of config options are detailed below.

Versions configured in `package.json` files go in either the `"engines"` field or an
`"hnvm"` field, with the latter overriding the former:
```js
{
  "name": "some-package",
  "version": "1.0.0",
  "engines": {
    "node": "10.0.0",
    "pnpm": "3.0.0",
    "yarn": "1.19.0",
    "bun": "1.3.14"
  },
  "hnvm": {
    "node": "11.0.0" // This overrules any versions set in "engines"
  }
}
```

The `"hnvm"` field can contain the same values as in the `"engines"` field. It's useful for when
you want HNVM to be stricter than your engines. For example, your package might be compatible with a
node version range, but HNVM itself runs at a specific version:

```js
{
  "name": "some-package",
  "version": "1.0.0",
  },
  "engines": {
    "node": ">=10.0.0"
  },
  "hnvm": {
    "node": "11.0.0"
  }
}
```

### `HNVM_PATH` (Defaults to `~/.hnvm`)

Location on disk to download binaries to, defaulting to an `.hnvm` directory in your `$HOME`
directory.

### `HNVM_NODE`, `HNVM_PNPM`, `HNVM_YARN`, `HNVM_BUN` (Defaults to `latest`)

Version of `node`/`pnpm`/`yarn`/`bun` to use. If semver ranges are provided instead of exact versions
(e.g. the defaults are set to `latest`), HNVM will perform curl requests to resolve those to an
exact version. However you'll get a warning about this since it could slow down execution time from
the async request, or it might even fail to work at all if the curl requests fail to load.

It's best to create an `.hnvmrc` file in your home directory and set the versions to exact versions.

If you want to temporarily override any of these values, you can override the env vars when your
run the script:

```sh
env HNVM_NODE='12.0.0' node --version # v12.0.0
```

### `HNVM_RANGE_CACHE` (Defaults to 60)

To try to mitigate the slowdown above, semver range results are cached locally. The default is 60s
but you can control this by setting the `HNVM_RANGE_CACHE` environment variable. To disable the
cache set the value to 0.

### `HNVM_QUIET` (Defaults to `false`)

HNVM outputs information about the current version of node running, and any download statuses if a
new version is being downloaded so that you know exactly what's going on. If you don't want this
output, set the `HNVM_QUIET` environment variable to `true` to have this output redirected to
`/dev/null`.

### `HNVM_MIN_SAFE_NODE` (Defaults to `true`)

After the Node version is resolved, hnvm checks it against the rules in `lib/hnvm/min-safe-node.txt`. A
Node release that is below the fixed release for its own major line (for example 22.1.0, when 22.23.4 fixes
CVE-2026-111497) is replaced by that fixed release, with a warning that names both versions. The pin in your
`package.json` or `.hnvmrc` is not edited. Set `HNVM_MIN_SAFE_NODE=false` to run exactly the version you pinned.
Majors newer than the highest rule are never changed.

The warning is not hidden by `HNVM_QUIET`. It does follow `HNVM_OUTPUT_DESTINATION`, and a failed write (for example, a closed stderr) never stops hnvm from running node.

### `HNVM_MIN_SAFE_NODE_MAJOR` (Defaults to `false`)

Some pinned lines have no fix at all (23, 25, and anything below 22). By default hnvm only warns about those and
runs the pinned version, because moving to another major can break native add-ons. Set
`HNVM_MIN_SAFE_NODE_MAJOR=true` to have hnvm run the fixed release named in `min-safe-node.txt` instead (24.22.1,
or 26.12.1 for 25). The next security incident only needs a new line in that file.

### Managed (MDM) settings for the two keys above

`HNVM_MIN_SAFE_NODE` and `HNVM_MIN_SAFE_NODE_MAJOR` can also be set by an MDM configuration profile, which wins over the
environment and every `.hnvmrc`, and which a user cannot edit. See [Managed (MDM) settings](#managed-mdm-settings).

### `HNVM_NODE_DIST` (Defaults to `https://nodejs.org/dist`)

Location of the NodeJS distribution files. If you provide a custom destination, you should ensure
that the repository layout mirrors that of nodejs.org:
```
/node-v${node_ver}-${platform}-${cpu_arch}.tar.gz
```

### `HNVM_NODE_VARIANT` (Defaults to '')

Variant of the NodeJS distribution files to fetch. This exists mostly to support the `musl` builds.

### `HNVM_SKIP_URL_VALIDATION` (Defaults to `false`)

When set to `true`, skips the HEAD request validation before downloading packages. By default, hnvm validates that download URLs are accessible before attempting to download, which provides faster failure feedback and clearer error messages. Set this to `true` if you want to skip validation (e.g., in environments where HEAD requests are blocked but GET requests work).

### `HNVM_PNPM_REGISTRY` (Defaults to `https://registry.npmjs.org`)

If using the `--with-pnpm` flag, `npm`-compatible registry to install `pnpm` from. Defaults to the
public `npm` registry.

### `HNVM_YARN_DIST` (Defaults to `https://yarnpkg.com/downloads`)

If using the `--with-yarn` flag, the location from where to download `yarn` from. When providing a
custom destination, you should ensure the layout mirrors that of `yarnpkg.com`:
```
/${yarn_ver}/yarn-v${yarn_ver}.tar.gz
```

### `HNVM_BUN_DIST` (Defaults to `https://github.com/oven-sh/bun/releases/download`)

Location from where to download `bun` from. When providing a custom destination, you should ensure
the layout mirrors that of bun's GitHub releases:
```
/bun-v${bun_ver}/bun-${platform}-${cpu_arch}.zip
```
where `platform` is `darwin`/`linux` and `cpu_arch` is `x64`/`aarch64`.

### `HNVM_BUN_VARIANT` (Defaults to '')

Variant of the bun distribution files to fetch. Set to `baseline` for CPUs without AVX2 support, or
`musl` for musl-based Linux distributions (e.g. Alpine). This is appended to the download filename
(e.g. `bun-linux-x64-musl.zip`).

### `HNVM_DISTRUST_CERTS` (Defaults to `true`)

Node bundles its trusted root certificates inside the `node` binary, so an old Node version keeps
trusting a root CA even after that CA has been compromised. Projects can pin old versions
indefinitely. To keep them safe, HNVM removes known-distrusted roots from every Node binary it
manages. The list lives in [`lib/hnvm/distrusted-certs.txt`](lib/hnvm/distrusted-certs.txt) and
currently covers the
[eMudhra CA compromise](https://emudhra.we-have-a-problem.com) (`emSign ECC Root CA - G3` and
`AAA Certificate Services`).

HNVM patches a binary the first time that version is used, both for new downloads and for versions
already in `HNVM_PATH`. Each removed root is overwritten in place with an expired placeholder
certificate. On macOS the binary is then re-signed ad hoc, keeping its entitlements. Before HNVM
swaps the patched binary in, it checks that the binary runs and no longer trusts the removed roots.
The result is recorded in `${HNVM_PATH}/node/<version>/.hnvm-distrusted-certs` so later runs skip
the check.

This applies to `node`, `npm`, `npx`, `pnpm` and `yarn`, which all run through the patched node.
It does not apply to `bun`. It also does not touch the OS certificate store, which Node reads
instead when run with `--use-system-ca`/`--use-openssl-ca`, or which you may point
`NODE_EXTRA_CA_CERTS` at.

Set to `false` to opt out. Patching is one-way: turning it off later does not restore a patched
binary. To get the original back, delete `${HNVM_PATH}/node/<version>` and HNVM re-downloads it.

### `HNVM_DISTRUST_CERTS_EXTRA` (Defaults to '')

Comma-separated SHA-256 fingerprints of additional root certificates to remove, on top of the
built-in list. Colons, case and a leading `sha256 Fingerprint=` label don't matter, so the output
of `openssl x509 -noout -fingerprint -sha256` works as-is. Anything that isn't a SHA-256
fingerprint after that is rejected (a warning, or an error with `HNVM_DISTRUST_CERTS_STRICT=true`).
The valid fingerprints are still removed, and the warning repeats on every run until the list is fixed.
Changing this list re-patches each node version the next time it's used.

### `HNVM_DISTRUST_CERTS_STRICT` (Defaults to `false`)

If HNVM can't remove the distrusted roots from a node binary (for example the binary can't be
written, re-signing fails, or the patched binary fails verification), by default it prints a warning,
keeps the original binary and runs it anyway. It tries again on the next run. Set to `true` to
refuse to run instead.

### Managed (MDM) settings

`HNVM_MIN_SAFE_NODE`, `HNVM_MIN_SAFE_NODE_MAJOR`, `HNVM_DISTRUST_CERTS`, `HNVM_DISTRUST_CERTS_EXTRA` and
`HNVM_DISTRUST_CERTS_STRICT` can also be set by an MDM configuration profile, which wins over the environment and every
`.hnvmrc`, and which a user cannot edit. Deploy a Custom Settings payload for the preference domain `com.compass.hnvm`.
`HNVM_DISTRUST_CERTS_EXTRA` is an array of fingerprints (a comma-separated string also works); the other keys are booleans:

```xml
<key>HNVM_MIN_SAFE_NODE</key><true/>
<key>HNVM_MIN_SAFE_NODE_MAJOR</key><true/>
<key>HNVM_DISTRUST_CERTS</key><true/>
<key>HNVM_DISTRUST_CERTS_STRICT</key><true/>
<key>HNVM_DISTRUST_CERTS_EXTRA</key>
<array>
  <string>0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef</string>
</array>
```

macOS writes the profile to `/Library/Managed Preferences/com.compass.hnvm.plist`, and hnvm reads it with `defaults`.
A key that is not in the profile falls back to the environment, then `.hnvmrc`, then the default. Only these keys are
read from there for now. For the booleans, values of `true`, `1` and `yes` (or `false`, `0` and `no`), in any case, are
accepted; anything else is ignored. A managed `HNVM_DISTRUST_CERTS_EXTRA` is always removed, and any fingerprints set in the
environment or `.hnvmrc` are removed on top of it. This only applies on macOS. To check what a Mac has, run
`defaults read "/Library/Managed Preferences/com.compass.hnvm"`.

### `HNVM_NOFALLBACK` (Defaults to `false`)

When set to true, HNVM will stop going up the tree to look for `.hnvmrc` files. This is useful in
monorepo's if you want to enforce that your specific package's HNVM version is run and it doesn't
end up falling back to a version outside of the repo.

```sh
# In package's .hnvmrc
HNVM_NODE=11.0.0

# In git root's .hnvmrc
HNVM_NOFALLBACK=true

# In HNVM default .hnvmrc
HNVM_NODE=12.0.0
```

If I run `node --version` next to the package, I'll get `v11.0.0`. If I run it outside the package
but still in the repo, I'll get an error asking to specify a node version instead of defaulting to
node v12.0.0.

## Why Not Just Use [NVM](https://github.com/nvm-sh/nvm)

Because `nvm` doesn't support [`fish`](https://fish.sh) 😅. This project also focuses on individual
projects or packages declaring versions of node to use, as opposed to switching globally between
different versions. It results in a simpler setup with far less code.

## Development

HNVM is written in Go. `make test` runs the unit tests (offline), `make lint` runs `go vet`, `gofmt`
and `shellcheck`, and `make build` builds `dist/hnvm`.

### Integration Tests

NOTE: The tests hit live npm registry servers and go through the process of downloading multiple copies of node. Do not run in bandwidth-constrained environments!

Tests for HNVM are written with [jest](https://jestjs.io/) and require node 14.4.0 or higher (protip: use hnvm to test hnvm 😉). To run the tests, `cd test/ && npm install && npm test`.
After `make build` they run against the Go binary in `dist/hnvm`; without it, against the bash
implementation that remains in `lib/hnvm/` until the tests are ported.

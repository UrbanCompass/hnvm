# HNVM: Hermetic Node Version Manager

Hermetically sealed versions of [node](https://npmjs.org) and [pnpm](https://pnpm.js.org).

Instead of relying on the version of `node`, `npm`, `pnpm`, etc. each individual developer or
machine has installed locally, packages use a system we've named HNVM, which stands for Hermetic
Node Version Manager. Having our `node` binaries be hermetic means each app or package defines what
version of `node` they depend on. That version is installed and used to run whenever the package
runs scripts in `node`, `npm`, `pnpm`, and `yarn`. This ensures that everyone's systems output
consistent packages, and upgrades required by one package don't affect another.

## Installation

### [Homebrew](https://brew.sh)

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

This will install both `hnvm` and `jq`.

### Manual install script

If you can't or don't want to install using `homebrew` or `basher`, you can use the provided
`install.sh` and `uninstall.sh` scripts to add or remove the local bin path to your global $PATH:

```sh
source install.sh
node -v # Uses hnvm node script to get the node version

source uninstall.sh
node -v # No longer uses hnvm node script
```

Note that `hnvm` depends on [`jq`](https://stedolan.github.io/jq/) being available in your global
`$PATH`, and if you don't use `brew` you'll have to make sure that `jq` is installed on your own.

## Usage

HNVM reads the version of `node`/`pnpm`/`yarn` set in your `package.json` file' `"engines"` field. If
no version is set, it will default to the current versions set in HNVM's own `package.json`. Unlike
HNVM 1.0, you don't have to find any particular bash script to run HNVM. Just use the regular
`node`, `npm`, etc. commands you're used to from anywhere on your computer. If you run it
next to a `package.json` file, it will read the engines field. If not, it'll default to the global
version.

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
1. The `package.json` file in the current process working directory (`$PWD/package.json`)
2. An `.hnvmrc` file in the `PWD` (`$PWD/.hnvmrc`)
3. An `.hnvmrc` file at the root of your git repo (if running in a git repo)
4. An `.hnvmrc` file in your home directory (`~/.hnvmrc`)
5. The `.hnvmrc` file that ships with the current version of HNVM

The `.hnvmrc` files are all simple key/value pairs prepended with `HNVM_`:
```sh
HNVM_PATH=/path/to/.hnvm
HNVM_NODE=10.0.0
HNVM_PNPM=3.0.0
HNVM_YARN=1.19.0
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
    "yarn": "1.19.0"
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

### `HNVM_NODE`, `HNVM_PNPM`, `HNVM_YARN` (Defaults to `latest`)

Version of `node`/`pnpm`/`yarn` to use. If semver ranges are provided instead of exact versions
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

### `HNVM_NODE_DIST` (Defaults to `https://nodejs.org/dist`)

Location of the NodeJS distribution files. If you provide a custom destination, you should ensure
that the repository layout mirrors that of nodejs.org:
```
/node-v${node_ver}-${platform}-${cpu_arch}.tar.gz
```

### `HNVM_PNPM_REGISTRY` (Defaults to `https://registry.npmjs.org`)

If using the `--with-pnpm` flag, `npm`-compatible registry to install `pnpm` from. Defaults to the
public `npm` registry.

### `HNVM_YARN_DIST` (Defaults to `https://yarnpkg.com/downloads`)

If using the `--with-yarn` flag, the location from where to download `yarn` from. When providing a
custom destination, you should ensure the layout mirrors that of `yarnpkg.com`:
```
/${yarn_ver}/yarn-v${yarn_ver}.tar.gz
```


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

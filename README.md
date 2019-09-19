# HNVM: Hermetic Node Version Manager

Hermetically sealed versions of [node](https://npmjs.org) and [pnpm](https://pnpm.js.org).

Instead of relying on the version of `node`, `npm`, and `pnpm` each individual developer or machine
has installed locally, packages use a system we've named `hnvm`, which stands for Hermetic Node
Version Manager. Having our `node` binaries be hermetic means each app or package defines what
version of `node` they depend on. That version is installed and used to run whenever the package
runs scripts in `node`, `npm`, and `pnpm`. This ensures that everyone's systems output consistent
packages, and upgrades required by one package don't affect another.

## Installation

HNVM is distributed via [Homebrew](https://brew.sh) inside the
[`UrbanCompass/versions` tap](https://github.com/UrbanCompass/homebrew-versions):

```sh
brew tap UrbanCompass/versions

# If you get a permissions error tap'ing, try using the repo's ssh url
brew tap UrbanCompass/versions git@github.com:UrbanCompass/homebrew-versions.git

brew install hnvm
```

## Usage

HNVM reads the version of `node` and `pnpm` set in your `package.json` file' `"engines"` field. If
no version is set, it will default to the current versions set in HNVM's own `package.json`. Unlike
HNVM 1.0, you don't have to find any particular bash script to run HNVM. Just use the regular
`node`, `npm`, and `pnpm` commands you're used to from anywhere on your computer. If you run it
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

### Download Path

By default, `hnvm` will download node and `pnpm` binaries to `~/.hnvm`. To customize this path, simply
set the `HNVM_PATH` environment variable, either by exporting it or setting it in your profile:

```sh
env HNVM_PATH=/path/to/.hnvm
```

### Node and PNPM Versions

HNVM checks for what versions of `node` and `pnpm` to use in one of 3 places, in order:

1. It looks for a `package.json` file in the same directory from which the script was invoked. If
that file has an `"engines"` field, it'll read the `node` version from either `engines.hnvm` or
`engines.node`, and the `pnpm` version from `engines.pnpm`.
2. It checks for an `HNVM_NODE_VER` and `HNVM_PNPM_VER` environment variable.
3. It falls back to the defaults declared in the `.env` file baked into `hnvm` itself

If semver ranges are provided instead of exact versions, `hnvm` will perform curl requests to
resolve those to an exact version. However you'll get a warning about this since it could slow down
execution time from the async request, or it might even fail to work at all if the curl requests
fail to load.

To try to mitigate this slowdown, semver range results are cached locally. The default is 60s but
you can control this by setting the `HNVM_RANGE_CACHE` environment variable.

### Silencing output

By default, `hnvm` will output information about the current version of node running, and any
download statuses if a new version is being downloaded so that you know exactly what's going on. If
you don't want this output, simply set the `HNVM_SILENCE_OUTPUT` environment variable to true to
have this output redirected to `/dev/null`.

## Why Not Just Use [NVM](https://github.com/nvm-sh/nvm)

Because `nvm` doesn't support [`fish`](https://fish.sh) 😅. Additionally, it's gotten a bit too
bloated by trying to do too many things. HNVM is focused on using and running node at a specific
version if declared, not in having multiple environments globally.

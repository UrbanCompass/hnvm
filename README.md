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

By default, `hnvm` will download node and pnpm binaries to `~/.hnvm`. To customize this path, simply
set the `HNVM_PATH` environment variable, either by exporting it or setting it in your profile:


```sh
env HNVM_PATH=/path/to/.hnvm
```

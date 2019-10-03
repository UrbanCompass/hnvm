#!/usr/bin/env bash

set -e

source="${BASH_SOURCE[0]}"
while [ -h "$source" ]; do # resolve $source until the file is no longer a symlink
  script_dir="$( cd -P "$( dirname "$source" )" >/dev/null 2>&1 && pwd )"
  source="$(readlink "$source")"
  [[ $source != /* ]] && source="$script_dir/$source"
done
script_dir="$( cd -P "$( dirname "$source" )" >/dev/null 2>&1 && pwd )"

source "$script_dir/colors.sh"
source "$script_dir/config.sh"

node_path="$HNVM_PATH/node/$node_ver"
node_bin="$node_path/bin/node"
npm_bin="$node_path/bin/npm"
npx_bin="$node_path/bin/npx"

pnpm_path="$HNVM_PATH/pnpm"
pnpm_bin="$pnpm_path/lib/bin/pnpm.js"
pnpx_bin="$pnpm_path/lib/bin/pnpx.js"

yarn_path="$HNVM_PATH/yarn"
yarn_bin="$yarn_path/bin/yarn.js"

function download_node() {
  platform=
  if [[ "$OSTYPE" == "linux-gnu" ]]; then
    platform="linux"
  elif [[ "$OSTYPE" == "darwin"* ]]; then
    platform="darwin"
  else
    red "OS Platform not supported"
    exit 1
  fi

  rm -rf "${node_path}"
  mkdir -p "${node_path}"

  blue "Downloading node v$node_ver to $node_path" > $COMMAND_OUTPUT

  if [[ "$HNVM_QUIET" == "true" ]]; then
    curl https://nodejs.org/dist/v${node_ver}/node-v${node_ver}-${platform}-x64.tar.gz --silent |
      tar xz -C ${node_path} --strip-components=1 > $COMMAND_OUTPUT
  else
    curl https://nodejs.org/dist/v${node_ver}/node-v${node_ver}-${platform}-x64.tar.gz |
      tar xz -C ${node_path} --strip-components=1 > $COMMAND_OUTPUT
  fi
}

function download_pnpm() {
  rm -rf "${pnpm_path}"
  mkdir -p "${pnpm_path}"

  blue "Downloading pnpm v$pnpm_ver to $pnpm_path" > $COMMAND_OUTPUT

  if [[ "$HNVM_QUIET" == "true" ]]; then
    curl -L https://unpkg.com/@pnpm/self-installer --silent |
      PNPM_VERSION=$pnpm_ver PNPM_DEST=$pnpm_path ${node_bin} > $COMMAND_OUTPUT
  else
    curl -L https://unpkg.com/@pnpm/self-installer |
      PNPM_VERSION=$pnpm_ver PNPM_DEST=$pnpm_path ${node_bin} > $COMMAND_OUTPUT
  fi
}

function download_yarn() {
  rm -rf "${yarn_path}"
  mkdir -p "${yarn_path}"

  blue "Downloading yarn v$yarn_ver to $yarn_path" > $COMMAND_OUTPUT

  if [[ "$HNVM_QUIET" == "true" ]]; then
    curl -L https://yarnpkg.com/downloads/$yarn_ver/yarn-v$yarn_ver.tar.gz --silent |
      tar xz -C ${yarn_path} --strip-components=1 > $COMMAND_OUTPUT
  else
    curl -L https://yarnpkg.com/downloads/$yarn_ver/yarn-v$yarn_ver.tar.gz |
      tar xz -C ${yarn_path} --strip-components=1 > $COMMAND_OUTPUT
  fi
}

if [[ ! -x "$node_bin" ]]; then
  download_node
fi

if [[ "$0" == *pnpm || "$0" == *pnpx ]] && [[ "$(${node_bin} ${pnpm_bin} -v)" != $pnpm_ver ]]; then
  download_pnpm
fi

if [[ "$0" == *yarn ]] && [[ "$(${node_bin} ${yarn_bin} -v)" != $yarn_ver ]]; then
  download_yarn
fi

blue "Using Hermetic NodeJS v$node_ver" > $COMMAND_OUTPUT

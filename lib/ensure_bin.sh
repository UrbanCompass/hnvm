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

node_path="$HNVM_PATH/$node_ver"
node_bin="$node_path/bin/node"
npm_bin="$node_path/bin/npm"
npx_bin="$node_path/bin/npx"
pnpm_bin="$node_path/lib/node_modules/pnpm/bin/pnpm.js"
pnpx_bin="$node_path/lib/node_modules/pnpm/bin/pnpx.js"

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

  blue "Downloading Node v$node_ver"> $COMMAND_OUTPUT

  if [[ "$HNVM_QUIET" == "true" ]]; then
    curl https://nodejs.org/dist/v${node_ver}/node-v${node_ver}-${platform}-x64.tar.gz --silent |
      tar xz -C ${node_path} --strip-components=1 > $COMMAND_OUTPUT
  else
    curl https://nodejs.org/dist/v${node_ver}/node-v${node_ver}-${platform}-x64.tar.gz |
      tar xz -C ${node_path} --strip-components=1 > $COMMAND_OUTPUT
  fi
}

function download_pnpm() {
  blue "Downloading Pnpm v$pnpm_ver" > $COMMAND_OUTPUT
  if [[ "$HNVM_QUIET" == "true" ]]; then
    curl -L https://unpkg.com/@pnpm/self-installer --silent |
      PNPM_VERSION=$pnpm_ver ${node_bin} > $COMMAND_OUTPUT
  else
    curl -L https://unpkg.com/@pnpm/self-installer |
      PNPM_VERSION=$pnpm_ver ${node_bin} > $COMMAND_OUTPUT
  fi
}

if [[ ! -x "$node_bin" ]]; then
  download_node
fi

if [[ "$0" == *pnpm || "$0" == *pnpx ]] && [[ ! -x "$pnpm_bin" || "$(${node_bin} ${pnpm_bin} -v)" != $pnpm_ver ]]; then
  download_pnpm
fi

blue "Using hermetic NodeJS v$node_ver" > $COMMAND_OUTPUT

#!/usr/bin/env bash

set -e

source="${BASH_SOURCE[0]}"
while [ -h "$source" ]; do # resolve $source until the file is no longer a symlink
  script_dir="$( cd -P "$( dirname "$source" )" >/dev/null 2>&1 && pwd )"
  source="$(readlink "$source")"
  [[ $source != /* ]] && source="$script_dir/$source"
done
script_dir="$( cd -P "$( dirname "$source" )" >/dev/null 2>&1 && pwd )"

export $(egrep -v '^#' $script_dir/../.env | xargs)
export HNVM_PATH="${HNVM_PATH:-$HOME/.hnvm}"

source "$script_dir/versions.sh"

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

  curl https://nodejs.org/dist/v${node_ver}/node-v${node_ver}-${platform}-x64.tar.gz | tar xz -C ${node_path} --strip-components=1
}

function download_pnpm() {
  curl -L https://unpkg.com/@pnpm/self-installer | PNPM_VERSION=$pnpm_ver ${node_bin}
}

if [[ ! -x "$node_bin" ]]; then
  download_node
fi

if [[ ! -x "$pnpm_bin" || "$(${pnpm_bin} -v)" != $pnpm_ver ]]; then
  download_pnpm
fi

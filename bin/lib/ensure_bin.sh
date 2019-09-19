#!/usr/bin/env bash

set -e

source="${BASH_SOURCE[0]}"
while [ -h "$source" ]; do # resolve $source until the file is no longer a symlink
  script_dir="$( cd -P "$( dirname "$source" )" >/dev/null 2>&1 && pwd )"
  source="$(readlink "$source")"
  [[ $source != /* ]] && source="$script_dir/$source"
done
script_dir="$( cd -P "$( dirname "$source" )" >/dev/null 2>&1 && pwd )"

source "$script_dir/versions.sh"

HNVM_PATH="${HNVM_PATH:-$script_dir/../../.hnvm}"

node_path="$HNVM_PATH/$node_ver"
node_bin="$node_path/bin/node"
npm_bin="$node_path/bin/npm"
npx_bin="$node_path/bin/npx"
pnpm_bin="$node_path/lib/node_modules/pnpm/bin/pnpm.js"
pnpx_bin="$node_path/lib/node_modules/pnpm/bin/pnpx.js"

# Replaces all instances of "#!/usr/bin/env node" with "#!/usr/bin/env /path/to/hermetic/node" for
# the given path. Non-recursive.
# Note: this will break if given a binary file in which `sed` cannot operate with.
function ensure_hermetic_hashbang() {
  hashbang_pattern='^#!/usr/bin/env node$'
  hermetic_hashbang="#!/usr/bin/env ${node_bin}"

  if [ -d "$1" ]; then
    for i in "$1"/*; do
      if [ ! -d "$i" ]; then
        # Use `< $i 1<> $i` instead of `-i` to preserve symlinks
        # https://www.cyberciti.biz/faq/howto-prevent-sed-i-from-destroying-symlinks-on-linux-unix/
        sed 's,'"$hashbang_pattern"','"$hermetic_hashbang"',' < $i 1<> $i
      fi
    done
  else
    # Use `< $1 1<> $1` instead of `-i` to preserve symlinks
    # https://www.cyberciti.biz/faq/howto-prevent-sed-i-from-destroying-symlinks-on-linux-unix/
    sed 's,'"$hashbang_pattern"','"$hermetic_hashbang"',' < $1 1<> $1
  fi
}

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
  ensure_hermetic_hashbang "$node_path/bin/npm"
  ensure_hermetic_hashbang "$node_path/bin/npx"
}

function download_pnpm() {
  curl -L https://unpkg.com/@pnpm/self-installer | PNPM_VERSION=$pnpm_ver ${node_bin}
  ensure_hermetic_hashbang "$node_path/lib/node_modules/pnpm/bin/"
}

if ! [ -x "$node_bin" ]; then
  download_node
fi

if ! [ -x "$pnpm_bin" ]; then
  download_pnpm
fi

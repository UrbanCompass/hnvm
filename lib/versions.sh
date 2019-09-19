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
HNVM_RANGE_CACHE=${HNVM_RANGE_CACHE:-$DEFAULT_HNVM_RANGE_CACHE}

jq_bin="$script_dir/jq/jq"
pkg_json="$PWD/package.json"
echo "package.json $pkg_json"

# 1. Try version from package.json engines field
if [[ -f "$pkg_json" ]]; then
  if [[ -z "$node_ver" && -f "$pkg_json" ]]; then
    node_ver="$(cat $pkg_json | $jq_bin -r '.engines.hnvm')"
  fi

  if [[ "$node_ver" == "null" ]]; then
    node_ver="$(cat $pkg_json | $jq_bin -r '.engines.node')"
  fi

  pnpm_ver="$(cat $pkg_json | $jq_bin -r '.engines.pnpm')"
fi

# 2. Try version from env var
if [[ -z "$node_ver" || "$node_ver" == "null" ]]; then
  node_ver=$HNVM_NODE_VER;
fi

if [[ -z "$pnpm_ver" || "$pnpm_ver" == "null" ]]; then
  pnpm_ver=$HNVM_PNPM_VER;
fi

# 3. Fall back to default version
if [[ -z "$node_ver" || "$node_ver" == "null" ]]; then
  node_ver=$DEFAULT_HNVM_NODE_VER;
fi

if [[ -z "$pnpm_ver" || "$pnpm_ver" == "null" ]]; then
  pnpm_ver=$DEFAULT_HNVM_PNPM_VER;
fi

# Resolve an exact node version if a range was given
if [[ ! "$node_ver" =~ [0-9]+\.[0-9]+\.[0-9]+ ]]; then
  cache_file="$script_dir/../../.tmp/node_ver/$node_ver"
  mkdir -p "$(dirname $cache_file)"

  # Cache result for 60s
  if [ -f $cache_file ] && [ "$(( $(date +"%s") - $HNVM_RANGE_CACHE ))" -le "$(date -r $cache_file +"%s")" ]; then
    node_ver="$(cat $cache_file)"
  else
    echo -e $'\e[33mWarning\e[0m: Resolving node version range "'"$node_ver"'" is slower than providing an exact version.'

    node_ver="$(curl https://semver.io/node/resolve/$node_ver --silent)"
    echo $node_ver > $cache_file
  fi
fi

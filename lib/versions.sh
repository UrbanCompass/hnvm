#!/usr/bin/env bash

set -e

source="${BASH_SOURCE[0]}"
while [ -h "$source" ]; do # resolve $source until the file is no longer a symlink
  script_dir="$( cd -P "$( dirname "$source" )" >/dev/null 2>&1 && pwd )"
  source="$(readlink "$source")"
  [[ $source != /* ]] && source="$script_dir/$source"
done
script_dir="$( cd -P "$( dirname "$source" )" >/dev/null 2>&1 && pwd )"

jq_bin="$script_dir/jq/jq"

pkg_json="$PWD/package.json"
if [[ ! -f "$pkg_json" ]]; then # Default to hnvm-global package.json for versions
  pkg_json="$script_dir/../package.json"
fi

node_ver="$(cat $pkg_json | $jq_bin -r '.engines.hnvm')"
if [[ "$node_ver" == "null" ]]; then
  node_ver="$(cat $pkg_json | $jq_bin -r '.engines.node')"
fi

# Resolve an exact version
if [[ ! "$node_ver" =~ [0-9]+\.[0-9]+\.[0-9]+ ]]; then
  cache_file="$script_dir/../../.tmp/node_ver/$node_ver"
  mkdir -p "$(dirname $cache_file)"

  # Cache result for 60s
  if [ -f $cache_file ] && [ "$(( $(date +"%s") - 60 ))" -le "$(date -r $cache_file +"%s")" ]; then
    node_ver="$(cat $cache_file)"
  else
    echo -e $'\e[33mWarning\e[0m: Resolving node version range "'"$node_ver"'" is slower than providing an exact version.'

    node_ver="$(curl https://semver.io/node/resolve/$node_ver --silent)"
    echo $node_ver > $cache_file
  fi
fi

pnpm_ver="$(cat $pkg_json | $jq_bin -r '.engines.pnpm')"

#!/usr/bin/env bash

set -e

# The exposed bin files are symlinked into the user's $PATH, so we need to make our way back to
# the original path location to understand where the rest of this package's files are installed
source="${BASH_SOURCE[0]}"
while [ -h "$source" ]; do # resolve $source until the file is no longer a symlink
  script_dir="$( cd -P "$( dirname "$source" )" >/dev/null 2>&1 && pwd )"
  source="$(readlink "$source")"
  [[ $source != /* ]] && source="$script_dir/$source"
done
script_dir="$( cd -P "$( dirname "$source" )" >/dev/null 2>&1 && pwd )"

# Read and export rcfile variables from configured directories
rc_dirs="$script_dir/..;$HOME;.git;$PWD"
IFS=';' read -ra dirs_array <<< "$rc_dirs"
for i in "${dirs_array[@]}"; do
  rc_file="$i/.hnvmrc"

  if [ $i == '.git' ]; then
    git_root="$(if [ "`which git && git rev-parse --show-cdup`" != "" ]; then cd `git rev-parse --show-cdup`; pwd; fi;)"

    if [[ ! -z "$git_root" && -f "$git_root/.hnvmrc" ]]; then
      rc_file="$git_root/.hnvmrc"
    fi
  fi

  if [ -f "$rc_file" ]; then
    export $(egrep -v '^#' $rc_file | sed 's#~#'$HOME'#g' | xargs)
  fi
done

export COMMAND_OUTPUT=/dev/stdout
if [ "$HNVM_QUIET" == "true" ]; then
  COMMAND_OUTPUT=/dev/null
fi

jq_bin="$script_dir/jq/jq"
pkg_json="$PWD/package.json"

# Try version from package.json engines field
if [[ -f "$pkg_json" ]]; then
  if [[ -z "$node_ver" && -f "$pkg_json" ]]; then
    node_ver="$(cat $pkg_json | $jq_bin -r '.engines.hnvm')"
  fi

  if [[ "$node_ver" == "null" ]]; then
    node_ver="$(cat $pkg_json | $jq_bin -r '.engines.node')"
  fi

  pnpm_ver="$(cat $pkg_json | $jq_bin -r '.engines.pnpm')"
fi

# Fall back to env var
if [[ -z "$node_ver" || "$node_ver" == "null" ]]; then
  node_ver=$HNVM_NODE;
fi

if [[ -z "$pnpm_ver" || "$pnpm_ver" == "null" ]]; then
  pnpm_ver=$HNVM_PNPM;
fi

# Resolve an exact node version if a range was given
if [[ ! "$node_ver" =~ [0-9]+\.[0-9]+\.[0-9]+ ]]; then
  cache_file="$script_dir/../../.tmp/node_ver/$node_ver"
  mkdir -p "$(dirname $cache_file)"

  # Cache result for 60s
  if [ -f $cache_file ] && [ "$(( $(date +"%s") - $HNVM_RANGE_CACHE ))" -le "$(date -r $cache_file +"%s")" ]; then
    node_ver="$(cat $cache_file)"
  else
    echo -e $'\e[33mWarning\e[0m: Resolving node version range "'"$node_ver"'" is slower than providing an exact version.' > $COMMAND_OUTPUT

    node_ver="$(curl https://semver.io/node/resolve/$node_ver --silent)"
    echo $node_ver > $cache_file
  fi
fi

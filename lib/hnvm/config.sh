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

export COMMAND_OUTPUT=/dev/stdout

# Read env vars set in profile or at runtime
export node_ver="${HNVM_NODE}"
export pnpm_ver="${HNVM_PNPM}"
export yarn_ver="${HNVM_YARN}"

# Read and export rcfile variables from configured directories
exports=''
rc_dirs="${PWD};.git;${HOME};${script_dir}/.."
IFS=';' read -ra dirs_array <<< "$rc_dirs"
for i in "${dirs_array[@]}"; do
  rc_file="$i/.hnvmrc"

  if [[ $i == '.git' && -n "$(type -p git)" ]]; then
    git_root="$(if [ "$(git rev-parse --show-cdup 2> /dev/null)" != "" ]; then cd "$(git rev-parse --show-cdup)"; pwd; fi;)"

    if [[ -n "${git_root}" && -f "${git_root}/.hnvmrc" && -n "$(cat "${git_root}"/.hnvmrc)" ]]; then
      rc_file="${git_root}/.hnvmrc"
    fi
  fi

  if [[ -f "${rc_file}" && "${HNVM_NOFALLBACK}" != "true" ]]; then
    exports="$(cat "${rc_file}") ${exports}"
    # shellcheck disable=SC2046
    export $(echo "${exports}" | grep -E -v '^#'| sed 's#~#'"${HOME}"'#g' | xargs)
  fi
done

if [ "$HNVM_QUIET" == "true" ]; then
  COMMAND_OUTPUT=/dev/null
fi

# Try version from package.json engines field
pkg_json="${PWD}/package.json"

if [[ -f "${pkg_json}" ]]; then
  pkg_json_contents="$(cat "${pkg_json}")"

   echo "$pkg_json_contents" | jq '.' > /dev/null || {
    red "An error occurred while parsing package.json"
    exit 1
  }

  if [[ -z "${node_ver}" ]]; then
    node_ver="$(echo "${pkg_json_contents}" | jq -r '.hnvm.node')"

    if [[ "${node_ver}" == "null" ]]; then
      node_ver="$(echo "${pkg_json_contents}" | jq -r '.engines.hnvm')"
    fi

    if [[ "${node_ver}" == "null" ]]; then
      node_ver="$(echo "${pkg_json_contents}" | jq -r '.engines.node')"
    fi
  fi

  if [[ -z "${pnpm_ver}" ]]; then
    pnpm_ver="$(echo "${pkg_json_contents}" | jq -r '.hnvm.pnpm')"

    if [[ "${pnpm_ver}" == "null" ]]; then
      pnpm_ver="$(echo "${pkg_json_contents}" | jq -r '.engines.pnpm')"
    fi
  fi

  if [[ -z "${yarn_ver}" ]]; then
    yarn_ver="$(echo "${pkg_json_contents}" | jq -r '.hnvm.yarn')"

    if [[ "${yarn_ver}" == "null" ]]; then
      yarn_ver="$(echo "${pkg_json_contents}" | jq -r '.engines.yarn')"
    fi
  fi
fi

# Fall back to env var
if [[ -z "${node_ver}" || "${node_ver}" == "null" ]]; then
  node_ver=${HNVM_NODE};
fi

if [[ -z "${pnpm_ver}" || "${pnpm_ver}" == "null" ]]; then
  pnpm_ver=${HNVM_PNPM};
fi

if [[ -z "${yarn_ver}" || "${yarn_ver}" == "null" ]]; then
  yarn_ver=${HNVM_YARN};
fi

# No fallback version(s) could be determined, error out for those missing
if [[ -z "${pnpm_ver}" && ("${0}" == *pnpm || "${0}" == *pnpx) ]]; then
  red "No HNVM_PNPM version set. Please set a pnpm version."
  exit 1
fi

if [[ -z "${yarn_ver}" && "${0}" == *yarn ]]; then
  red "No HNVM_YARN version set. Please set a yarn version."
  exit 1
fi

if [[ -z "${node_ver}" ]]; then
  red "No HNVM_NODE version set. Please set a Node version."
  exit 1
fi

# Resolve an exact version when a semver range is given. Queries results from the npm registry.
#
# $1: Name of the package in the npm registry
# $2: Semver range
#
# Outputs results to a variable named "resolve_ver_result".
function resolve_ver() {
  local name=${1}
  local ver=${2}
  local cache_file="${script_dir}/../.tmp/${name}/${ver}"

  if [[ ! "$ver" =~ [0-9]+\.[0-9]+\.[0-9]+ ]]; then
    mkdir -p "$(dirname "${cache_file}")"

    # Cache result
    if [ -f "${cache_file}" ] && [ "$(( $(date +"%s") - HNVM_RANGE_CACHE ))" -le "$(date -r "${cache_file}" +"%s")" ]; then
      ver="$(cat "${cache_file}")"
    else
      echo -e $'\e[33mWarning\e[0m: Resolving '"${name}"' version range "'"${ver}"'" is slower than providing an exact version.' > ${COMMAND_OUTPUT}

      ver="$(curl "http://registry.npmjs.org/${name}/${ver}" --silent | jq -r '.version')" > ${COMMAND_OUTPUT}
      echo "${ver}" > "${cache_file}"
    fi
  fi

  resolve_ver_result=${ver}
}

resolve_ver "node" "${node_ver}"
node_ver=${resolve_ver_result}

if [[ "${0}" == *pnpm || "${0}" == *pnpx ]]; then
  resolve_ver "pnpm" "${pnpm_ver}"
  pnpm_ver=${resolve_ver_result}
fi

if [[ "${0}" == *yarn ]]; then
  resolve_ver "yarn" "${yarn_ver}"
  yarn_ver=${resolve_ver_result}
fi

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

COMMAND_OUTPUT=""

# Context for why we try multiple output redirect targets:
# https://github.com/UrbanCompass/hnvm/pull/55#discussion_r1583426262
if [[ -n "$HNVM_OUTPUT_DESTINATION" && ! -S "$HNVM_OUTPUT_DESTINATION" ]]; then
  COMMAND_OUTPUT="$HNVM_OUTPUT_DESTINATION"
elif [[ -e "/dev/stdout" && -w "/dev/stdout" && ! -S "/dev/stdout" ]]; then
  COMMAND_OUTPUT="/dev/stdout"
elif [[ -e "/dev/fd/1" && -w "/dev/fd/1" && ! -S "/dev/stdout" ]]; then
  COMMAND_OUTPUT="/dev/fd/1"
else
  # If COMMAND_OUTPUT is still not assigned by here, fall back to posix-standard /dev/null
  #
  # Very important: any debug warnings should ONLY go to stderr.
  # If these echoes go to stdout, you get issues like this:
  # https://compass-tech.atlassian.net/jira/servicedesk/projects/TIP/queues/custom/268/TIP-8901
  echo "WARNING: Could not find a writable, non-socket stdout redirect target!" >&2
  echo "WARNING: Further HNVM output will be redirected to '/dev/null'" >&2
  COMMAND_OUTPUT="/dev/null"
fi

export COMMAND_OUTPUT

# Set these defaults here instead of the rc file so that HNVM_NOFALLBACK never blocks these defaults
export HNVM_PATH=${HNVM_PATH:-$HOME/.hnvm}
export HNVM_RANGE_CACHE=${HNVM_RANGE_CACHE:-60}
export HNVM_QUIET=${HNVM_QUIET:-false}
export HNVM_NODE_DIST=${HNVM_NODE_DIST:-'https://nodejs.org/dist'}
export HNVM_PNPM_REGISTRY=${HNVM_PNPM_REGISTRY:-'https://registry.npmjs.org'}
export HNVM_YARN_DIST=${HNVM_YARN_DIST:-'https://yarnpkg.com/downloads'}

# Read env vars set in profile or at runtime
export node_ver="${HNVM_NODE}"
export pnpm_ver="${HNVM_PNPM}"
export yarn_ver="${HNVM_YARN}"

available_node_bin=

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
    # The rc file and previous exports should ALWAYS be separated by a newline
    # otherwise, the grep to weed out comment lines will fail if an hnvmrc starts with an L1 comment
    newline=$'\n'
    exports="$(cat "${rc_file}")${newline}${exports}"
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
  error "No HNVM_PNPM version set. Please set a pnpm version."
  exit 1
fi

if [[ -z "${yarn_ver}" && "${0}" == *yarn ]]; then
  error "No HNVM_YARN version set. Please set a yarn version."
  exit 1
fi

if [[ -z "${node_ver}" ]]; then
  error "No HNVM_NODE version set. Please set a Node version."
  exit 1
fi

function is_invalid_version() {
  [[ ! "$1" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]
}

# Finds _any_ locally available copy of node and sets `available_node_bin` to its path.
function find_local_node() {
  local available_node_ver=
  available_node_ver="$(find "$HNVM_PATH/node/"* | head -n 1)"
  if [ -z "$available_node_ver" ]; then
    red "No local copy of node available. Please use hnvm at least once on a specific version before attempting semver ranges."
    exit 1
  fi

  available_node_bin="${available_node_ver}/bin/node"
}

# Resolve an exact version when a semver range is given. Queries results from the npm registry.
#
# $1: Name of the package in the npm registry
# $2: Semver range
#
# Outputs results to a variable named "resolve_ver_result".
function resolve_ver() {
  local name=${1}
  local ver=${2}
  local initial_ver=${2}
  local ver_sanitized
  local cache_file
  ver_sanitized="${ver//[^a-z0-9_]/_}"
  cache_file="${HNVM_PATH}/.tmp/${name}/${ver_sanitized}"

  # Only spend time resolving the version if it's not a fully resolved version number.
  if is_invalid_version "$ver"; then
    mkdir -p "$(dirname "${cache_file}")"

    # Check if the previous cache result exists and has been modified within the past HNVM_RANGE_CACHE seconds.
    if [ -f "${cache_file}" ] && [ "$(( $(date +"%s") - HNVM_RANGE_CACHE ))" -le "$(date -r "${cache_file}" +"%s")" ]; then
      ver="$(cat "${cache_file}")"
    else
      echo -e $'\e[33mWarning\e[0m: Resolving '"${name}"' "'"${ver}"'" is slower than providing an exact version.'  >> "${COMMAND_OUTPUT}"

      # Try to resolve a version tag directly from the registry first, but gracefully fail if it's malformed.
      ver="$(curl "https://registry.npmjs.org/${name}/${ver}" --silent | jq -r '.version' || echo 'INVALID')"  >> "${COMMAND_OUTPUT}"
      if is_invalid_version "$ver"; then
        find_local_node

        # First try to find a local matching version.
        available_versions="$(find "$HNVM_PATH/$name/"* -maxdepth 0 -type d -print0 | xargs -0 basename | tr '\n' ':')"
        matching_versions_input=$(cat <<EOF
{
  "desiredVersionRange": "${initial_ver}",
  "availableVersionsColonDelimited": "${available_versions}"
}
EOF
        )

        set +e
        ver=$(exec "${available_node_bin}" "${script_dir}/find-matching-version.js" <<< "${matching_versions_input}" 2>/dev/null)
        set -e

        # If we didn't have a local copy, fetch the list of versions in existence and use the latest in the range.
        if is_invalid_version "$ver"; then
          echo -e $'\e[33mWarning\e[0m: "'"${initial_ver}"'" is not satisfied by any local version and must be resolved asynchronously.'  >> "${COMMAND_OUTPUT}"
          npm_package_info="$(curl "https://registry.npmjs.org/${name}" --silent)" > /dev/null
          matching_versions_input=$(cat <<EOF
{
  "desiredVersionRange": "${initial_ver}",
  "npmPackageInfo": ${npm_package_info}
}
EOF
          )

          ver=$(exec "${available_node_bin}" "${script_dir}/find-matching-version.js" <<< "${matching_versions_input}")
        fi
      fi

      if is_invalid_version "$ver"; then
        red """Failed to resolve ""${initial_ver}"" to a valid version, ""${ver}"" is not valid."""
      fi

      # Cache the resolved version for future runs.
      echo "${ver}" > "${cache_file}"
    fi
  fi

  blue """Resolved $name ""${initial_ver}"" to ${ver}""" >> "${COMMAND_OUTPUT}"
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

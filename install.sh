#!/usr/bin/env bash
#
# Adds the bin scripts for hnvm to your $PATH when you `source` it
#
# Usage:
#   source install.sh
#   node -v # Uses hnvm node script

set -e

source="${BASH_SOURCE[0]}"
while [ -h "$source" ]; do # resolve $source until the file is no longer a symlink
  script_dir="$( cd -P "$( dirname "${source}" )" >/dev/null 2>&1 && pwd )"
  source="$(readlink "${source}")"
  [[ ${source} != /* ]] && source="${script_dir}/${source}"
done
script_dir="$( cd -P "$( dirname "${source}" )" >/dev/null 2>&1 && pwd )"

bin_path="${script_dir}/bin"

# Checks to see if we've already added the bin path to $PATH
not_in_path() {
  [[ ":${PATH}:" != *":${bin_path}:"* ]]
}

if not_in_path; then
  export PATH="${bin_path}:${PATH}"

  # shellcheck source=/dev/null
  source "${script_dir}/lib/colors.sh"
  green "Added local hnvm to PATH:"
  echo "${PATH}"
fi

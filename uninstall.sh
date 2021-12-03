#!/usr/bin/env bash
#
# Removes the local bin scripts for hnvm from your $PATH when you `source` it
#
# Usage:
#   source uninstall.sh
#   node -v # No longer uses hnvm node script

set -e

source="${BASH_SOURCE[0]}"
while [ -h "${source}" ]; do # resolve $source until the file is no longer a symlink
  script_dir="$( cd -P "$( dirname "${source}" )" >/dev/null 2>&1 && pwd )"
  source="$(readlink "${source}")"
  [[ ${source} != /* ]] && source="${script_dir}/${source}"
done
script_dir="$( cd -P "$( dirname "${source}" )" >/dev/null 2>&1 && pwd )"

bin_path="${script_dir}/bin"

# Checks to see if we've already added the bin path to $PATH
is_in_path() {
  [[ ":${PATH}:" == *":${bin_path}:"* ]]
}

if is_in_path; then
  PATH=:${PATH}:
  PATH=${PATH//:$bin_path:/:}
  PATH=${PATH#:}; PATH=${PATH%:}

  # shellcheck source=/dev/null
  source "${script_dir}/lib/hnvm/colors.sh"
  green "Removed local hnvm from PATH:"
  echo "${PATH}"
fi

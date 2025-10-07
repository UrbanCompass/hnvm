#!/usr/bin/env bash

set -e

source="${BASH_SOURCE[0]}"
while [ -h "${source}" ]; do # resolve $source until the file is no longer a symlink
  script_dir="$( cd -P "$( dirname "${source}" )" >/dev/null 2>&1 && pwd )"
  source="$(readlink "${source}")"
  [[ ${source} != /* ]] && source="${script_dir}/${source}"
done
script_dir="$( cd -P "$( dirname "${source}" )" >/dev/null 2>&1 && pwd )"

node_ver=
pnpm_ver=
yarn_ver=

# shellcheck source=/dev/null
source "${script_dir}/colors.sh"
# shellcheck source=/dev/null
source "${script_dir}/config.sh"

export node_path="${HNVM_PATH}/node/${node_ver}"
export node_bin="${node_path}/bin/node"
export npm_bin="${node_path}/bin/npm"
export npx_bin="${node_path}/bin/npx"

export pnpm_path="${HNVM_PATH}/pnpm/${pnpm_ver}"
export pnpm_bin="${pnpm_path}/bin/pnpm.js"
export pnpx_bin="${pnpm_path}/bin/pnpx.js"

export yarn_path="${HNVM_PATH}/yarn/${yarn_ver}"
export yarn_bin="${yarn_path}/bin/yarn.js"

function download_node() {
  platform=
  if [[ "${OSTYPE}" == "linux-gnu" ]]; then
    platform="linux"
  elif [[ "${OSTYPE}" == "darwin"* ]]; then
    platform="darwin"
  else
    error "OS Platform not supported"
    exit 1
  fi

  cpu_arch="x64"
  if [[ $(uname -m) == "arm64" ]]; then
    node_major=$(echo "$node_ver" | grep -Eo "^\d+")
    if [[ "$node_major" -ge 16 ]]; then
      cpu_arch="arm64"
    fi
  fi

  rm -rf "${node_path}"
  mkdir -p "${node_path}"

  if [[ "$COMMAND_OUTPUT" == "&2" ]]; then
    echo $'\e[1;34m'"Downloading node v${node_ver} to ${HNVM_PATH}/node"$'\e[0m' >&2
  else
    echo $'\e[1;34m'"Downloading node v${node_ver} to ${HNVM_PATH}/node"$'\e[0m' >> "${COMMAND_OUTPUT}"
  fi

  node_download_url="${HNVM_NODE_DIST}/v${node_ver}/node-v${node_ver}-${platform}-${cpu_arch}.tar.gz"
  if [[ "${HNVM_QUIET}" == "true" ]]; then
    if [[ "$COMMAND_OUTPUT" == "&2" ]]; then
      curl "$node_download_url" --silent --fail 2>&2 |
        tar xz -C "${node_path}" --strip-components=1 2>&2
    else
      curl "$node_download_url" --silent --fail |
        tar xz -C "${node_path}" --strip-components=1 >> "${COMMAND_OUTPUT}"
    fi
  else
    if [[ "$COMMAND_OUTPUT" == "&2" ]]; then
      curl "$node_download_url" --fail 2>&2 |
        tar xz -C "${node_path}" --strip-components=1 2>&2
    else
      curl "$node_download_url" --fail |
        tar xz -C "${node_path}" --strip-components=1 >> "${COMMAND_OUTPUT}"
    fi
  fi
}

function download_pnpm() {
  rm -rf "${pnpm_path}"
  mkdir -p "${pnpm_path}"

  if [[ "$COMMAND_OUTPUT" == "&2" ]]; then
    echo $'\e[1;34m'"Downloading pnpm v${pnpm_ver} to ${HNVM_PATH}/pnpm"$'\e[0m' >&2
  else
    echo $'\e[1;34m'"Downloading pnpm v${pnpm_ver} to ${HNVM_PATH}/pnpm"$'\e[0m' >> "${COMMAND_OUTPUT}"
  fi

  pnpm_installer_script=$(cat "${script_dir}/../pnpm-self-installer/install.js")

  if [[ "$COMMAND_OUTPUT" == "&2" ]]; then
    echo "$pnpm_installer_script" | PNPM_VERSION=${pnpm_ver} PNPM_DEST=${pnpm_path} PNPM_REGISTRY=${HNVM_PNPM_REGISTRY} ${node_bin} 2>&2
  else
    echo "$pnpm_installer_script" | PNPM_VERSION=${pnpm_ver} PNPM_DEST=${pnpm_path} PNPM_REGISTRY=${HNVM_PNPM_REGISTRY} ${node_bin} >> \
      "${COMMAND_OUTPUT}"
  fi
}

function download_yarn() {
  rm -rf "${yarn_path}"
  mkdir -p "${yarn_path}"

  if [[ "$COMMAND_OUTPUT" == "&2" ]]; then
    echo $'\e[1;34m'"Downloading yarn v${yarn_ver} to ${HNVM_PATH}/yarn"$'\e[0m' >&2
  else
    echo $'\e[1;34m'"Downloading yarn v${yarn_ver} to ${HNVM_PATH}/yarn"$'\e[0m' >> "${COMMAND_OUTPUT}"
  fi

  if [[ "${HNVM_QUIET}" == "true" ]]; then
    if [[ "$COMMAND_OUTPUT" == "&2" ]]; then
      curl -L "${HNVM_YARN_DIST}/${yarn_ver}/yarn-v${yarn_ver}.tar.gz" --silent --fail 2>&2 |
        tar xz -C "${yarn_path}" --strip-components=1 2>&2
    else
      curl -L "${HNVM_YARN_DIST}/${yarn_ver}/yarn-v${yarn_ver}.tar.gz" --silent --fail |
        tar xz -C "${yarn_path}" --strip-components=1 >> "${COMMAND_OUTPUT}"
    fi
  else
    if [[ "$COMMAND_OUTPUT" == "&2" ]]; then
      curl -L "${HNVM_YARN_DIST}/${yarn_ver}/yarn-v${yarn_ver}.tar.gz" --fail 2>&2 |
        tar xz -C "${yarn_path}" --strip-components=1 2>&2
    else
      curl -L "${HNVM_YARN_DIST}/${yarn_ver}/yarn-v${yarn_ver}.tar.gz" --fail |
        tar xz -C "${yarn_path}" --strip-components=1 >> "${COMMAND_OUTPUT}"
    fi
  fi
}

# Something's globally installing pnpm and pnpx, need to remove otherwise npm scripts won't use
# hnvm and they'll use this globally installed one instead
if [[ -f "${node_path}/bin/pnpm" ]]; then
  if [[ "$COMMAND_OUTPUT" == "&2" ]]; then
    echo $'\e[1;33m'"WARNING"$'\e[0m'": Found conflicting global install of pnpm, removing..." >&2
  else
    echo $'\e[1;33m'"WARNING"$'\e[0m'": Found conflicting global install of pnpm, removing..." >> "${COMMAND_OUTPUT}"
  fi
  rm "${node_path}/bin/pnpm"
fi

if [[ -f "${node_path}/bin/pnpx" ]]; then
  if [[ "$COMMAND_OUTPUT" == "&2" ]]; then
    echo $'\e[1;33m'"WARNING"$'\e[0m'": Found conflicting global install of pnpx, removing..." >&2
  else
    echo $'\e[1;33m'"WARNING"$'\e[0m'": Found conflicting global install of pnpx, removing..." >> "${COMMAND_OUTPUT}"
  fi
  rm "${node_path}/bin/pnpx"
fi

# pnpm 6+ uses .cjs files for its bins
if [ -f "${pnpm_path}/bin/pnpm.cjs" ]; then
  pnpm_bin="${pnpm_path}/bin/pnpm.cjs"
fi

if [ -f "${pnpm_path}/bin/pnpx.cjs" ]; then
  pnpx_bin="${pnpm_path}/bin/pnpx.cjs"
fi

if [[ ! -x "${node_bin}" ]]; then
  download_node
fi

if [[ "${0}" == *pnpm || "${0}" == *pnpx ]] && [[ ! -f "${pnpm_bin}" || "$("${node_bin}" "${pnpm_bin}" -v)" != "${pnpm_ver}" ]]; then
  download_pnpm
fi

if [[ "${0}" == *yarn ]] && [[ ! -f "${yarn_bin} " || "$("${node_bin}" "${yarn_bin}" -v)" != "${yarn_ver}" ]]; then
  download_yarn
fi

# pnpm 6+ uses .cjs files for its bins
if [ -f "${pnpm_path}/bin/pnpm.cjs" ]; then
  pnpm_bin="${pnpm_path}/bin/pnpm.cjs"
fi

if [ -f "${pnpm_path}/bin/pnpx.cjs" ]; then
  pnpx_bin="${pnpm_path}/bin/pnpx.cjs"
fi

if [[ "$COMMAND_OUTPUT" == "&2" ]]; then
  echo $'\e[1;34m'"Using Hermetic NodeJS v${node_ver}"$'\e[0m' >&2
else
  echo $'\e[1;34m'"Using Hermetic NodeJS v${node_ver}"$'\e[0m' >> "${COMMAND_OUTPUT}"
fi

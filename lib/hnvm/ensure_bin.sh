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

function validate_url() {
  local url="$1"
  
  if [[ "${HNVM_SKIP_URL_VALIDATION}" == "true" ]]; then
    return 0
  fi

  if curl --head --silent --fail --location --output /dev/null "$url"; then
    return 0
  else
    error "URL validation failed: ${url}"
    error "The requested package/version may not exist or the URL is incorrect."
    if [[ -n "${HNVM_NODE_VARIANT}" ]]; then
      error "Note: You are using HNVM_NODE_VARIANT='${HNVM_NODE_VARIANT}'"
      error "This variant may not be available for the requested version/platform."
    fi
    return 1
  fi
}

function download_node() {
  platform=
  if [[ "${OSTYPE}" == "linux-"* ]]; then
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

  variant=""
  if ! [[ -z "${HNVM_NODE_VARIANT}" ]]; then
    variant="-${HNVM_NODE_VARIANT}"
  fi

  node_download_url="${HNVM_NODE_DIST}/v${node_ver}/node-v${node_ver}-${platform}-${cpu_arch}${variant}.tar.gz"
  
  # Validate URL before attempting download
  if ! validate_url "$node_download_url" "Node.js v${node_ver}${variant}"; then
    exit 1
  fi

  blue "Downloading node v${node_ver} to ${HNVM_PATH}/node" | write_to_hnvm_output

  if [[ "${HNVM_QUIET}" == "true" ]]; then
    curl "$node_download_url" --silent --fail |
      tar xz -C "${node_path}" --strip-components=1 | write_to_hnvm_output
  else
    curl "$node_download_url" --fail |
      tar xz -C "${node_path}" --strip-components=1 | write_to_hnvm_output
  fi
}

function download_pnpm() {
  rm -rf "${pnpm_path}"
  mkdir -p "${pnpm_path}"

  blue "Downloading pnpm v${pnpm_ver} to ${HNVM_PATH}/pnpm" | write_to_hnvm_output

  pnpm_installer_script=$(cat "${script_dir}/../pnpm-self-installer/install.js")

  echo "$pnpm_installer_script" | PNPM_VERSION=${pnpm_ver} PNPM_DEST=${pnpm_path} PNPM_REGISTRY=${HNVM_PNPM_REGISTRY} ${node_bin} | write_to_hnvm_output
}

function download_yarn() {
  rm -rf "${yarn_path}"
  mkdir -p "${yarn_path}"

  yarn_download_url="${HNVM_YARN_DIST}/${yarn_ver}/yarn-v${yarn_ver}.tar.gz"
  
  # Validate URL before attempting download
  if ! validate_url "$yarn_download_url" "Yarn v${yarn_ver}"; then
    exit 1
  fi

  blue "Downloading yarn v${yarn_ver} to ${HNVM_PATH}/yarn" | write_to_hnvm_output

  if [[ "${HNVM_QUIET}" == "true" ]]; then
    curl -L "$yarn_download_url" --silent --fail |
      tar xz -C "${yarn_path}" --strip-components=1 | write_to_hnvm_output
  else
    curl -L "$yarn_download_url" --fail |
      tar xz -C "${yarn_path}" --strip-components=1 | write_to_hnvm_output
  fi
}

# Something's globally installing pnpm and pnpx, need to remove otherwise npm scripts won't use
# hnvm and they'll use this globally installed one instead
if [[ -f "${node_path}/bin/pnpm" ]]; then
  warning "Found conflicting global install of pnpm, removing..." | write_to_hnvm_output
  rm "${node_path}/bin/pnpm" | write_to_hnvm_output
fi

if [[ -f "${node_path}/bin/pnpx" ]]; then
  warning "Found conflicting global install of pnpx, removing..." | write_to_hnvm_output
  rm "${node_path}/bin/pnpx" | write_to_hnvm_output
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

blue "Using Hermetic NodeJS v${node_ver}" | write_to_hnvm_output

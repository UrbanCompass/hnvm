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
bun_ver=
# Set by config.sh (sourced below) based on the invocation name; declared here so shellcheck can
# see it is assigned before use.
is_bun=false

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

# NOTE: bun_ver may still be an unresolved semver range at this point. bun_path/bun_bin are set
# after bun_ver is resolved, further down (resolution can require download_node, defined below).

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
    if [[ -n "${HNVM_BUN_VARIANT}" ]]; then
      error "Note: You are using HNVM_BUN_VARIANT='${HNVM_BUN_VARIANT}'"
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
  if [[ -n "${HNVM_NODE_VARIANT}" ]]; then
    # prefix variant with '-'
    variant="-${HNVM_NODE_VARIANT}"
  fi

  node_download_url="${HNVM_NODE_DIST}/v${node_ver}/node-v${node_ver}-${platform}-${cpu_arch}${variant}.tar.gz"
  
  # Validate URL before attempting download
  if ! validate_url "$node_download_url"; then
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
  if ! validate_url "$yarn_download_url"; then
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

function download_bun() {
  platform=
  if [[ "${OSTYPE}" == "linux-"* ]]; then
    platform="linux"
  elif [[ "${OSTYPE}" == "darwin"* ]]; then
    platform="darwin"
  else
    error "OS Platform not supported"
    exit 1
  fi

  # bun uses "aarch64" (not node's "arm64") for its ARM builds
  cpu_arch="x64"
  if [[ $(uname -m) == "arm64" || $(uname -m) == "aarch64" ]]; then
    cpu_arch="aarch64"
  fi

  variant=""
  if [[ -n "${HNVM_BUN_VARIANT}" ]]; then
    # prefix variant with '-' (e.g. "baseline" for older CPUs, "musl" for Alpine)
    variant="-${HNVM_BUN_VARIANT}"
  fi

  # e.g. bun-darwin-aarch64, bun-linux-x64-musl
  bun_target="bun-${platform}-${cpu_arch}${variant}"
  bun_download_url="${HNVM_BUN_DIST}/bun-v${bun_ver}/${bun_target}.zip"

  # Validate URL before attempting download
  if ! validate_url "$bun_download_url"; then
    exit 1
  fi

  rm -rf "${bun_path}"
  mkdir -p "${bun_path}/bin"

  blue "Downloading bun v${bun_ver} to ${HNVM_PATH}/bun" | write_to_hnvm_output

  # bun ships a .zip (not a .tar.gz stream), so download to a temp file then unzip
  bun_zip="${bun_path}/${bun_target}.zip"

  if [[ "${HNVM_QUIET}" == "true" ]]; then
    curl -L "$bun_download_url" --silent --fail --output "${bun_zip}" | write_to_hnvm_output
    unzip -q -o "${bun_zip}" -d "${bun_path}" | write_to_hnvm_output
  else
    curl -L "$bun_download_url" --fail --output "${bun_zip}" | write_to_hnvm_output
    unzip -o "${bun_zip}" -d "${bun_path}" | write_to_hnvm_output
  fi

  # The zip extracts to a folder named after the target with the bun binary inside it
  mv "${bun_path}/${bun_target}/bun" "${bun_bin}"
  chmod +x "${bun_bin}"
  rm -rf "${bun_zip}" "${bun_path:?}/${bun_target}"
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

# bun does not run through node, so skip the node download for bun invocations.
if [[ "${is_bun}" != "true" ]] && [[ ! -x "${node_bin}" ]]; then
  download_node
fi

if [[ "${0}" == *pnpm || "${0}" == *pnpx ]] && [[ ! -f "${pnpm_bin}" || "$("${node_bin}" "${pnpm_bin}" -v)" != "${pnpm_ver}" ]]; then
  download_pnpm
fi

if [[ "${0}" == *yarn ]] && [[ ! -f "${yarn_bin}" || "$("${node_bin}" "${yarn_bin}" -v)" != "${yarn_ver}" ]]; then
  download_yarn
fi

# Resolve the bun version here (not in config.sh) because a semver range may need download_node to
# run find-matching-version.js. resolve_ver + find_local_node are defined in config.sh.
if [[ "${is_bun}" == "true" ]]; then
  resolve_ver "bun" "${bun_ver}"
  # resolve_ver_result is set by resolve_ver (defined in config.sh, sourced above)
  # shellcheck disable=SC2154
  bun_ver="${resolve_ver_result}"

  export bun_path="${HNVM_PATH}/bun/${bun_ver}"
  export bun_bin="${bun_path}/bin/bun"

  if [[ ! -x "${bun_bin}" || "$("${bun_bin}" --version 2>/dev/null)" != "${bun_ver}" ]]; then
    download_bun
  fi
fi

# pnpm 6+ uses .cjs files for its bins
if [ -f "${pnpm_path}/bin/pnpm.cjs" ]; then
  pnpm_bin="${pnpm_path}/bin/pnpm.cjs"
fi

if [ -f "${pnpm_path}/bin/pnpx.cjs" ]; then
  pnpx_bin="${pnpm_path}/bin/pnpx.cjs"
fi

if [[ "${is_bun}" == "true" ]]; then
  blue "Using Hermetic bun v${bun_ver}" | write_to_hnvm_output
else
  blue "Using Hermetic NodeJS v${node_ver}" | write_to_hnvm_output
fi

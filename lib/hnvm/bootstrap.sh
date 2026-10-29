#!/usr/bin/env bash
# Finds (downloading it if necessary) the hnvm Go binary and execs it as the tool named by $0.
# Sourced by bin/*. Written for bash 3.2 (the macOS default); needs only curl, tar and a sha256 tool.
#
# Lookup order:
#   1. $HNVM_GO_BIN                      development and tests
#   2. <repo>/dist/hnvm                  a local `go build`, or a release tarball layout
#   3. $HNVM_PATH/.hnvm-bin/<ver>/hnvm   cached download
#   4. download hnvm_<os>_<arch>.tar.gz from
#      ${HNVM_RELEASE_DIST:-https://github.com/UrbanCompass/hnvm/releases/download}, verified against checksums.txt
#
# The version downloaded is, in order:
#   1. lib/hnvm/bootstrap-version        written into the release source tarball (Homebrew) by CI
#   2. the vX.Y.Z tag the checkout is on a basher install pinned to a tag
#   3. the latest release                anything else (e.g. master), rechecked at most hourly

hnvm_bootstrap_fail() {
  echo "hnvm: $*" >&2
  exit 1
}

hnvm_bootstrap_sha256() {
  if command -v sha256sum > /dev/null 2>&1; then
    sha256sum "$1" | cut -d ' ' -f 1
  elif command -v shasum > /dev/null 2>&1; then
    shasum -a 256 "$1" | cut -d ' ' -f 1
  else
    hnvm_bootstrap_fail "need sha256sum or shasum to verify the download"
  fi
}

hnvm_bootstrap_download() {
  local version="$1" dest_dir="$2"
  local dist="${HNVM_RELEASE_DIST:-https://github.com/UrbanCompass/hnvm/releases/download}"
  local os arch machine archive tmp expected actual

  case "$(uname -s)" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    *) hnvm_bootstrap_fail "unsupported OS $(uname -s)" ;;
  esac
  machine="$(uname -m)"
  case "${machine}" in
    x86_64 | amd64) arch=amd64 ;;
    arm64 | aarch64) arch=arm64 ;;
    *) hnvm_bootstrap_fail "unsupported architecture ${machine}" ;;
  esac

  archive="hnvm_${os}_${arch}.tar.gz"
  mkdir -p "${HNVM_PATH}/.tmp" || hnvm_bootstrap_fail "cannot create ${HNVM_PATH}/.tmp"
  tmp="$(mktemp -d "${HNVM_PATH}/.tmp/bootstrap.XXXXXX")" || hnvm_bootstrap_fail "cannot create a temporary directory"
  # shellcheck disable=SC2064
  trap "rm -rf '${tmp}'" EXIT

  echo "hnvm: downloading hnvm ${version} (${os}/${arch})" >&2
  curl -fsSL "${dist}/v${version}/${archive}" -o "${tmp}/${archive}" ||
    hnvm_bootstrap_fail "could not download ${dist}/v${version}/${archive}"
  curl -fsSL "${dist}/v${version}/checksums.txt" -o "${tmp}/checksums.txt" ||
    hnvm_bootstrap_fail "could not download ${dist}/v${version}/checksums.txt"

  # Fail closed: a missing or mismatching checksum is never a reason to run the binary anyway.
  expected="$(awk -v f="${archive}" '$2 == f { print $1 }' "${tmp}/checksums.txt")"
  [ -n "${expected}" ] || hnvm_bootstrap_fail "${archive} is not listed in checksums.txt"
  actual="$(hnvm_bootstrap_sha256 "${tmp}/${archive}")"
  [ "${actual}" = "${expected}" ] || hnvm_bootstrap_fail "checksum mismatch for ${archive} (expected ${expected}, got ${actual})"

  mkdir -p "${tmp}/out"
  tar -xzf "${tmp}/${archive}" -C "${tmp}/out" hnvm || hnvm_bootstrap_fail "could not extract ${archive}"
  chmod +x "${tmp}/out/hnvm"

  # Atomic publish: concurrent first runs each download, and renaming a file over the destination is atomic and
  # idempotent. (Renaming a directory is not: mv would move it *inside* an existing destination.)
  mkdir -p "${dest_dir}" || hnvm_bootstrap_fail "could not create ${dest_dir}"
  if ! mv -f "${tmp}/out/hnvm" "${dest_dir}/hnvm.$$" || ! mv -f "${dest_dir}/hnvm.$$" "${dest_dir}/hnvm"; then
    hnvm_bootstrap_fail "could not install hnvm into ${dest_dir}"
  fi
  trap - EXIT
  rm -rf "${tmp}"
}

# Prints the highest vX.Y.Z tag on HEAD of the checkout at $1, without the v, if there is one.
hnvm_bootstrap_tag_version() {
  [ -e "$1/.git" ] && command -v git > /dev/null 2>&1 || return 0
  git -C "$1" tag --points-at HEAD 2> /dev/null | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' |
    sed 's/^v//' | sort -t . -k 1,1n -k 2,2n -k 3,3n | tail -n 1
}

# Prints the latest release's version. The answer is cached for an hour; when the lookup fails a stale
# cached answer is used rather than failing.
hnvm_bootstrap_latest_version() {
  local cache="${HNVM_PATH}/.hnvm-bin/latest" version=

  if [ -s "${cache}" ] && [ -n "$(find "${cache}" -mmin -60 2> /dev/null)" ]; then
    tr -d '[:space:]' < "${cache}"
    return
  fi

  if [ -n "${HNVM_RELEASE_DIST:-}" ]; then
    # Mirrors publish the latest version as a plain text file next to the release directories
    version="$(curl -fsSL "${HNVM_RELEASE_DIST}/latest" 2> /dev/null | tr -d '[:space:]')"
  else
    # GitHub redirects releases/latest to releases/tag/vX.Y.Z
    version="$(curl -fsSL -o /dev/null -w '%{url_effective}' https://github.com/UrbanCompass/hnvm/releases/latest 2> /dev/null)"
    version="${version##*/v}"
  fi

  if echo "${version}" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
    mkdir -p "${HNVM_PATH}/.hnvm-bin" && echo "${version}" > "${cache}.$$" && mv -f "${cache}.$$" "${cache}"
    echo "${version}"
  elif [ -s "${cache}" ]; then
    tr -d '[:space:]' < "${cache}"
  else
    hnvm_bootstrap_fail "could not look up the latest hnvm release"
  fi
}

hnvm_bootstrap_exec() {
  local script_dir="$1" tool="$2"
  shift 2
  local bin version

  HNVM_PATH="${HNVM_PATH:-$HOME/.hnvm}"

  if [ -n "${HNVM_GO_BIN:-}" ]; then
    bin="${HNVM_GO_BIN}"
  elif [ -x "${script_dir}/../dist/hnvm" ]; then
    bin="${script_dir}/../dist/hnvm"
  else
    if [ -s "${script_dir}/../lib/hnvm/bootstrap-version" ]; then
      version="$(tr -d '[:space:]' < "${script_dir}/../lib/hnvm/bootstrap-version")"
    else
      version="$(hnvm_bootstrap_tag_version "${script_dir}/..")"
      [ -n "${version}" ] || version="$(hnvm_bootstrap_latest_version)" || exit 1
    fi
    bin="${HNVM_PATH}/.hnvm-bin/${version}/hnvm"
    [ -x "${bin}" ] || hnvm_bootstrap_download "${version}" "${HNVM_PATH}/.hnvm-bin/${version}"
  fi

  [ -x "${bin}" ] || hnvm_bootstrap_fail "${bin} is not executable"
  # argv[0] selects which tool the binary behaves as
  exec -a "${tool}" "${bin}" "$@"
}

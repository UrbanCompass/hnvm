#!/usr/bin/env bash

# Raises a pinned Node version that has known security problems to the fixed release for its line.
# Sourced by config.sh once the Node version is resolved to an exact version. Written for bash 3.2 (the macOS default).
#
# Reads:  node_ver (an exact version), HNVM_MIN_SAFE_NODE, HNVM_MIN_SAFE_NODE_MAJOR
# Sets:   node_ver, when a version is replaced
# Rules live in min-safe-node.txt next to this file, so the next incident only needs a new line there.

# Returns 0 if exact version $1 is lower than exact version $2
function version_lt() {
  local -a a b
  local i
  IFS=. read -ra a <<< "$1"
  IFS=. read -ra b <<< "$2"
  for i in 0 1 2; do
    if (( 10#${a[i]} < 10#${b[i]} )); then return 0; fi
    if (( 10#${a[i]} > 10#${b[i]} )); then return 1; fi
  done
  return 1
}

# Security notices are not hidden by HNVM_QUIET (which points COMMAND_OUTPUT at /dev/null), but they do follow
# HNVM_OUTPUT_DESTINATION when one is set. A failed write (stderr closed, a full disk) never stops hnvm from running node.
function security_warning() {
  local message dest="${HNVM_OUTPUT_DESTINATION:-}"
  message=$'\e[33mWarning\e[0m: '"$1"
  if [[ -n "${dest}" && ! -S "${dest}" ]]; then
    { echo -e "${message}" >> "${dest}"; } 2>/dev/null || true
  else
    # ">&2" first, so the message goes to the real stderr; only the command's own error output is discarded
    echo -e "${message}" >&2 2>/dev/null || true
  fi
}

function apply_min_safe_node() {
  # shellcheck disable=SC2154 # script_dir is set by config.sh
  local rules_file="${script_dir}/min-safe-node.txt"
  local pinned="${node_ver}"
  local major="${pinned%%.*}"
  local key safe rule_safe

  [[ "${pinned}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ && -f "${rules_file}" ]] || return 0

  safe=""
  while read -r key rule_safe _; do
    [[ -z "${key}" || "${key}" == \#* ]] && continue
    if [[ "${key}" == "<"* ]]; then
      (( major < ${key#<} )) && { safe="${rule_safe}"; break; }
    elif [[ "${key}" == "${major}" ]]; then
      safe="${rule_safe}"
      break
    fi
  done < "${rules_file}"

  # No rule (a newer line than any rule here), or already at or above the safe version
  [[ -n "${safe}" ]] || return 0
  if [[ "${safe%%.*}" == "${major}" ]]; then
    version_lt "${pinned}" "${safe}" || return 0
    [[ "${HNVM_MIN_SAFE_NODE:-true}" == "false" ]] && return 0
    security_warning "node ${pinned} has known security problems; using ${safe} instead. Set HNVM_MIN_SAFE_NODE=false to opt out."
    node_ver="${safe}"
  else
    if [[ "${HNVM_MIN_SAFE_NODE_MAJOR:-false}" == "true" ]]; then
      security_warning "node ${pinned} has known security problems and no fix on its line; using ${safe} instead."
      node_ver="${safe}"
    else
      security_warning "node ${pinned} has known security problems and no fix on its line. Move to ${safe} or newer, or set HNVM_MIN_SAFE_NODE_MAJOR=true to have hnvm do it."
    fi
  fi
}

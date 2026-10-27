#!/usr/bin/env bash

# Managed (MDM) preferences: a configuration profile for the com.compass.hnvm domain is written to /Library/Managed Preferences,
# which only an administrator can change. For the keys read here, a managed value wins over the environment and every .hnvmrc.
# Sourced by config.sh. Written for bash 3.2 (the macOS default).

# This is a plain assignment (not an overridable default) so that no environment variable can point it somewhere else.
managed_prefs_plist="/Library/Managed Preferences/com.compass.hnvm"

# Prints the managed value of key $1 as `defaults` prints it. Prints nothing when it is not set or this is not macOS.
# Checking for the file first means a machine without the profile never pays for a `defaults` subprocess.
function managed_pref_raw() {
  [[ -x /usr/bin/defaults && -f "${managed_prefs_plist}.plist" ]] || return 0
  /usr/bin/defaults read "${managed_prefs_plist}" "$1" 2>/dev/null || true
}

# Prints the managed value of key $1 as true or false. Prints nothing when it is not set, is not a boolean, or this is not macOS.
function managed_pref() {
  case "$(managed_pref_raw "$1" | tr '[:upper:]' '[:lower:]')" in
    1|true|yes) echo true ;;
    0|false|no) echo false ;;
  esac
}

# Prints the managed value of key $1 as a comma-separated list. It may be a string (already comma-separated) or an array,
# which `defaults` prints as one quoted or bare element per line between ( and ). Prints nothing when it is not set.
function managed_pref_list() {
  local raw
  raw="$(managed_pref_raw "$1")"
  if [[ "${raw}" == "("* ]]; then
    raw="$(echo "${raw}" | sed -e '1d' -e '$d' -e 's/^[[:space:]]*"\{0,1\}//' -e 's/"\{0,1\},\{0,1\}[[:space:]]*$//' | paste -s -d ',' -)"
  fi
  echo "${raw}"
}

# HNVM_DISTRUST_CERTS, HNVM_DISTRUST_CERTS_EXTRA and HNVM_DISTRUST_CERTS_STRICT: a managed value replaces whatever the
# environment and .hnvmrc set. Sets distrust_certs_managed, distrust_certs_extra_managed and distrust_certs_strict_managed
# to true for the keys it replaced, so messages don't suggest a setting the user cannot change.
# shellcheck disable=SC2034 # read by ensure_bin.sh
function apply_managed_distrust_certs() {
  local managed

  managed="$(managed_pref HNVM_DISTRUST_CERTS)"
  if [[ -n "${managed}" ]]; then
    HNVM_DISTRUST_CERTS="${managed}"
    distrust_certs_managed=true
  fi

  managed="$(managed_pref_list HNVM_DISTRUST_CERTS_EXTRA)"
  if [[ -n "${managed}" ]]; then
    HNVM_DISTRUST_CERTS_EXTRA="${managed}"
    distrust_certs_extra_managed=true
  fi

  managed="$(managed_pref HNVM_DISTRUST_CERTS_STRICT)"
  if [[ -n "${managed}" ]]; then
    HNVM_DISTRUST_CERTS_STRICT="${managed}"
    distrust_certs_strict_managed=true
  fi
}

#!/usr/bin/env bash

set -e

# Print Coloring Functions
function green {
  echo $'\e[1;32m'"$*"$'\e[0m'
}

function red {
  echo $'\e[1;31m'"$*"$'\e[0m'
}

function blue {
  echo $'\e[1;34m'"$*"$'\e[0m'
}

function yellow {
  echo $'\e[1;33m'"$*"$'\e[0m'
}

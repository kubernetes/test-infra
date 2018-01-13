#!/bin/bash
# Copyright 2018 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


# Identifies go_library() bazel targets in the workspace with no dependencies.
# Deletes the target and its files. Cleans up any :all-srcs references

# By default just checks for extra libraries, run with --fix to make changes.


set -o nounset
set -o errexit
set -o pipefail

# Output every go_library rule in the repo
find-go-libraries() {
  # Excluding generated //foo:go_default_library~library~
  bazel query \
    'kind("go_library rule", //...)' \
    except 'attr("name", ".*~library~", //...)'
}

# Output the target if it has only one dependency (itself)
is-unused() {
  local n
  n="$(bazel query "rdeps(//..., \"$1\", 1)" | wc -l)"
  if [[ "$n" -gt "1" ]]; then  # Always counts itself
    echo "LIVE: $1" >&2
    return 0
  fi
  echo "DEAD: $1" >&2
  echo "$1"
}

# Filter down to unused targets
filter-to-unused() {
  for i in "$@"; do
    is-unused "$i"
  done
}

# Output this:source.go that:file.txt sources of //this //that targets
target-sources() {
  (for i in "$@"; do
    bazel query "kind(\"source file\", deps(\"$(package-name "$i"):package-srcs\", 1))"
  done) | sort -u
}

sources() {
  local t
  for j in $(target-sources "${@}"); do
    case "$(target-name "$j")" in
      LICENSE*|PATENTS*|*.md)  # Keep these files
        continue
        ;;
    esac
    echo $(package-name "${j:2}")/$(target-name $j)
  done
}

# Convert //foo:and/bar:stuff to //foo:and/bar
package-name() {
  echo ${1%:*}
}

# Convert //foo:and/bar:stuff to stuff
target-name() {
  echo ${1##*:}
}

# Find every package that directly depends on //foo:all-srcs
all-srcs-refs() {
  packages $(for i in "$@"; do
    bazel query "rdeps(//..., \"${i}:all-srcs\", 1)"
  done)
}

# Convert //foo/bar:stuff //this //foo/bar:stuff to //foo/bar //this
packages() {
  (for i in "$@"; do
    echo $(package-name "${i}")
  done) | sort -u
}

# Convert //foo //bar to foo/BUILD bar/BUILD
builds() {
  for i in "${@}"; do
    echo ${i:2}/BUILD
  done
}


# Remove lines with //something:all-srcs from files
# Usage:
#   remove-all-srcs <targets-to-remove> <remove-from-packages>
remove-all-srcs() {
  for b in $1; do
    sed -i -e "\|${b}:all-srcs|d" $(builds $2)
  done
}

# Global variable to track when we are finished
remove_unused_go_libraries_finished=
# Remove every go_library() target from workspace with no one depending on it.
remove-unused-go-libraries() {
  local libraries deps
  libraries="$(find-go-libraries | sort -u)"
  deps="$(filter-to-unused $libraries)"
  if [[ -z "$deps" ]]; then
    echo "All remaining go libraries are alive" >&2
    remove_unused_go_libraries_finished=true
    return 0
  elif [[ "$1" == "--check" || "$1" != "--fix" ]]; then
    echo "Found unused libraries:" >&2
    for d in $deps; do
      echo $d
    done
    exit 1
  else
    echo Cleaning up unused dependencies... >&2
  fi
  local unused_packs unused_files all_srcs_packs
  unused_packs=$(packages $deps)
  unused_files=$(sources $deps)
  # Packages with //unused-package:all-srcs references
  all_srcs_packs=$(all-srcs-refs $unused_packs)

  pushd "$(dirname ${BASH_SOURCE})/.."
  remove-all-srcs "$unused_packs" "$all_srcs_packs"
  rm -f $unused_files $(builds $unused_packs)
  popd
}

# Continue until we do not remove anything
while [[ -z $remove_unused_go_libraries_finished ]]; do
  remove-unused-go-libraries "${1:-}"
done

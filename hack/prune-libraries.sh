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

# Output sources used by target
target-sources() {
  for i in "$@"; do
    bazel query "kind(\"source file\", deps(\"$(package-name "$i"):package-srcs\", 1))"
  done
}

# Convert //foo/bar:stuff to //foo/bar
package-name() {
  echo $1 | cut -d : -f 1
}

# Find every package that directly depends on //foo:all-srcs
all-srcs-refs() {
  build-files $(for i in "$@"; do
    local package
    package="$(dirname "$i")"
    bazel query "rdeps(//..., \"${package}:all-srcs\", 1)"
  done)
}

# Convert //foo/bar:stuff to //foo/bar/BUILD
build-files() {
  (for i in "$@"; do
    echo $(package-name "$i")/BUILD
  done) | sort -u
}

# Remove lines with //something:all-srcs from files
# Usage:
#   remove-all-srcs <targets-to-remove> <remove-from-packages>
remove-all-srcs() {
  for b in $1; do
    sed -i -e "\|$(dirname "$b"):all-srcs|d" $(to-path $2)
  done
}

# For each arg //foo/bar:whatever.sh becomes foo/bar/whatever.sh
to-path() {
  for i in "$@"; do
    # Format is //foo/bar:whatever.sh
    # Output foo/bar/whatever.sh
    local base dir
    base=$(basename $i)
    dir=$(dirname $i)
    echo "${dir:2}/${base/:/\/}"
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
  local builds files ref_builds
  builds=$(build-files $deps)
  files=$(target-sources $deps)
  ref_builds=$(all-srcs-refs $builds)

  pushd "$(dirname ${BASH_SOURCE})/.."
  remove-all-srcs "$builds" "$ref_builds"
  git rm $(to-path $files | sort -u)
  git rm -f $(to-path $builds | sort -u)
  popd
}

# Continue until we do not remove anything
while [[ -z $remove_unused_go_libraries_finished ]]; do
  remove-unused-go-libraries "${1:-}"
done

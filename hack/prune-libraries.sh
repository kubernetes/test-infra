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

# This list is used for rules in vendor/ which may not have any explicit
# dependencies outside of vendor, e.g. helper commands we have vendored.
# It should probably match the list in Gopkg.toml.
REQUIRED=(
  //vendor/github.com/bazelbuild/bazel-gazelle/cmd/gazelle:gazelle
  //vendor/github.com/golang/dep/cmd/dep:dep
  //vendor/github.com/client9/misspell/cmd/misspell:misspell
  //vendor/k8s.io/repo-infra/kazel:kazel
)

# darwin is great
SED=sed
if which gsed &>/dev/null; then
  SED=gsed
fi
if ! ($SED --version 2>&1 | grep -q GNU); then
  echo "!!! GNU sed is required.  If on OS X, use 'brew install gnu-sed'." >&2
  exit 1
fi

unused-go-libraries() {
  # Find all the go_library rules in vendor except those that something outside
  # of vendor eventually depends on.
  required_items=( "${REQUIRED[@]/#/+ }" )
  echo "Looking for //vendor targets that no one outside of //vendor depends on..." >&2
  bazel query "kind('go_library rule', //vendor/... -deps(//... -//vendor/... ${required_items[@]}))"
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

# Convert //foo //bar to foo/BUILD bar/BUILD.bazel (whichever exist)
builds() {
  for i in "${@}"; do
    echo $(ls ${i:2}/{BUILD,BUILD.bazel} 2>/dev/null)
  done
}

# Remove lines with //something:all-srcs from files
# Usage:
#   remove-all-srcs <targets-to-remove> <remove-from-packages>
remove-all-srcs() {
  for b in $1; do
    $SED -i -e "\|${b}:all-srcs|d" $(builds $2)
  done
}

# Global variable to track when we are finished
remove_unused_go_libraries_finished=
# Remove every go_library() target from workspace with no one depending on it.
remove-unused-go-libraries() {
  local libraries deps
  deps="$(unused-go-libraries)"
  if [[ -z "$deps" ]]; then
    echo "All remaining go libraries are alive" >&2
    remove_unused_go_libraries_finished=true
    return 0
  fi
  echo "Unused libraries:" >&2
  for d in $deps; do
    echo "  DEAD: $d" >&2
  done
  if [[ "$1" == "--check" || "$1" != "--fix" ]]; then
    echo "Correct with $0 --fix" >&2
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

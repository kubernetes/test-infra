#!/usr/bin/env bash
# Copyright 2021 The Kubernetes Authors.
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

set -o nounset
set -o errexit
set -o pipefail

SCRIPT_ROOT="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="${SCRIPT_ROOT}/../../.."

function usage() {
  >&2 cat <<EOF
Build Prow components and deploy them into the KIND test cluster.

Usage: $0 [OPTIONS] [packages]

Examples:
  # Update all packages with '-u' flag in the root folder. Run "go mod tidy".
  $0 --minor

  # Update all packages with '-u=patch' flag in the root folder. Run "go mod tidy".
  $0 --patch

  # Update "ko" package in the hack/tools/go.mod file. Run "go mod tidy".
  $0 --minor --tools github.com/google/ko

Options:
    --minor:
        Pass "-u" flag to "go get". Run "go mod tidy".

    --patch:
        Pass "-u=patch" flag to "go get". Run "go mod tidy".

    --tools:
        Do the actions inside the hack/tools folder, not the root folder. All
        other flags are still observed.

    --only-tidy:
        Don't update anything. Instead just run "go mod tidy" in the root and
        hack/tools folders. This is the behavior if no arguments are provided.

    --help:
        Display this help message.
EOF
}

function update() {
  declare -a packages
  local update_mode
  update_mode="${1}"
  shift
  packages=("$@")
  # Update all packages if no packages are provided.
  if [[ -z "${packages[*]}" ]]; then
    packages=("./...")
  fi

  echo >&2 "Running" go get "${update_mode}" "${packages[@]}"
  go get "${update_mode}" "${packages[@]}"
}

function tidy() {
  echo >&2 "Running" go mod tidy
  go mod tidy
}

function main() {
  declare -a packages
  local update_mode
  local tools
  local only_tidy=0

  if ! (($#)); then
    only_tidy=1
  fi

  for arg in "$@"; do
    case "${arg}" in
      --minor)
        update_mode="-u"
        ;;
      --patch)
        update_mode="-u=patch"
        ;;
      --tools)
        tools="${arg}"
        ;;
      --only-tidy)
        only_tidy=1
        ;;
      --help)
        usage
        return
        ;;
      -*)
        echo >&2 "Unrecognized option '${arg}'"
        usage
        return
        ;;
      *)
        packages+=("${arg}")
        ;;
    esac
  done

  echo >&2 "Ensuring go version."
  # shellcheck disable=SC1091
  source "${REPO_ROOT}"/hack/build/setup-go.sh

  echo >&2 "Go version: $(go version)"

  export GO111MODULE=on
  export GOPROXY=https://proxy.golang.org
  export GOSUMDB=sum.golang.org

  if ((only_tidy)); then
    pushd "${REPO_ROOT}"
    tidy
    pushd "${REPO_ROOT}/hack/tools"
    tidy
    echo >&2 "SUCCESS: ran go mod tidy in root and hack/tools folders"
    return
  fi

  if [[ -n "${tools:-}" ]]; then
    pushd "${REPO_ROOT}/hack/tools"
  else
    pushd "${REPO_ROOT}"
  fi
  if [[ -n "${update_mode:-}" ]]; then
    update "${update_mode}" "${packages[@]}"
  fi

  tidy

  echo >&2 "SUCCESS: updated modules"
}

trap 'echo "FAILED" >&2' ERR

main "$@"

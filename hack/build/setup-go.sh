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

# cd to the repo root and setup go
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
cd "${REPO_ROOT}"

# read go-version file unless GO_VERSION is set
GO_VERSION="${GO_VERSION:-"$(cat .go-version)"}"

# only setup go if we haven't set FORCE_HOST_GO, or `go version` doesn't match
# go version output looks like:
# go version go1.14.5 darwin/amd64
if ! ([ -n "${FORCE_HOST_GO:-}" ] ||
      (command -v go >/dev/null && [ "$(go version | cut -d' ' -f3)" = "go${GO_VERSION}" ])); then

    if ! (command -v gvm >/dev/null); then
        apt-get install bison
        bash -c "$(curl -s -S -L https://raw.githubusercontent.com/moovweb/gvm/master/binscripts/gvm-installer)"
        source /root/.gvm/scripts/gvm
    fi

    gvm install go${GO_VERSION}
    gvm use go${GO_VERSION}
fi

# force go modules
export GO111MODULE=on

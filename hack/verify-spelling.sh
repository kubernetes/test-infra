#!/usr/bin/env bash
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

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

go install ./vendor/github.com/client9/misspell/cmd/misspell
if ! which misspell >/dev/null 2>&1; then
    echo "Can't find misspell - is your GOPATH 'bin' in your PATH?"
    echo "  GOPATH: ${GOPATH}"
    echo "  PATH:   ${PATH}"
    exit 1
fi

git ls-files | grep -v -e vendor -e static -e third_party | xargs misspell -error

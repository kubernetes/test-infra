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

# bump.sh will
# * ensure there are no pending changes
# * optionally activate GOOGLE_APPLICATION_CREDENTIALS and configure-docker if set
# * run //prow:release-push to build and push prow images
# * update all the cluster/*.yaml files to use the new image tags

set -o errexit
set -o nounset
set -o pipefail

# TODO(fejta): rewrite this in a better language REAL SOON

# See https://misc.flogisoft.com/bash/tip_colors_and_formatting

color-version() { # Bold blue
  echo -e "\x1B[1;34m${@}\x1B[0m"
}

color-error() { # Light red
  echo -e "\x1B[91m${@}\x1B[0m"
}

color-target() { # Bold cyan
  echo -e "\x1B[1;33m${@}\x1B[0m"
}

# darwin is great
SED=sed
if which gsed &>/dev/null; then
  SED=gsed
fi
if ! ($SED --version 2>&1 | grep -q GNU); then
  echo "!!! GNU sed is required.  If on OS X, use 'brew install gnu-sed'." >&2
  exit 1
fi

if [[ -n "${GOOGLE_APPLICATION_CREDENTIALS:-}" ]]; then
  echo "Detected GOOGLE_APPLICATION_CREDENTIALS, activating..." >&2
  gcloud auth activate-service-account --key-file="$GOOGLE_APPLICATION_CREDENTIALS"
  gcloud auth configure-docker
fi

# Build and push the current commit, failing on any uncommitted changes.
new_version="v$(date -u '+%Y%m%d')-$(git describe --tags --always --dirty)"
echo -e "version: $(color-version ${new_version})" >&2
if [[ "${new_version}" == *-dirty ]]; then
  echo -e "$(color-error ERROR): uncommitted changes to repo" >&2
  echo "  Fix with git commit" >&2
  exit 1
fi
echo -e "Pushing $(color-version ${new_version}) via $(color-target //prow:release-push --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64) ..." >&2
bazel run //prow:release-push --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64

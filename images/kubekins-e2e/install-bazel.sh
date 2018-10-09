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

# this script fetches and runs the upstream installer based on BAZEL_VERSION
# like 0.14.0 or 0.14.0rc1 etc.
set -o errexit
set -o nounset

# match BAZEL_VERSION to installer URL
INSTALLER="bazel-${BAZEL_VERSION}-installer-linux-x86_64.sh"
if [[ "${BAZEL_VERSION}" =~ ([0-9\.]+)(rc[0-9]+) ]]; then
    DOWNLOAD_URL="https://storage.googleapis.com/bazel/${BASH_REMATCH[1]}/${BASH_REMATCH[2]}/${INSTALLER}"
else
    DOWNLOAD_URL="https://github.com/bazelbuild/bazel/releases/download/${BAZEL_VERSION}/${INSTALLER}"
fi
echo "$DOWNLOAD_URL"
# get the installer
wget -q "${DOWNLOAD_URL}" && chmod +x "${INSTALLER}"
# install to user dir
"./${INSTALLER}"
# remove the installer, we no longer need it
rm "${INSTALLER}"

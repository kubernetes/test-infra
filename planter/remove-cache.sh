#!/bin/sh
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

# This is a utility to remove the Bazel cache after using Planter
#
# After using planter the cache contents are correctly owned by the host user,
# but some files are not writable so we make them writable and then delete
# the cache dir rather than the normal `bazel clean`.
# NOTE: this is a rough approximation of `bazel clean --expunge`
set -o errexit
set -o nounset

BAZEL_CACHE="${HOME}/.cache/bazel"
chmod -R +w "${BAZEL_CACHE}" && rm -rf "${BAZEL_CACHE}"

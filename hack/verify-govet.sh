#!/usr/bin/env bash
# Copyright 2017 The Kubernetes Authors.
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

echo "Calling verify-govet.sh is no longer necessary: vetting is run automatically as part of all builds."
echo "Building all go targets to verify that we pass vetting."
# Don't bother with go_binary because that just forces us to link code we already checked.
bazel build $(bazel query --keep_going --noshow_progress 'kind("go_library|go_test", //...)')

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

# used in presubmits / CI testing
# bazel build then unit test, exiting non-zero if either failed

res=0

bazel=(
  bazel
)

if [[ "${BAZEL_REMOTE_CACHE_ENABLED}" == "true" ]]; then
  # TODO(fejta): remove logic associated with this env
  echo "Disabling deprecated http cache in system/home bazelrcs..." >&2
  bazel+=(
    --nosystem_rc
    --nohome_rc
  )
fi

"${bazel[@]}" build --config=ci //...
if [[ $? -ne 0 ]]; then
    res=1
fi

"${bazel[@]}" test --config=ci //... --config=unit
if [[ $? -ne 0 ]]; then
    res=1
fi

exit ${res}

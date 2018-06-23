#!/bin/sh
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

# write a fake user entry with settings matching the host user
# note that this is different from the user we installed bazel as, we want
# it to look just like the user calling planter outside the container
# so that the file permissions, log paths etc are the same, and we will
# run the container as this $UID:$GID so tools like python will expect
# a matching entry here to lookup $HOME, etc.
echo "${USER}:!:${UID}:${GID}:${FULL_NAME:-}:${HOME}:/bin/bash" >> /etc/passwd

# actually run the user's command
exec "$@"

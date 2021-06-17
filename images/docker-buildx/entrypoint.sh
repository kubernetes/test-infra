#!/bin/sh
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

set -euo pipefail

docker run --rm --privileged multiarch/qemu-user-static@sha256:c772ee1965aa0be9915ee1b018a0dd92ea361b4fa1bcab5bbc033517749b2af4 --reset -p yes

/root/.docker/cli-plugins/docker-buildx create --use
/root/.docker/cli-plugins/docker-buildx "$@"

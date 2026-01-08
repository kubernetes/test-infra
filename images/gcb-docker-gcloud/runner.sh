#!/usr/bin/env bash
# Copyright 2025 The Kubernetes Authors.
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

# generic runner script

early_exit_handler() {
    if [ -n "${WRAPPED_COMMAND_PID:-}" ]; then
        kill -TERM "$WRAPPED_COMMAND_PID" || true
    fi
}

trap early_exit_handler INT TERM

# disable error exit so we can run post-command cleanup
set +o errexit

# Handle initialising buildx multiarch
if [ -z "${BUILDX_NO_DEFAULT_ATTESTATIONS}" ]; then
    export BUILDX_NO_DEFAULT_ATTESTATIONS=1
fi
docker run --privileged --rm tonistiigi/binfmt:qemu-v10.0.4 --install all

docker buildx create \
    --name multiarch-multiplatform-builder \
    --driver docker-container \
    --bootstrap --use

# actually start bootstrap and the job
set -o xtrace
"$@" &
WRAPPED_COMMAND_PID=$!
wait $WRAPPED_COMMAND_PID
EXIT_VALUE=$?
set +o xtrace

# preserve exit value from job / bootstrap
exit ${EXIT_VALUE}

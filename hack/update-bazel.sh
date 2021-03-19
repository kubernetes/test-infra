#!/usr/bin/env bash
# Copyright 2016 The Kubernetes Authors.
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


bazel-from-image() {
	if [ -x "$(command -v docker)" ]; then
	    CONTAINER_ENGINE=docker
	elif [ -x "$(command -v podman)" ]; then
	    CONTAINER_ENGINE=podman
	else
		echo "There is no docker or podman installed. Please use hack/update-bazel.sh script instead."
		exit 1
	fi

	WORKDIR=$(pwd)
	TEST_INFRA_PATH=${WORKDIR%/hack}


	$CONTAINER_ENGINE run --user $(id -u):$(id -g) --volume ${TEST_INFRA_PATH}:/test-infra:Z \
	--workdir /test-infra --rm gcr.io/k8s-testimages/launcher.gcr.io/google/bazel:latest-test-infra $@
}

bazel-direct() {
  bazel $@
}

bazel-from-bazelisk() {
  bazelisk $@
}

if [[ "${1:-}" == "--from-image" ]]; then
	bazel=bazel-from-image
elif [ -x "$(command -v bazelisk)" ]; then
  bazel=bazel-from-bazelisk
elif [ -x "$(command -v bazel)" ]; then
  bazel=bazel-direct
else
  bazel=bazel-from-image
fi

"$bazel" run @io_k8s_repo_infra//hack:update-bazel

exit $?
